package api

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/application"
	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/domain"
)

type rateStore struct {
	application.Store
	err     error
	created bool
}

func (s rateStore) SetRoomTypeRate(_ context.Context, r domain.RoomTypeRate) (domain.RoomTypeRate, bool, error) {
	return r, s.created, s.err
}
func (s rateStore) GetRoomTypeRate(_ context.Context, p, id int64) (domain.RoomTypeRate, error) {
	return domain.RoomTypeRate{PropertyID: p, RoomTypeID: id, AmountMinor: 100, Currency: "THB", Active: false}, s.err
}

// TST-PMS-001-02: reject representations before persistence and sanitize infrastructure errors.
func TestRateHTTPValidationAndErrors(t *testing.T) {
	path := "/properties/1/room-types/2/rate"
	h := Handler(application.NewService(rateStore{created: true}))
	for _, body := range []string{
		`{"amount_minor":0,"currency":"THB","active":true}`,
		`{"amount_minor":-1,"currency":"THB","active":true}`,
		`{"amount_minor":1.5,"currency":"THB","active":true}`,
		`{"amount_minor":1.0,"currency":"THB","active":true}`,
		`{"amount_minor":1e2,"currency":"THB","active":true}`,
		`{"amount_minor":"100","currency":"THB","active":true}`,
		`{"amount_minor":9223372036854775808,"currency":"THB","active":true}`,
		`{"amount_minor":null,"currency":"THB","active":true}`,
		`{"currency":"THB","active":true}`,
		`{"amount_minor":100,"currency":"USD","active":true}`,
		`{"amount_minor":100,"currency":"thb","active":true}`,
		`{"amount_minor":100,"active":true}`,
		`{"amount_minor":100,"currency":"THB"}`,
		`{"amount_minor":100,"currency":"THB","active":null}`,
		`{"amount_minor":100,"currency":"THB","active":"true"}`,
		`{"amount_minor":100,"currency":"THB","active":true,"unknown":1}`,
		`{"amount_minor":100,"currency":"THB","active":true} {}`,
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatalf("accepted invalid body %s: %d %s", body, w.Code, w.Body.String())
		}
	}
	for _, created := range []bool{true, false} {
		h = Handler(application.NewService(rateStore{created: created}))
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", path, strings.NewReader(`{"amount_minor":9223372036854775807,"currency":"THB","active":true}`))
		r.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(w, r)
		want := 200
		if created {
			want = 201
		}
		if w.Code != want {
			t.Fatalf("create/replace status: %d", w.Code)
		}
		if created && w.Header().Get("Location") != path {
			t.Fatal("missing create Location")
		}
	}
	for _, tt := range []struct {
		method string
		err    error
		status int
	}{{"PUT", application.ErrReference, 422}, {"GET", application.ErrNotFound, 404}, {"PUT", application.ErrConflict, 409}, {"GET", errors.New("postgres secret details"), 500}, {"PUT", errors.New("postgres secret details"), 500}} {
		h = Handler(application.NewService(rateStore{err: tt.err}))
		w := httptest.NewRecorder()
		r := httptest.NewRequest(tt.method, path, strings.NewReader(`{"amount_minor":100,"currency":"THB","active":true}`))
		r.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(w, r)
		if w.Code != tt.status || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("error mapping/redaction: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestEligibilityHTTPRejectsAmbiguousQuery(t *testing.T) {
	// Parsing failures must never reach the nil service/store.
	h := Handler(nil)
	for _, path := range []string{
		"/properties/nope/room-eligibility?check_in=2026-01-01&check_out=2026-01-02",
		"/properties/1/room-eligibility",
		"/properties/1/room-eligibility?check_in=2026-01-01",
		"/properties/1/room-eligibility?check_in=2026-01-01&check_in=2026-02-01&check_out=2026-02-02",
		"/properties/1/room-eligibility?check_in=2026-01-01&check_out=2026-01-02&guests=2",
		"/properties/1/room-eligibility?check_in=%GG&check_out=2026-01-02",
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 400 {
			t.Fatalf("ambiguous query accepted: %s %d", path, w.Code)
		}
	}
}
