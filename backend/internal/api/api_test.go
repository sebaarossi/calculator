package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sebaarossi/calculator/backend/internal/calculator"
	"github.com/sebaarossi/calculator/backend/internal/history"
	"github.com/sebaarossi/calculator/backend/internal/paymentplan"
	"github.com/sebaarossi/calculator/backend/internal/storage/memory"
)

func newRouterWith(repo history.Repository, staticDir string) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(logger, history.NewService(repo), staticDir)
}

func newTestRouter() http.Handler {
	return newRouterWith(memory.New(100), "")
}

// do sends a request the way a well-behaved client would: bodies are JSON.
func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return doWithContentType(t, h, method, path, body, "application/json")
}

func doWithContentType(t *testing.T, h http.Handler, method, path, body, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("response is not valid JSON: %v\nbody: %s", err, rec.Body)
	}
	return v
}

func wantError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, status, rec.Body)
	}
	got := decode[errorResponse](t, rec)
	if got.Error.Code != code {
		t.Errorf("error code = %q, want %q", got.Error.Code, code)
	}
	if got.Error.Message == "" {
		t.Error("error message is empty")
	}
}

// seed records n calculations (1+0, 2+0, ...) through the API.
func seed(t *testing.T, h http.Handler, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		body := fmt.Sprintf(`{"operation":"add","a":"%d","b":"0"}`, i)
		if rec := do(t, h, http.MethodPost, "/api/v1/calculations", body); rec.Code != http.StatusCreated {
			t.Fatalf("seeding failed: %d %s", rec.Code, rec.Body)
		}
	}
}

func TestCreateCalculation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string // the response without its createdAt field
	}{
		{"add", `{"operation":"add","a":"2","b":"3"}`, `{"id":1,"operation":"add","a":"2","b":"3","result":"5"`},
		{"add is exact", `{"operation":"add","a":"0.1","b":"0.2"}`, `{"id":1,"operation":"add","a":"0.1","b":"0.2","result":"0.3"`},
		{"subtract", `{"operation":"subtract","a":"2","b":"3"}`, `{"id":1,"operation":"subtract","a":"2","b":"3","result":"-1"`},
		{"multiply", `{"operation":"multiply","a":"1.5","b":"4"}`, `{"id":1,"operation":"multiply","a":"1.5","b":"4","result":"6"`},
		{"divide", `{"operation":"divide","a":"9","b":"2"}`, `{"id":1,"operation":"divide","a":"9","b":"2","result":"4.5"`},
		{"power", `{"operation":"power","a":"2","b":"8"}`, `{"id":1,"operation":"power","a":"2","b":"8","result":"256"`},
		{"percentage", `{"operation":"percentage","a":"20","b":"150"}`, `{"id":1,"operation":"percentage","a":"20","b":"150","result":"30"`},
		{"sqrt", `{"operation":"sqrt","a":"81"}`, `{"id":1,"operation":"sqrt","a":"81","result":"9"`},
		{"null b on a unary operation", `{"operation":"sqrt","a":"81","b":null}`, `{"id":1,"operation":"sqrt","a":"81","result":"9"`},
		{"fields in any order, with whitespace", " {\n\t\"b\": \"3\", \"a\": \"2\", \"operation\": \"add\" }\n", `{"id":1,"operation":"add","a":"2","b":"3","result":"5"`},
		{"trailing zeros are not significant digits", `{"operation":"add","a":"1.` + strings.Repeat("0", 50) + `","b":"1"}`, `{"id":1,"operation":"add","a":"1","b":"1","result":"2"`},
		{"negative zero", `{"operation":"add","a":"-0","b":"0"}`, `{"id":1,"operation":"add","a":"0","b":"0","result":"0"`},
		{"largest operand", `{"operation":"percentage","a":"1E+6144","b":"10"}`, `{"id":1,"operation":"percentage","a":"1E+6144","b":"10","result":"1E+6143"`},
		{"explicit zero operands", `{"operation":"add","a":"0","b":"0"}`, `{"id":1,"operation":"add","a":"0","b":"0","result":"0"`},
		{"scientific notation", `{"operation":"multiply","a":"1.5E+3","b":"2"}`, `{"id":1,"operation":"multiply","a":"1500","b":"2","result":"3000"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, newTestRouter(), http.MethodPost, "/api/v1/calculations", tt.body)
			if rec.Code != http.StatusCreated {
				t.Fatalf("status = %d, want 201; body: %s", rec.Code, rec.Body)
			}
			if got := rec.Header().Get("Location"); got != "/api/v1/calculations/1" {
				t.Errorf("Location = %q, want /api/v1/calculations/1", got)
			}
			if !strings.HasPrefix(rec.Body.String(), tt.want+`,"createdAt":"`) {
				t.Errorf("body = %s, want it to start with %s", rec.Body, tt.want)
			}
			if got := decode[history.Record](t, rec); time.Since(got.CreatedAt) > time.Minute {
				t.Errorf("createdAt = %v, want a current timestamp", got.CreatedAt)
			}
		})
	}
}

func TestCreateCalculationErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{"empty body", ``, 400, codeInvalidRequest},
		{"malformed JSON", `{"operation":`, 400, codeInvalidRequest},
		{"not an object", `[1,2]`, 400, codeInvalidRequest},
		{"trailing data", `{"operation":"add","a":"1","b":"2"}{}`, 400, codeInvalidRequest},
		{"unknown field", `{"operation":"add","a":"1","b":"2","c":3}`, 400, codeInvalidRequest},
		{"operand is a JSON number", `{"operation":"add","a":1,"b":"2"}`, 400, codeInvalidRequest},
		{"operand is not a number", `{"operation":"add","a":"abc","b":"2"}`, 400, codeInvalidRequest},
		{"operand is empty", `{"operation":"add","a":"","b":"2"}`, 400, codeInvalidRequest},
		{"operand is NaN", `{"operation":"add","a":"NaN","b":"2"}`, 400, codeInvalidRequest},
		{"operand has too many digits", `{"operation":"add","a":"` + strings.Repeat("9", calculator.Precision+1) + `","b":"2"}`, 400, codeInvalidRequest},
		{"second operand is not a number", `{"operation":"add","a":"1","b":"1,5"}`, 400, codeInvalidRequest},
		{"operand out of range", `{"operation":"add","a":"1e9999","b":"2"}`, 400, codeInvalidRequest},
		{"JSON null", `null`, 400, codeInvalidRequest},
		{"JSON string", `"add"`, 400, codeInvalidRequest},
		{"unterminated object", `{"operation":"add","a":"1","b":"2"`, 400, codeInvalidRequest},
		{"missing value", `{"operation":"add","a":,"b":"2"}`, 400, codeInvalidRequest},
		{"mismatched closing bracket", `{"operation":"add","a":"1","b":"2"]`, 400, codeInvalidRequest},
		{"duplicate field", `{"operation":"add","a":"1","a":"5","b":"2"}`, 400, codeInvalidRequest},
		{"field in the wrong case", `{"operation":"add","A":"1","b":"2"}`, 400, codeInvalidRequest},
		{"operation is a number", `{"operation":1,"a":"1","b":"2"}`, 400, codeInvalidRequest},
		{"operand is an object", `{"operation":"add","a":{"value":"1"},"b":"2"}`, 400, codeInvalidRequest},
		{"operand is a boolean", `{"operation":"add","a":true,"b":"2"}`, 400, codeInvalidRequest},
		{"operand has surrounding spaces", `{"operation":"add","a":" 1 ","b":"2"}`, 400, codeInvalidRequest},
		{"operand uses a decimal comma", `{"operation":"add","a":"1,5","b":"2"}`, 400, codeInvalidRequest},
		{"operand uses thousands separators", `{"operation":"add","a":"1,000.00","b":"2"}`, 400, codeInvalidRequest},
		{"operand uses non-ASCII digits", `{"operation":"add","a":"１２","b":"2"}`, 400, codeInvalidRequest},
		{"operand is hexadecimal", `{"operation":"add","a":"0x10","b":"2"}`, 400, codeInvalidRequest},
		{"operand is infinite", `{"operation":"add","a":"Infinity","b":"2"}`, 400, codeInvalidRequest},
		{"operand below the supported range", `{"operation":"add","a":"1E-6144","b":"2"}`, 400, codeInvalidRequest},
		{"b on a unary operation", `{"operation":"sqrt","a":"81","b":"7"}`, 400, codeInvalidRequest},
		{"empty operation", `{"operation":"","a":"1","b":"2"}`, 400, codeInvalidRequest},
		{"null operation", `{"operation":null,"a":"1","b":"2"}`, 400, codeInvalidRequest},
		{"operation in the wrong case", `{"operation":"ADD","a":"1","b":"2"}`, 400, codeUnknownOperation},
		{"operation with spaces", `{"operation":" add ","a":"1","b":"2"}`, 400, codeUnknownOperation},
		{"result too close to zero", `{"operation":"divide","a":"1E-6143","b":"10"}`, 422, codeResultOutOfRange},
		{"zero to the power of zero", `{"operation":"power","a":"0","b":"0"}`, 422, codeUndefinedResult},
		{"division by zero written as 0.00", `{"operation":"divide","a":"1","b":"0.00"}`, 422, codeDivisionByZero},
		{"division by negative zero", `{"operation":"divide","a":"1","b":"-0"}`, 422, codeDivisionByZero},
		{"body too large, in trailing data", `{"operation":"add","a":"1","b":"2"}` + strings.Repeat(" ", maxBodyBytes), 413, codeRequestTooLarge},
		{"missing operation", `{"a":"1","b":"2"}`, 400, codeInvalidRequest},
		{"missing a", `{"operation":"add","b":"2"}`, 400, codeInvalidRequest},
		{"null a", `{"operation":"add","a":null,"b":"2"}`, 400, codeInvalidRequest},
		{"missing b", `{"operation":"add","a":"1"}`, 400, codeInvalidRequest},
		{"unknown operation", `{"operation":"modulo","a":"1","b":"2"}`, 400, codeUnknownOperation},
		{"division by zero", `{"operation":"divide","a":"1","b":"0"}`, 422, codeDivisionByZero},
		{"negative square root", `{"operation":"sqrt","a":"-1"}`, 422, codeNegativeSqrt},
		{"undefined result", `{"operation":"power","a":"-8","b":"0.5"}`, 422, codeUndefinedResult},
		{"result out of range", `{"operation":"multiply","a":"1E+6144","b":"10"}`, 422, codeResultOutOfRange},
		{"body too large", `{"operation":"add","a":"1","b":"` + strings.Repeat("1", maxBodyBytes) + `"}`, 413, codeRequestTooLarge},
	}

	h := newTestRouter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantError(t, do(t, h, http.MethodPost, "/api/v1/calculations", tt.body), tt.wantStatus, tt.wantCode)
		})
	}

	// None of the failures above may have been recorded.
	if got := decode[listResponse](t, do(t, h, http.MethodGet, "/api/v1/calculations", "")); len(got.Calculations) != 0 {
		t.Errorf("history has %d records after only failed requests, want 0", len(got.Calculations))
	}
}

