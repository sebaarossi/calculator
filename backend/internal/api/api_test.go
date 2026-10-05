package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebaarossi/calculator/backend/internal/calculator"
)

func newTestRouter(staticDir string) http.Handler {
	return NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), staticDir)
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
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

func TestCalculateSuccess(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"add", `{"operation":"add","a":2,"b":3}`, `{"operation":"add","a":2,"b":3,"result":5}`},
		{"subtract", `{"operation":"subtract","a":2,"b":3}`, `{"operation":"subtract","a":2,"b":3,"result":-1}`},
		{"multiply", `{"operation":"multiply","a":1.5,"b":4}`, `{"operation":"multiply","a":1.5,"b":4,"result":6}`},
		{"divide", `{"operation":"divide","a":9,"b":2}`, `{"operation":"divide","a":9,"b":2,"result":4.5}`},
		{"power", `{"operation":"power","a":2,"b":8}`, `{"operation":"power","a":2,"b":8,"result":256}`},
		{"percentage", `{"operation":"percentage","a":20,"b":150}`, `{"operation":"percentage","a":20,"b":150,"result":30}`},
		{"sqrt", `{"operation":"sqrt","a":81}`, `{"operation":"sqrt","a":81,"result":9}`},
		{"sqrt ignores b", `{"operation":"sqrt","a":81,"b":7}`, `{"operation":"sqrt","a":81,"result":9}`},
		{"explicit zero operands", `{"operation":"add","a":0,"b":0}`, `{"operation":"add","a":0,"b":0,"result":0}`},
	}

	h := newTestRouter("")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, h, http.MethodPost, "/api/v1/calculate", tt.body)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.want {
				t.Errorf("body = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestCalculateErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{"empty body", ``, 400, codeInvalidRequest},
		{"malformed JSON", `{"operation":`, 400, codeInvalidRequest},
		{"not an object", `[1,2]`, 400, codeInvalidRequest},
		{"trailing data", `{"operation":"add","a":1,"b":2}{}`, 400, codeInvalidRequest},
		{"unknown field", `{"operation":"add","a":1,"b":2,"c":3}`, 400, codeInvalidRequest},
		{"operand is a string", `{"operation":"add","a":"1","b":2}`, 400, codeInvalidRequest},
		{"operand out of float range", `{"operation":"add","a":1e999,"b":2}`, 400, codeInvalidRequest},
		{"missing operation", `{"a":1,"b":2}`, 400, codeInvalidRequest},
		{"missing a", `{"operation":"add","b":2}`, 400, codeInvalidRequest},
		{"null a", `{"operation":"add","a":null,"b":2}`, 400, codeInvalidRequest},
		{"missing b", `{"operation":"add","a":1}`, 400, codeInvalidRequest},
		{"unknown operation", `{"operation":"modulo","a":1,"b":2}`, 400, codeUnknownOperation},
		{"division by zero", `{"operation":"divide","a":1,"b":0}`, 422, codeDivisionByZero},
		{"negative square root", `{"operation":"sqrt","a":-1}`, 422, codeNegativeSqrt},
		{"undefined result", `{"operation":"power","a":-8,"b":0.5}`, 422, codeUndefinedResult},
		{"result out of range", `{"operation":"multiply","a":1e308,"b":10}`, 422, codeResultOutOfRange},
		{"body too large", `{"operation":"add","a":1,"b":` + strings.Repeat("1", maxBodyBytes) + `}`, 413, codeRequestTooLarge},
	}

	h := newTestRouter("")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, h, http.MethodPost, "/api/v1/calculate", tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tt.wantStatus, rec.Body)
			}
			got := decode[errorResponse](t, rec)
			if got.Error.Code != tt.wantCode {
				t.Errorf("error code = %q, want %q", got.Error.Code, tt.wantCode)
			}
			if got.Error.Message == "" {
				t.Error("error message is empty")
			}
		})
	}
}

func TestOperationsEndpoint(t *testing.T) {
	rec := do(t, newTestRouter(""), http.MethodGet, "/api/v1/operations", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	got := decode[operationsResponse](t, rec)
	if len(got.Operations) != len(calculator.Operations()) {
		t.Errorf("got %d operations, want %d", len(got.Operations), len(calculator.Operations()))
	}
}

func TestHealthEndpoint(t *testing.T) {
	rec := do(t, newTestRouter(""), http.MethodGet, "/healthz", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := decode[map[string]string](t, rec); got["status"] != "ok" {
		t.Errorf("status = %q, want ok", got["status"])
	}
}

func TestDocs(t *testing.T) {
	h := newTestRouter("")

	t.Run("OpenAPI spec documents every operation", func(t *testing.T) {
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
		wantAllow  string
	}{
		{"GET on calculate", http.MethodGet, "/api/v1/calculate", 405, "POST"},
		{"POST on operations", http.MethodPost, "/api/v1/operations", 405, "GET"},
		{"unknown endpoint", http.MethodGet, "/api/v1/nope", 404, ""},
	}

	h := newTestRouter("")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, h, tt.method, tt.path, "")
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := rec.Header().Get("Allow"); got != tt.wantAllow {
				t.Errorf("Allow = %q, want %q", got, tt.wantAllow)
			}
			decode[errorResponse](t, rec)
		})
	}
}

func TestStaticFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>calculator</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("served when configured", func(t *testing.T) {
		rec := do(t, newTestRouter(dir), http.MethodGet, "/", "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "calculator") {
			t.Errorf("GET / = %d %q, want the index page", rec.Code, rec.Body)
		}
	})

	t.Run("not served by default", func(t *testing.T) {
		if rec := do(t, newTestRouter(""), http.MethodGet, "/", ""); rec.Code != http.StatusNotFound {
			t.Errorf("GET / = %d, want 404", rec.Code)
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
		{errors.New("boom"), 500, codeInternal},
	}

	for _, tt := range tests {
		status, code := classify(tt.err)
		if status != tt.wantStatus || code != tt.wantCode {
			t.Errorf("classify(%v) = %d, %q; want %d, %q", tt.err, status, code, tt.wantStatus, tt.wantCode)
		}
	}
}
