// Package api exposes the calculator over a JSON REST API.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/sebaarossi/calculator/backend/internal/calculator"
	"github.com/sebaarossi/calculator/backend/internal/history"
)

const (
	// maxBodyBytes caps request bodies; a calculation request is a few dozen bytes.
	maxBodyBytes = 1 << 10

	defaultPageSize = 20
	maxPageSize     = 100
)

// Error codes are part of the API contract: clients branch on them, while
// messages are meant for humans and may change.
const (
	codeInvalidRequest    = "invalid_request"
	codeUnknownOperation  = "unknown_operation"
	codeDivisionByZero    = "division_by_zero"
	codeNegativeSqrt      = "negative_square_root"
	codeUndefinedResult   = "undefined_result"
	codeResultOutOfRange  = "result_out_of_range"
	codeNotFound          = "not_found"
	codeInternal          = "internal_error"
	codeRequestTooLarge   = "request_too_large"
	codeUnsupportedMethod = "method_not_allowed"
)

type calculateRequest struct {
	Operation string `json:"operation"`
	// Operands are decimal strings ("0.1"), never JSON numbers, so no client or
	// proxy can turn them into binary floating point on the way. Pointers
	// distinguish an omitted operand from an explicit one.
	A *string `json:"a"`
	B *string `json:"b"`
}

type listResponse struct {
	Calculations []history.Record `json:"calculations"`
	// NextCursor is passed back as ?cursor= to fetch the next page; absent on the last one.
	NextCursor string `json:"nextCursor,omitempty"`
}

