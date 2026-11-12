package timetower

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminOnly(t *testing.T) {
	server := &Server{
		adminPassword:    "admin-pass",
		enforcerPassword: "enforcer-pass",
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := server.AdminOnly(next)

	tests := []struct {
		name      string
		authorize func(*http.Request)
		want      int
		challenge bool
	}{
		{name: "missing credentials", authorize: func(*http.Request) {}, want: http.StatusUnauthorized, challenge: true},
		{name: "wrong password", authorize: func(r *http.Request) {
			r.SetBasicAuth(adminUsername, "wrong")
		}, want: http.StatusUnauthorized, challenge: true},
		{name: "enforcer credentials", authorize: func(r *http.Request) {
			r.SetBasicAuth(enforcerUsername, "enforcer-pass")
		}, want: http.StatusForbidden},
		{name: "admin credentials", authorize: func(r *http.Request) {
			r.SetBasicAuth(adminUsername, "admin-pass")
		}, want: http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			tt.authorize(req)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.want, rec.Body.String())
			}
			challenge := strings.Contains(rec.Header().Get("WWW-Authenticate"), "Basic")
			if challenge != tt.challenge {
				t.Errorf("Basic challenge = %v, want %v", challenge, tt.challenge)
			}
			if tt.want == http.StatusNoContent && rec.Header().Get("Vary") != "Authorization" {
				t.Errorf("Vary = %q, want Authorization", rec.Header().Get("Vary"))
			}
		})
	}
}