func TestCreateCalculationRequiresJSONContentType(t *testing.T) {
	const body = `{"operation":"add","a":"1","b":"2"}`
	h := newTestRouter()

	for _, contentType := range []string{"", "text/plain", "application/x-www-form-urlencoded", "multipart/form-data; boundary=x", "application/jsonx", "garbage;;"} {
		rec := doWithContentType(t, h, http.MethodPost, "/api/v1/calculations", body, contentType)
		wantError(t, rec, http.StatusUnsupportedMediaType, codeUnsupportedMedia)
	}
	for _, contentType := range []string{"application/json", "application/json; charset=utf-8", "Application/JSON"} {
		rec := doWithContentType(t, h, http.MethodPost, "/api/v1/calculations", body, contentType)
		if rec.Code != http.StatusCreated {
			t.Errorf("Content-Type %q: status = %d, want 201", contentType, rec.Code)
		}
	}
}

// Error messages must tell the client what to fix.
func TestCreateCalculationErrorMessages(t *testing.T) {
	tests := []struct{ body, want string }{
		{``, "must not be empty"},
		{`{"operation":"add","a":"1","a":"5","b":"2"}`, `"a" appears more than once`},
		{`{"operation":"add","a":"1","b":"2","c":"3"}`, `unknown field "c"`},
		{`{"operation":"add","a":1,"b":"2"}`, `"a" must be a string`},
		{`{"operation":"add","a":"1"}`, `"b" is required`},
		{`{"operation":"sqrt","a":"1","b":"2"}`, `"b" is not allowed for operation "sqrt"`},
		{`{"operation":"add","a":"abc","b":"2"}`, `"a" must be a decimal number`},
		{`{"operation":"modulo","a":"1","b":"2"}`, `unknown operation "modulo"`},
	}

	h := newTestRouter()
	for _, tt := range tests {
		rec := do(t, h, http.MethodPost, "/api/v1/calculations", tt.body)
		if got := decode[errorResponse](t, rec).Error.Message; !strings.Contains(got, tt.want) {
			t.Errorf("body %s: message = %q, want it to contain %q", tt.body, got, tt.want)
		}
	}
}

