// Package api exposes the calculator over a JSON REST API.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/sebaarossi/calculator/backend/internal/calculator"
)

// maxBodyBytes caps request bodies; a calculation request is a few dozen bytes.
const maxBodyBytes = 1 << 10

// Error codes are part of the API contract: clients branch on them, while
// messages are meant for humans and may change.
const (
	codeInvalidRequest    = "invalid_request"
	codeUnknownOperation  = "unknown_operation"
	codeDivisionByZero    = "division_by_zero"
	codeNegativeSqrt      = "negative_square_root"
	codeUndefinedResult   = "undefined_result"
	codeResultOutOfRange  = "result_out_of_range"
	codeInternal          = "internal_error"
	codeRequestTooLarge   = "request_too_large"
	codeUnsupportedMethod = "method_not_allowed"
)

type calculateRequest struct {
	Operation string `json:"operation"`
	// Pointers distinguish an omitted operand from an explicit zero.
	A *float64 `json:"a"`
	B *float64 `json:"b"`
}

type calculateResponse struct {
	Operation calculator.Operation `json:"operation"`
	A         float64              `json:"a"`
	B         *float64             `json:"b,omitempty"`
	Result    float64              `json:"result"`
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

// NewRouter builds the HTTP handler for the service.
//
// If staticDir is non-empty, files in it are served at "/" so a single binary
// can host both the API and the built frontend.
func NewRouter(logger *slog.Logger, staticDir string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("GET /api/v1/operations", handleOperations)
	mux.HandleFunc("POST /api/v1/calculate", handleCalculate)
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

func handleCalculate(w http.ResponseWriter, r *http.Request) {
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
	if req.A == nil {
		writeError(w, http.StatusBadRequest, codeInvalidRequest, `field "a" is required`)
		return
	}

	resp := calculateResponse{Operation: op, A: *req.A}
	var b float64
	if info.Operands > 1 {
		if req.B == nil {
			writeError(w, http.StatusBadRequest, codeInvalidRequest,
				fmt.Sprintf(`field "b" is required for operation %q`, req.Operation))
			return
		}
		b = *req.B
		resp.B = req.B
	}

	result, err := calculator.Calculate(op, *req.A, b)
	if err != nil {
		status, code := classify(err)
		writeError(w, status, code, err.Error())
		return
	}
	resp.Result = result
	writeJSON(w, http.StatusOK, resp)
}

// handleAPIFallback keeps error responses JSON for anything under /api/ that
// did not match a route, instead of the mux's plain-text defaults.
func handleAPIFallback(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/v1/calculate":
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, codeUnsupportedMethod, "use POST for this endpoint")
	case "/api/v1/operations":
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, codeUnsupportedMethod, "use GET for this endpoint")
	default:
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
	}
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
			return fmt.Errorf("field %q has an invalid value", typeErr.Field)
		case errors.Is(err, io.EOF):
			return errors.New("request body must not be empty")
		default:
			return errors.New("request body must be a valid JSON object with fields operation, a and b")
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
