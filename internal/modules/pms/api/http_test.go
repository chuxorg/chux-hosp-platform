package api

import (
	"net/http/httptest"
	"testing"
)

func TestModuleHandler(t *testing.T) {
	handler := Handler(nil)
	cases := []struct {
		name, method, path string
		wantCode           int
		wantBody           string
	}{
		{"bootstrap route", "GET", "/status", 200, "{\"module\":\"pms\",\"status\":\"bootstrap\"}\n"},
		{"unsupported method", "POST", "/status", 405, ""},
		{"unimplemented business route", "GET", "/properties", 404, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != tc.wantCode {
				t.Fatalf("status = %d; want %d", w.Code, tc.wantCode)
			}
			if tc.wantBody != "" && w.Body.String() != tc.wantBody {
				t.Fatalf("body = %q; want %q", w.Body.String(), tc.wantBody)
			}
			if tc.wantCode == 200 {
				if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
					t.Fatal("module diagnostic must not be cached and must use JSON")
				}
			}
		})
	}
}