func TestCreatePaymentPlan(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			"pay in 4",
			`{"plan":"pay_in_4","amount":"99.99","purchaseDate":"2026-10-05"}`,
			`{"plan":"pay_in_4","amount":"99.99","currency":"USD","interest":"0.00","intervalDays":14,"installments":[` +
				`{"number":1,"dueDate":"2026-10-05","amount":"25.02"},` +
				`{"number":2,"dueDate":"2026-10-19","amount":"24.99"},` +
				`{"number":3,"dueDate":"2026-11-02","amount":"24.99"},` +
				`{"number":4,"dueDate":"2026-11-16","amount":"24.99"}]}`,
		},
		{
			"pay in 2, whole dollars",
			`{"plan":"pay_in_2","amount":"50","purchaseDate":"2026-12-25"}`,
			`{"plan":"pay_in_2","amount":"50.00","currency":"USD","interest":"0.00","intervalDays":14,"installments":[` +
				`{"number":1,"dueDate":"2026-12-25","amount":"25.00"},` +
				`{"number":2,"dueDate":"2027-01-08","amount":"25.00"}]}`,
		},
	}

	h := newTestRouter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, h, http.MethodPost, "/api/v1/payment-plans", tt.body)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.want {
				t.Errorf("body = %s\nwant   %s", got, tt.want)
			}
		})
	}

	// A plan is a computation, not a calculation to be recorded.
	if got := decode[listResponse](t, do(t, h, http.MethodGet, "/api/v1/calculations", "")); len(got.Calculations) != 0 {
		t.Errorf("history has %d records after building payment plans, want 0", len(got.Calculations))
	}
}

