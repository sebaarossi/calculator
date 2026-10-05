// Package api exposes the calculator over a JSON REST API.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/sebaarossi/calculator/backend/internal/calculator"
	"github.com/sebaarossi/calculator/backend/internal/history"
	"github.com/sebaarossi/calculator/backend/internal/paymentplan"
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
	codeUnknownPlan       = "unknown_plan"
	codeAmountOutOfRange  = "amount_out_of_range"
	codeNotFound          = "not_found"
	codeInternal          = "internal_error"
	codeRequestTooLarge   = "request_too_large"
	codeUnsupportedMedia  = "unsupported_media_type"
	codeUnsupportedMethod = "method_not_allowed"
)

// calculateRequest is the body of POST /api/v1/calculations as the client
// sent it, before validation. Every field is a pointer so that a missing field
// can be told apart from an empty one.
//
// Operands are decimal strings ("0.1"), never JSON numbers, so no client or
// proxy can turn them into binary floating point on the way.
type calculateRequest struct {
	Operation *string
	A         *string
	B         *string
}

// requestError is a problem with a request that should be reported to the
// client as is.
type requestError struct {
	status  int
	code    string
	message string
}

func badRequest(format string, args ...any) *requestError {
	return &requestError{http.StatusBadRequest, codeInvalidRequest, fmt.Sprintf(format, args...)}
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
// can host both the API and the built frontend. Otherwise "/" redirects to the
// API documentation.
func NewRouter(logger *slog.Logger, service *history.Service, staticDir string) http.Handler {
	h := &handler{history: service, logger: logger}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("GET /api/v1/operations", handleOperations)
	mux.HandleFunc("POST /api/v1/calculations", h.createCalculation)
	mux.HandleFunc("GET /api/v1/calculations", h.listCalculations)
	mux.HandleFunc("GET /api/v1/calculations/{id}", h.getCalculation)
	mux.HandleFunc("POST /api/v1/payment-plans", h.createPaymentPlan)
	mux.HandleFunc("GET /api/v1/openapi.yaml", handleOpenAPISpec)
	mux.HandleFunc("GET /docs", handleSwaggerUI)
	mux.HandleFunc("/api/", handleAPIFallback)

	if staticDir != "" {
		mux.Handle("/", http.FileServer(http.Dir(staticDir)))
	} else {
		// Running as an API only: point a visitor to the root at the docs
		// instead of showing a bare 404.
		mux.Handle("GET /{$}", http.RedirectHandler("/docs", http.StatusFound))
	}

	return logRequests(logger, recoverPanics(logger, mux))
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleOperations(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, operationsResponse{Operations: calculator.Operations()})
}

// createCalculation performs a calculation and adds it to the history.
func (h *handler) createCalculation(w http.ResponseWriter, r *http.Request) {
	req, err := readCalculateRequest(w, r)
	if err != nil {
		writeRequestError(w, err)
		return
	}
	op, a, b, err := req.validate()
	if err != nil {
		writeRequestError(w, err)
		return
	}

	record, calcErr := h.history.Calculate(r.Context(), op, a, b)
	if calcErr != nil {
		h.writeDomainError(w, calcErr)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/v1/calculations/%d", record.ID))
	writeJSON(w, http.StatusCreated, record)
}

// validate applies every rule a calculation request must satisfy and returns
// the operation with its parsed operands:
//
//   - operation is required and must be a supported one;
//   - a is required;
//   - b is required for binary operations and not allowed for unary ones, so a
//     client can never believe an operand was used when it was ignored;
//   - operands must be finite decimal numbers within the supported precision.
func (req calculateRequest) validate() (op calculator.Operation, a, b calculator.Number, err *requestError) {
	if req.Operation == nil || *req.Operation == "" {
		return "", a, b, badRequest(`field "operation" is required`)
	}
	op = calculator.Operation(*req.Operation)
	info, ok := calculator.Lookup(op)
	if !ok {
		return "", a, b, &requestError{http.StatusBadRequest, codeUnknownOperation,
			fmt.Sprintf("unknown operation %q; see GET /api/v1/operations", *req.Operation)}
	}

	if a, err = parseOperand("a", req.A); err != nil {
		return "", a, b, err
	}
	if info.Operands == 1 {
		if req.B != nil {
			return "", a, b, badRequest(`field "b" is not allowed for operation %q, which takes one operand`, op)
		}
		return op, a, b, nil
	}
	if b, err = parseOperand("b", req.B); err != nil {
		return "", a, b, err
	}
	return op, a, b, nil
}

// parseOperand reads a required decimal operand.
func parseOperand(field string, value *string) (calculator.Number, *requestError) {
	if value == nil {
		return calculator.Number{}, badRequest("field %q is required", field)
	}
	number, err := calculator.Parse(*value)
	if err != nil {
		return calculator.Number{}, badRequest(
			`field %q must be a decimal number with at most %d significant digits, such as "12.5"`,
			field, calculator.Precision)
	}
	return number, nil
}

// paymentPlanRequest is the body of POST /api/v1/payment-plans before validation.
type paymentPlanRequest struct {
	Plan         *string
	Amount       *string
	PurchaseDate *string
}

// createPaymentPlan splits a purchase into installments. Nothing is stored:
// the same request always produces the same schedule.
func (h *handler) createPaymentPlan(w http.ResponseWriter, r *http.Request) {
	var req paymentPlanRequest
	fields := map[string]**string{"plan": &req.Plan, "amount": &req.Amount, "purchaseDate": &req.PurchaseDate}
	if err := readObject(w, r, fields); err != nil {
		writeRequestError(w, err)
		return
	}
	plan, amount, purchase, err := req.validate()
	if err != nil {
		writeRequestError(w, err)
		return
	}

	schedule, buildErr := paymentplan.Build(plan, amount, purchase)
	if buildErr != nil {
		h.writeDomainError(w, buildErr)
		return
	}
	writeJSON(w, http.StatusOK, schedule)
}

// validate checks that every field is present and well formed. Whether the
// amount can actually be split is a business rule and is left to the domain.
func (req paymentPlanRequest) validate() (paymentplan.Plan, paymentplan.Money, paymentplan.Date, *requestError) {
	var noDate paymentplan.Date
	if req.Plan == nil || *req.Plan == "" {
		return "", 0, noDate, badRequest(`field "plan" is required`)
	}
	if req.Amount == nil {
		return "", 0, noDate, badRequest(`field "amount" is required`)
	}
	if req.PurchaseDate == nil {
		return "", 0, noDate, badRequest(`field "purchaseDate" is required`)
	}
	amount, err := paymentplan.ParseMoney(*req.Amount)
	if err != nil {
		return "", 0, noDate, badRequest(`field "amount": %v`, err)
	}
	purchase, err := paymentplan.ParseDate(*req.PurchaseDate)
	if err != nil {
		return "", 0, noDate, badRequest(`field "purchaseDate": %v`, err)
	}
	return paymentplan.Plan(*req.Plan), amount, purchase, nil
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
	// Reached only when no route matched. If the path is a real endpoint the
	// method must be the problem; anything else does not exist.
	allowed := ""
	switch path := r.URL.Path; {
	case path == "/api/v1/calculations":
		allowed = "GET, POST"
	case path == "/api/v1/operations", isCalculationPath(path):
		allowed = "GET"
	case path == "/api/v1/payment-plans":
		allowed = "POST"
	}
	if allowed == "" || strings.Contains(allowed, r.Method) {
		writeError(w, http.StatusNotFound, codeNotFound, "no such endpoint")
		return
	}
	methodNotAllowed(w, allowed)
}

// isCalculationPath reports whether path has the shape /api/v1/calculations/{id}.
func isCalculationPath(path string) bool {
	id, found := strings.CutPrefix(path, "/api/v1/calculations/")
	return found && id != "" && !strings.Contains(id, "/")
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
	case errors.Is(err, paymentplan.ErrUnknownPlan):
		return http.StatusBadRequest, codeUnknownPlan
	case errors.Is(err, paymentplan.ErrAmountTooSmall), errors.Is(err, paymentplan.ErrAmountTooLarge):
		return http.StatusUnprocessableEntity, codeAmountOutOfRange
	default:
		return http.StatusInternalServerError, codeInternal
	}
}

func readCalculateRequest(w http.ResponseWriter, r *http.Request) (calculateRequest, *requestError) {
	var req calculateRequest
	err := readObject(w, r, map[string]**string{"operation": &req.Operation, "a": &req.A, "b": &req.B})
	return req, err
}

// readObject reads a JSON object whose values are all strings into the given
// fields. Unlike encoding/json's default behaviour it rejects duplicate keys,
// keys in the wrong case and unknown keys: when a body is ambiguous, two
// systems can disagree about what it means, and a financial API must not guess.
func readObject(w http.ResponseWriter, r *http.Request, fields map[string]**string) *requestError {
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType != "application/json" {
		return &requestError{http.StatusUnsupportedMediaType, codeUnsupportedMedia,
			"Content-Type must be application/json"}
	}

	seen := map[string]bool{}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))

	if token, err := dec.Token(); err != nil || token != json.Delim('{') {
		if errors.Is(err, io.EOF) {
			return badRequest("request body must not be empty")
		}
		return malformedBody(err)
	}
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return malformedBody(err)
		}
		key, _ := token.(string) // object keys are always strings
		field, known := fields[key]
		switch {
		case !known:
			return badRequest("unknown field %q", key)
		case seen[key]:
			return badRequest("field %q appears more than once", key)
		}
		seen[key] = true

		if err := dec.Decode(field); err != nil {
			var typeErr *json.UnmarshalTypeError
			if errors.As(err, &typeErr) {
				return badRequest("field %q must be a string", key)
			}
			return malformedBody(err)
		}
	}
	if _, err := dec.Token(); err != nil { // the closing brace
		return malformedBody(err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return malformedBody(err)
		}
		return badRequest("request body must contain a single JSON object")
	}
	return nil
}

// malformedBody explains why a body could not be read as a JSON object.
func malformedBody(err error) *requestError {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return &requestError{http.StatusRequestEntityTooLarge, codeRequestTooLarge,
			fmt.Sprintf("request body must not exceed %d bytes", maxBodyBytes)}
	}
	return badRequest("request body must be a valid JSON object")
}

func writeRequestError(w http.ResponseWriter, err *requestError) {
	writeError(w, err.status, err.code, err.message)
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
