package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// okHandler is the terminal handler wrapped by corsMiddleware. It records that
// it ran and returns 200 so we can distinguish pass-through from short-circuit.
func okHandler(ran *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*ran = true
		w.WriteHeader(http.StatusOK)
	})
}

func TestCorsMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		allowedOrigins []string
		method         string
		requestOrigin  string
		wantACAO       string // expected Access-Control-Allow-Origin ("" => header absent)
		wantVary       bool   // expect Vary: Origin
		wantStatus     int
		wantNextRan    bool
	}{
		{
			name:           "wildcard sets ACAO * and no credentials",
			allowedOrigins: []string{"*"},
			method:         http.MethodGet,
			requestOrigin:  "https://any.example",
			wantACAO:       "*",
			wantVary:       false,
			wantStatus:     http.StatusOK,
			wantNextRan:    true,
		},
		{
			name:           "allow-list matching origin is reflected with Vary",
			allowedOrigins: []string{"https://good.example", "https://ok.example"},
			method:         http.MethodGet,
			requestOrigin:  "https://good.example",
			wantACAO:       "https://good.example",
			wantVary:       true,
			wantStatus:     http.StatusOK,
			wantNextRan:    true,
		},
		{
			name:           "allow-list non-matching origin gets no ACAO and no Vary",
			allowedOrigins: []string{"https://good.example"},
			method:         http.MethodGet,
			requestOrigin:  "https://evil.example",
			wantACAO:       "",
			wantVary:       false,
			wantStatus:     http.StatusOK,
			wantNextRan:    true,
		},
		{
			name:           "OPTIONS preflight short-circuits with 204",
			allowedOrigins: []string{"*"},
			method:         http.MethodOptions,
			requestOrigin:  "https://any.example",
			wantACAO:       "*",
			wantVary:       false,
			wantStatus:     http.StatusNoContent,
			wantNextRan:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var nextRan bool
			h := corsMiddleware(tc.allowedOrigins)(okHandler(&nextRan))

			req := httptest.NewRequest(tc.method, "/api/health/metrics", nil)
			req.Header.Set("Origin", tc.requestOrigin)
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if got := rec.Code; got != tc.wantStatus {
				t.Errorf("status = %d, want %d", got, tc.wantStatus)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tc.wantACAO {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, tc.wantACAO)
			}
			// The spec-invalid credentials combo must never be emitted (no cookies/auth used).
			if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
				t.Errorf("Access-Control-Allow-Credentials = %q, want absent", got)
			}
			gotVary := rec.Header().Get("Vary") == "Origin"
			if gotVary != tc.wantVary {
				t.Errorf("Vary:Origin present = %v, want %v (Vary=%q)", gotVary, tc.wantVary, rec.Header().Get("Vary"))
			}
			if nextRan != tc.wantNextRan {
				t.Errorf("next handler ran = %v, want %v", nextRan, tc.wantNextRan)
			}
		})
	}
}