func TestCreatePaymentPlanErrors(t *testing.T) {
	plan := func(fields string) string { return "{" + fields + "}" }
	const valid = `"plan":"pay_in_4","amount":"100","purchaseDate":"2026-10-05"`

	tests := []struct {
		name        string
		body        string
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{"empty body", ``, 400, codeInvalidRequest, "must not be empty"},
		{"malformed JSON", `{"plan":`, 400, codeInvalidRequest, "valid JSON object"},
		{"unknown field", plan(valid + `,"apr":"0"`), 400, codeInvalidRequest, `unknown field "apr"`},
		{"duplicate field", plan(valid + `,"amount":"1"`), 400, codeInvalidRequest, `"amount" appears more than once`},
		{"missing plan", plan(`"amount":"100","purchaseDate":"2026-10-05"`), 400, codeInvalidRequest, `"plan" is required`},
		{"missing amount", plan(`"plan":"pay_in_4","purchaseDate":"2026-10-05"`), 400, codeInvalidRequest, `"amount" is required`},
		{"missing date", plan(`"plan":"pay_in_4","amount":"100"`), 400, codeInvalidRequest, `"purchaseDate" is required`},
		{"amount as a JSON number", plan(`"plan":"pay_in_4","amount":100,"purchaseDate":"2026-10-05"`), 400, codeInvalidRequest, `"amount" must be a string`},
		{"negative amount", plan(`"plan":"pay_in_4","amount":"-100","purchaseDate":"2026-10-05"`), 400, codeInvalidRequest, "at most two decimals"},
		{"fraction of a cent", plan(`"plan":"pay_in_4","amount":"10.005","purchaseDate":"2026-10-05"`), 400, codeInvalidRequest, "at most two decimals"},
		{"thousands separator", plan(`"plan":"pay_in_4","amount":"1,000.00","purchaseDate":"2026-10-05"`), 400, codeInvalidRequest, "at most two decimals"},
		{"amount is not a number", plan(`"plan":"pay_in_4","amount":"lots","purchaseDate":"2026-10-05"`), 400, codeInvalidRequest, "at most two decimals"},
		{"impossible date", plan(`"plan":"pay_in_4","amount":"100","purchaseDate":"2026-02-30"`), 400, codeInvalidRequest, "calendar date"},
		{"date with a time", plan(`"plan":"pay_in_4","amount":"100","purchaseDate":"2026-10-05T10:00:00Z"`), 400, codeInvalidRequest, "calendar date"},
		{"date in another format", plan(`"plan":"pay_in_4","amount":"100","purchaseDate":"05/10/2026"`), 400, codeInvalidRequest, "calendar date"},
		{"unknown plan", plan(`"plan":"pay_in_3","amount":"100","purchaseDate":"2026-10-05"`), 400, codeUnknownPlan, `"pay_in_3"`},
		{"amount of zero", plan(`"plan":"pay_in_4","amount":"0","purchaseDate":"2026-10-05"`), 422, codeAmountOutOfRange, "too small"},
		{"fewer cents than installments", plan(`"plan":"pay_in_4","amount":"0.03","purchaseDate":"2026-10-05"`), 422, codeAmountOutOfRange, "too small"},
		{"amount over the maximum", plan(`"plan":"pay_in_4","amount":"1000000.01","purchaseDate":"2026-10-05"`), 422, codeAmountOutOfRange, "must not exceed"},
	}

	h := newTestRouter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, h, http.MethodPost, "/api/v1/payment-plans", tt.body)
			wantError(t, rec, tt.wantStatus, tt.wantCode)
			if got := decode[errorResponse](t, rec).Error.Message; !strings.Contains(got, tt.wantMessage) {
				t.Errorf("message = %q, want it to contain %q", got, tt.wantMessage)
			}
		})
	}

	t.Run("requires a JSON content type", func(t *testing.T) {
		rec := doWithContentType(t, h, http.MethodPost, "/api/v1/payment-plans", plan(valid), "text/plain")
		wantError(t, rec, http.StatusUnsupportedMediaType, codeUnsupportedMedia)
	})
}

