package main

import (
	"net/http/httptest"
	"testing"
	"time"
)

// TST-PLT-005-01: composing PMS alone must not depend on optional modules or a
// database connection for platform liveness and the module's bootstrap route.
func TestPMSOnlyComposition(t *testing.T) {
	handler := composeHandler(nil, time.Second)
	cases := []struct {
		name, method, path string
		wantCode           int
		wantBody           string
	}{
		{"platform health", "GET", "/healthz", 200, "{\"status\":\"ok\"}\n"},
		{"PMS module", "GET", "/pms/v1/status", 200, "{\"module\":\"pms\",\"status\":\"bootstrap\"}\n"},
		{"PMS unknown route", "GET", "/pms/v1/rates", 404, ""},
		{"PMS method rejection", "POST", "/pms/v1/status", 405, ""},
		{"optional module absent", "GET", "/experiences/v1/status", 404, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantCode)
			}
			if tc.wantBody != "" && w.Body.String() != tc.wantBody {
				t.Fatalf("body = %q, want %q", w.Body.String(), tc.wantBody)
			}
		})
	}
}
