package webui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func testdataPath(parts ...string) string {
	return filepath.Join(append([]string{"..", "..", "testdata"}, parts...)...)
}

func TestHandlerServesUI(t *testing.T) {
	handler, err := NewHandler(testdataPath("web"))
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	tests := []struct {
		name      string
		method    string
		path      string
		want      int
		body      string
		cachePart string
	}{
		{name: "index", method: http.MethodGet, path: "/", want: http.StatusOK, body: "test index", cachePart: "no-cache"},
		{name: "spa fallback", method: http.MethodGet, path: "/status", want: http.StatusOK, body: "test index", cachePart: "no-cache"},
		{name: "asset", method: http.MethodGet, path: "/_app/app.js", want: http.StatusOK, body: "test asset", cachePart: "no-cache"},
		{name: "immutable asset", method: http.MethodGet, path: "/_app/immutable/chunk.js", want: http.StatusOK, body: "immutable asset", cachePart: "immutable"},
		{name: "missing asset", method: http.MethodGet, path: "/_app/missing.js", want: http.StatusNotFound},
		{name: "missing file", method: http.MethodGet, path: "/missing.txt", want: http.StatusNotFound},
		{name: "method not allowed", method: http.MethodPost, path: "/", want: http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.want, rec.Body.String())
			}
			if tt.body != "" && !strings.Contains(rec.Body.String(), tt.body) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tt.body)
			}
			if tt.cachePart != "" && !strings.Contains(rec.Header().Get("Cache-Control"), tt.cachePart) {
				t.Errorf("Cache-Control = %q, want it to contain %q", rec.Header().Get("Cache-Control"), tt.cachePart)
			}
		})
	}
}

func TestNewHandlerRequiresIndex(t *testing.T) {
	if _, err := NewHandler(t.TempDir()); err == nil {
		t.Fatal("NewHandler() error = nil, want missing index error")
	}
}

func TestWithAPI(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	web := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "web")
	})
	handler := WithAPI(api, web)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("API status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	req = httptest.NewRequest(http.MethodGet, "/status", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "web" {
		t.Errorf("web response = (%d, %q), want (200, %q)", rec.Code, rec.Body.String(), "web")
	}
}