func TestListCalculations(t *testing.T) {
	t.Run("an empty history is an empty array", func(t *testing.T) {
		rec := do(t, newTestRouter(), http.MethodGet, "/api/v1/calculations", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if got := strings.TrimSpace(rec.Body.String()); got != `{"calculations":[]}` {
			t.Errorf("body = %s, want an empty calculations array", got)
		}
	})

	t.Run("newest first, paginated with a cursor", func(t *testing.T) {
		h := newTestRouter()
		seed(t, h, 5)

		first := decode[listResponse](t, do(t, h, http.MethodGet, "/api/v1/calculations?limit=2", ""))
		if len(first.Calculations) != 2 || first.Calculations[0].ID != 5 || first.Calculations[1].ID != 4 {
			t.Fatalf("first page = %+v, want IDs 5 and 4", first.Calculations)
		}
		if first.NextCursor != "4" {
			t.Fatalf("nextCursor = %q, want 4", first.NextCursor)
		}

		second := decode[listResponse](t, do(t, h, http.MethodGet, "/api/v1/calculations?limit=3&cursor="+first.NextCursor, ""))
		if len(second.Calculations) != 3 || second.Calculations[0].ID != 3 || second.Calculations[2].ID != 1 {
			t.Fatalf("second page = %+v, want IDs 3 to 1", second.Calculations)
		}
		if second.NextCursor != "" {
			t.Errorf("nextCursor = %q on the last page, want none", second.NextCursor)
		}
	})

	t.Run("uses the default page size", func(t *testing.T) {
		h := newTestRouter()
		seed(t, h, defaultPageSize+1)

		got := decode[listResponse](t, do(t, h, http.MethodGet, "/api/v1/calculations", ""))
		if len(got.Calculations) != defaultPageSize || got.NextCursor == "" {
			t.Errorf("got %d records and cursor %q, want %d and a cursor", len(got.Calculations), got.NextCursor, defaultPageSize)
		}
	})

	t.Run("rejects invalid query parameters", func(t *testing.T) {
		h := newTestRouter()
		for _, query := range []string{
			"limit=0", "limit=-1", "limit=101", "limit=abc", "limit=1.5", "limit=99999999999999999999",
			"cursor=abc", "cursor=0", "cursor=-3", "cursor=1.5", "cursor=99999999999999999999",
		} {
			wantError(t, do(t, h, http.MethodGet, "/api/v1/calculations?"+query, ""), 400, codeInvalidRequest)
		}
	})
}

func TestGetCalculation(t *testing.T) {
	h := newTestRouter()
	seed(t, h, 2)

	rec := do(t, h, http.MethodGet, "/api/v1/calculations/2", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := decode[history.Record](t, rec); got.ID != 2 || got.Result.String() != "2" {
		t.Errorf("record = %+v, want the second calculation", got)
	}

	for _, id := range []string{"99", "0", "-1", "abc", "1.0", "99999999999999999999", "1%20"} {
		wantError(t, do(t, h, http.MethodGet, "/api/v1/calculations/"+id, ""), 404, codeNotFound)
	}
}

// brokenRepo simulates a storage outage.
type brokenRepo struct{}

func (brokenRepo) Save(context.Context, history.Record) (history.Record, error) {
	return history.Record{}, errors.New("disk full: /var/lib/secret-path")
}
func (brokenRepo) List(context.Context, int, int64) ([]history.Record, error) {
	return nil, errors.New("connection refused")
}
func (brokenRepo) Get(context.Context, int64) (history.Record, error) {
	return history.Record{}, errors.New("connection refused")
}

func TestStorageFailuresAreHidden(t *testing.T) {
	h := newRouterWith(brokenRepo{}, "")

	requests := []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/calculations", `{"operation":"add","a":"1","b":"2"}`},
		{http.MethodGet, "/api/v1/calculations", ""},
		{http.MethodGet, "/api/v1/calculations/1", ""},
	}
	for _, r := range requests {
		rec := do(t, h, r.method, r.path, r.body)
		wantError(t, rec, 500, codeInternal)
		if strings.Contains(rec.Body.String(), "secret-path") || strings.Contains(rec.Body.String(), "refused") {
			t.Errorf("%s %s leaked internal details: %s", r.method, r.path, rec.Body)
		}
	}
}

// panickingRepo simulates a bug deep inside a request.
type panickingRepo struct{ brokenRepo }

func (panickingRepo) List(context.Context, int, int64) ([]history.Record, error) {
	panic("unexpected bug")
}

func TestPanicsBecomeInternalErrors(t *testing.T) {
	rec := do(t, newRouterWith(panickingRepo{}, ""), http.MethodGet, "/api/v1/calculations", "")

	wantError(t, rec, 500, codeInternal)
	if strings.Contains(rec.Body.String(), "unexpected bug") {
		t.Errorf("response leaked the panic value: %s", rec.Body)
	}
}

func TestOperationsEndpoint(t *testing.T) {
	rec := do(t, newTestRouter(), http.MethodGet, "/api/v1/operations", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	got := decode[operationsResponse](t, rec)
	if len(got.Operations) != len(calculator.Operations()) {
		t.Errorf("got %d operations, want %d", len(got.Operations), len(calculator.Operations()))
	}
}

func TestHealthEndpoint(t *testing.T) {
	rec := do(t, newTestRouter(), http.MethodGet, "/healthz", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := decode[map[string]string](t, rec); got["status"] != "ok" {
		t.Errorf("status = %q, want ok", got["status"])
	}
}

func TestDocs(t *testing.T) {
	h := newTestRouter()

	t.Run("OpenAPI spec documents every operation and route", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/api/v1/openapi.yaml", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		spec := rec.Body.String()
		for _, info := range calculator.Operations() {
			if !strings.Contains(spec, string(info.Name)) {
				t.Errorf("spec does not mention operation %q", info.Name)
			}
		}
		for _, path := range []string{"/api/v1/calculations:", "/api/v1/calculations/{id}:", "/api/v1/payment-plans:", "/api/v1/operations:", "/healthz:"} {
			if !strings.Contains(spec, path) {
				t.Errorf("spec does not document %s", path)
			}
		}
	})

	t.Run("Swagger UI points at the spec", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/docs", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "/api/v1/openapi.yaml") {
			t.Error("Swagger UI page does not reference the spec URL")
		}
	})
}