type operationsResponse struct {
	Operations []calculator.Info `json:"operations"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type handler struct {
	history *history.Service
	logger  *slog.Logger
}

// NewRouter builds the HTTP handler for the service.
//
// If staticDir is non-empty, files in it are served at "/" so a single binary
// can host both the API and the built frontend.
func NewRouter(logger *slog.Logger, service *history.Service, staticDir string) http.Handler {
	h := &handler{history: service, logger: logger}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("GET /api/v1/operations", handleOperations)
	mux.HandleFunc("POST /api/v1/calculations", h.createCalculation)
	mux.HandleFunc("GET /api/v1/calculations", h.listCalculations)
	mux.HandleFunc("GET /api/v1/calculations/{id}", h.getCalculation)
	mux.HandleFunc("GET /api/v1/openapi.yaml", handleOpenAPISpec)
	mux.HandleFunc("GET /docs", handleSwaggerUI)
	mux.HandleFunc("/api/", handleAPIFallback)

	if staticDir != "" {
		mux.Handle("/", http.FileServer(http.Dir(staticDir)))
	}

	return logRequests(logger, mux)
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleOperations(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, operationsResponse{Operations: calculator.Operations()})
}

// createCalculation performs a calculation and adds it to the history.
func (h *handler) createCalculation(w http.ResponseWriter, r *http.Request) {
	var req calculateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, codeRequestTooLarge, "request body is too large")
			return
		}
		writeError(w, http.StatusBadRequest, codeInvalidRequest, err.Error())
		return
	}

	if req.Operation == "" {
		writeError(w, http.StatusBadRequest, codeInvalidRequest, `field "operation" is required`)
		return
	}
	op := calculator.Operation(req.Operation)
	info, ok := calculator.Lookup(op)
	if !ok {
		writeError(w, http.StatusBadRequest, codeUnknownOperation,
			fmt.Sprintf("unknown operation %q; see GET /api/v1/operations", req.Operation))
		return
	}
	a, ok := parseOperand(w, "a", req.A)
	if !ok {
		return
	}
	var b calculator.Number
	if info.Operands > 1 {
		if b, ok = parseOperand(w, "b", req.B); !ok {
			return
		}
	}

	record, err := h.history.Calculate(r.Context(), op, a, b)
	if err != nil {
		h.writeDomainError(w, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/v1/calculations/%d", record.ID))
	writeJSON(w, http.StatusCreated, record)
}

// parseOperand reads a required decimal operand, writing the error response
// and returning false if it is missing or not a valid number.
func parseOperand(w http.ResponseWriter, field string, value *string) (calculator.Number, bool) {
	if value == nil {
		writeError(w, http.StatusBadRequest, codeInvalidRequest, fmt.Sprintf("field %q is required", field))
		return calculator.Number{}, false
	}
	number, err := calculator.Parse(*value)
	if err != nil {
		writeError(w, http.StatusBadRequest, codeInvalidRequest, fmt.Sprintf(
			`field %q must be a decimal number with at most %d significant digits, such as "12.5"`,
			field, calculator.Precision))
		return calculator.Number{}, false
	}
	return number, true
}

// listCalculations returns one page of the history, newest first.
func (h *handler) listCalculations(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	limit := defaultPageSize
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maxPageSize {
			writeError(w, http.StatusBadRequest, codeInvalidRequest,
				fmt.Sprintf(`query parameter "limit" must be between 1 and %d`, maxPageSize))
			return
		}
		limit = parsed
	}

	var before int64
	if raw := query.Get("cursor"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, `query parameter "cursor" is not valid`)
			return
		}
		before = parsed
	}

	page, err := h.history.List(r.Context(), limit, before)
	if err != nil {
		h.writeDomainError(w, err)
		return
	}
	resp := listResponse{Calculations: page.Records}
	if page.Next != 0 {
		resp.NextCursor = strconv.FormatInt(page.Next, 10)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *handler) getCalculation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		// An ID that is not a number cannot belong to any calculation.
		h.writeDomainError(w, history.ErrNotFound)
		return
	}
	record, err := h.history.Get(r.Context(), id)
	if err != nil {
		h.writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

// writeDomainError reports an error from the service. Unexpected errors are
// logged and hidden from the client.
func (h *handler) writeDomainError(w http.ResponseWriter, err error) {
	status, code := classify(err)
	message := err.Error()
	if status == http.StatusInternalServerError {
		h.logger.Error("request failed", "error", err)
		message = "internal server error"
	}
	writeError(w, status, code, message)
}

// handleAPIFallback keeps error responses JSON for anything under /api/ that
// did not match a route, instead of the mux's plain-text defaults.
func handleAPIFallback(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/api/v1/calculations":
		methodNotAllowed(w, "GET, POST")
	case strings.HasPrefix(path, "/api/v1/calculations/"), path == "/api/v1/operations":
		methodNotAllowed(w, "GET")
	default:
		writeError(w, http.StatusNotFound, codeNotFound, "no such endpoint")
	}
}

func methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeError(w, http.StatusMethodNotAllowed, codeUnsupportedMethod, "method not allowed; use "+allowed)
}

// classify maps domain errors to an HTTP status and a stable error code.
// Malformed requests are 400; well-formed requests that have no valid
// mathematical answer are 422.
func classify(err error) (status int, code string) {
	switch {
	case errors.Is(err, calculator.ErrUnknownOperation):
		return http.StatusBadRequest, codeUnknownOperation
	case errors.Is(err, calculator.ErrInvalidOperand):
		return http.StatusBadRequest, codeInvalidRequest
	case errors.Is(err, calculator.ErrDivisionByZero):
		return http.StatusUnprocessableEntity, codeDivisionByZero
	case errors.Is(err, calculator.ErrNegativeSqrt):
		return http.StatusUnprocessableEntity, codeNegativeSqrt
	case errors.Is(err, calculator.ErrUndefinedResult):
		return http.StatusUnprocessableEntity, codeUndefinedResult
	case errors.Is(err, calculator.ErrOutOfRange):
		return http.StatusUnprocessableEntity, codeResultOutOfRange
	case errors.Is(err, history.ErrNotFound):
		return http.StatusNotFound, codeNotFound
	default:
		return http.StatusInternalServerError, codeInternal
	}
}

// decodeJSON reads a single JSON object from the request body into dst,
// returning errors whose messages are safe to show to API clients.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var typeErr *json.UnmarshalTypeError
		var tooLarge *http.MaxBytesError
		switch {
		case errors.As(err, &tooLarge):
			return err
		case errors.As(err, &typeErr) && typeErr.Field != "":
			return fmt.Errorf("field %q must be a string", typeErr.Field)
		case errors.Is(err, io.EOF):
			return errors.New("request body must not be empty")
		default:
			return errors.New(`request body must be a JSON object such as {"operation": "add", "a": "1", "b": "2"}`)
		}
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// The status line is already sent, so an encoding failure here can only be
	// a broken connection; there is nothing useful left to tell the client.
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message}})
}