func TestAPIFallback(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantCode   string
		wantAllow  string
	}{
		{"DELETE on the collection", http.MethodDelete, "/api/v1/calculations", 405, codeUnsupportedMethod, "GET, POST"},
		{"DELETE on a record", http.MethodDelete, "/api/v1/calculations/1", 405, codeUnsupportedMethod, "GET"},
		{"POST on operations", http.MethodPost, "/api/v1/operations", 405, codeUnsupportedMethod, "GET"},
		{"PUT on a record", http.MethodPut, "/api/v1/calculations/1", 405, codeUnsupportedMethod, "GET"},
		{"PATCH on the collection", http.MethodPatch, "/api/v1/calculations", 405, codeUnsupportedMethod, "GET, POST"},
		{"GET on payment plans", http.MethodGet, "/api/v1/payment-plans", 405, codeUnsupportedMethod, "POST"},
		{"path below payment plans", http.MethodPost, "/api/v1/payment-plans/1", 404, codeNotFound, ""},
		{"unknown endpoint", http.MethodGet, "/api/v1/nope", 404, codeNotFound, ""},
		{"collection with a trailing slash", http.MethodGet, "/api/v1/calculations/", 404, codeNotFound, ""},
		{"path below a record", http.MethodGet, "/api/v1/calculations/1/extra", 404, codeNotFound, ""},
		{"POST below a record", http.MethodPost, "/api/v1/calculations/1/extra", 404, codeNotFound, ""},
		{"API root", http.MethodGet, "/api/", 404, codeNotFound, ""},
	}

	h := newTestRouter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, h, tt.method, tt.path, "")
			wantError(t, rec, tt.wantStatus, tt.wantCode)
			if got := rec.Header().Get("Allow"); got != tt.wantAllow {
				t.Errorf("Allow = %q, want %q", got, tt.wantAllow)
			}
		})
	}
}

func TestStaticFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>calculator</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("served when configured", func(t *testing.T) {
		rec := do(t, newRouterWith(memory.New(1), dir), http.MethodGet, "/", "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "calculator") {
			t.Errorf("GET / = %d %q, want the index page", rec.Code, rec.Body)
		}
	})

	t.Run("without them, the root redirects to the docs", func(t *testing.T) {
		rec := do(t, newTestRouter(), http.MethodGet, "/", "")
		if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/docs" {
			t.Errorf("GET / = %d to %q, want a 302 to /docs", rec.Code, rec.Header().Get("Location"))
		}
		if rec := do(t, newTestRouter(), http.MethodGet, "/nope", ""); rec.Code != http.StatusNotFound {
			t.Errorf("GET /nope = %d, want 404", rec.Code)
		}
	})
}

func TestClassify(t *testing.T) {
	tests := []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{calculator.ErrUnknownOperation, 400, codeUnknownOperation},
		{calculator.ErrInvalidOperand, 400, codeInvalidRequest},
		{calculator.ErrDivisionByZero, 422, codeDivisionByZero},
		{calculator.ErrNegativeSqrt, 422, codeNegativeSqrt},
		{calculator.ErrUndefinedResult, 422, codeUndefinedResult},
		{calculator.ErrOutOfRange, 422, codeResultOutOfRange},
		{history.ErrNotFound, 404, codeNotFound},
		{paymentplan.ErrUnknownPlan, 400, codeUnknownPlan},
		{paymentplan.ErrAmountTooSmall, 422, codeAmountOutOfRange},
		{paymentplan.ErrAmountTooLarge, 422, codeAmountOutOfRange},
		{errors.New("boom"), 500, codeInternal},
	}

	for _, tt := range tests {
		status, code := classify(tt.err)
		if status != tt.wantStatus || code != tt.wantCode {
			t.Errorf("classify(%v) = %d, %q; want %d, %q", tt.err, status, code, tt.wantStatus, tt.wantCode)
		}
	}
}
