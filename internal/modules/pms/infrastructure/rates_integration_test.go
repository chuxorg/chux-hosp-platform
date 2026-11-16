//go:build integration

package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/api"
	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/application"
	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/domain"
	"github.com/chuxorg/chux-hosp-platform/migrations"
	"github.com/jackc/pgx/v5/pgconn"
)

// TST-PMS-001-02 / REQ-PMS-001. All DDL/DML is in the existing verified random child DB.
func TestRatesAndEligibilityAgainstPostgres(t *testing.T) {
	databaseURL, pool := openIsolatedPMSDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	if v, err := migrations.Run(ctx, databaseURL, "up"); err != nil || v != 3 {
		t.Fatalf("rate migration: %d %v", v, err)
	}
	s := application.NewService(NewStore(pool))
	h := api.Handler(s)
	property := func(name, zone string, active bool) domain.Property {
		t.Helper()
		p, err := s.CreateProperty(ctx, domain.Property{Name: name, TimeZone: zone, Active: active})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	roomType := func(p domain.Property, name string, active bool) domain.RoomType {
		t.Helper()
		v, err := s.CreateRoomType(ctx, domain.RoomType{PropertyID: p.ID, Name: name, MaxOccupancy: 2, Active: active})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	room := func(p domain.Property, typ domain.RoomType, number string, active bool) domain.Room {
		t.Helper()
		v, err := s.CreateRoom(ctx, domain.Room{PropertyID: p.ID, RoomTypeID: typ.ID, Number: number, Active: active})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	p := property("Synthetic Hotel", "Asia/Bangkok", false)
	typ := roomType(p, "Standard", false)
	r1, r2 := room(p, typ, "101", true), room(p, typ, "102", true)
	unrated := roomType(p, "Unrated", true)
	room(p, unrated, "201", true)
	other := property("Other Hotel", "America/New_York", true)
	otherType := roomType(other, "Standard", true)
	room(other, otherType, "101", true)
	ratePath := func(pID, tID int64) string { return fmt.Sprintf("/properties/%d/room-types/%d/rate", pID, tID) }
	rateURL := ratePath(p.ID, typ.ID)
	set := func(path, body string, want int) domain.RoomTypeRate {
		t.Helper()
		code, b := jsonRequest(h, "PUT", path, body)
		assertStatus(t, code, want, b)
		if code < 300 {
			return parseResponse[domain.RoomTypeRate](t, b)
		}
		return domain.RoomTypeRate{}
	}
	get := func(path string, want int) []byte {
		t.Helper()
		code, b := jsonRequest(h, "GET", path, "")
		assertStatus(t, code, want, b)
		return b
	}
	search := func(pID int64, in, out string) domain.Eligibility {
		t.Helper()
		path := fmt.Sprintf("/properties/%d/room-eligibility?check_in=%s&check_out=%s", pID, in, out)
		return parseResponse[domain.Eligibility](t, get(path, 200))
	}
	assertRooms := func(got domain.Eligibility, ids ...int64) {
		t.Helper()
		if got.ReservationConflictsChecked || got.SearchMode != "inventory_eligibility" || got.Rooms == nil {
			t.Fatalf("missing eligibility limitation: %+v", got)
		}
		actual := make([]int64, 0)
		for _, v := range got.Rooms {
			actual = append(actual, v.Room.ID)
			if v.Room.PropertyID != got.PropertyID || v.NightlyRate.PropertyID != got.PropertyID || v.NightlyRate.RoomTypeID != v.Room.RoomTypeID {
				t.Fatal("cross-property/type result")
			}
		}
		if len(ids) == 0 {
			ids = []int64{}
		}
		if !reflect.DeepEqual(actual, ids) {
			t.Fatalf("rooms %v, want %v", actual, ids)
		}
	}

	t.Run("configure and read under inactive parents", func(t *testing.T) {
		rate := set(rateURL, `{"amount_minor":12345,"currency":"THB","active":true}`, 201)
		if rate.PropertyID != p.ID || rate.RoomTypeID != typ.ID || rate.AmountMinor != 12345 || rate.Currency != "THB" || !rate.Active {
			t.Fatalf("bad rate: %+v", rate)
		}
		if got := parseResponse[domain.RoomTypeRate](t, get(rateURL, 200)); got != rate {
			t.Fatal("rate roundtrip mismatch")
		}
		assertRooms(search(p.ID, "2026-11-15", "2026-11-16"))
	})
	t.Run("positive integer and exact currency validation is atomic", func(t *testing.T) {
		before := parseResponse[domain.RoomTypeRate](t, get(rateURL, 200))
		for _, body := range []string{
			`{"amount_minor":0,"currency":"THB","active":false}`,
			`{"amount_minor":-1,"currency":"THB","active":false}`,
			`{"amount_minor":1.5,"currency":"THB","active":false}`,
			`{"amount_minor":1.0,"currency":"THB","active":false}`,
			`{"amount_minor":1e3,"currency":"THB","active":false}`,
			`{"amount_minor":"100","currency":"THB","active":false}`,
			`{"amount_minor":9223372036854775808,"currency":"THB","active":false}`,
			`{"amount_minor":100,"currency":"USD","active":false}`,
			`{"amount_minor":100,"currency":"thb","active":false}`,
			`{"amount_minor":100,"currency":"THB"}`,
		} {
			set(rateURL, body, 400)
			if got := parseResponse[domain.RoomTypeRate](t, get(rateURL, 200)); got != before {
				t.Fatal("rejected write changed persisted rate")
			}
		}
		max := set(rateURL, `{"amount_minor":9223372036854775807,"currency":"THB","active":false}`, 200)
		if max.AmountMinor != 9223372036854775807 || max.Active {
			t.Fatal("int64 precision/deactivation lost")
		}
		set(rateURL, `{"amount_minor":12345,"currency":"THB","active":true}`, 200)
	})
	t.Run("unknown and cross-property references", func(t *testing.T) {
		body := `{"amount_minor":100,"currency":"THB","active":true}`
		set(ratePath(p.ID, 999999), body, 422)
		set(ratePath(999999, typ.ID), body, 422)
		// Cover the conflict-then-update path and the insert FK path separately.
		set(ratePath(other.ID, typ.ID), body, 422)
		set(ratePath(p.ID, otherType.ID), body, 422)
		get(ratePath(other.ID, typ.ID), 404)
		get(ratePath(p.ID, otherType.ID), 404)
		get(ratePath(p.ID, unrated.ID), 404)
		get("/properties/999999/room-eligibility?check_in=2026-01-01&check_out=2026-01-02", 404)
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM pms_room_type_rates").Scan(&count); err != nil || count != 1 {
			t.Fatalf("invalid references persisted: %d %v", count, err)
		}
	})
	t.Run("active hierarchy and multiple physical rooms", func(t *testing.T) {
		p.Active = true
		if _, err := s.UpdateProperty(ctx, p); err != nil {
			t.Fatal(err)
		}
		assertRooms(search(p.ID, "2026-11-15", "2026-11-16")) // RoomType still inactive.
		typ.Active = true
		if _, err := s.UpdateRoomType(ctx, typ); err != nil {
			t.Fatal(err)
		}
		got := search(p.ID, "2026-11-15", "2026-11-16")
		assertRooms(got, r1.ID, r2.ID)
		if got.TimeZone != "Asia/Bangkok" || got.Rooms[0].NightlyRate.AmountMinor != 12345 {
			t.Fatal("zone/rate missing")
		}
		r1.Active = false
		if _, err := s.UpdateRoom(ctx, r1); err != nil {
			t.Fatal(err)
		}
		assertRooms(search(p.ID, "2026-11-15", "2026-11-16"), r2.ID)
		r1.Active = true
		if _, err := s.UpdateRoom(ctx, r1); err != nil {
			t.Fatal(err)
		}
		set(rateURL, `{"amount_minor":12345,"currency":"THB","active":false}`, 200)
		assertRooms(search(p.ID, "2026-11-15", "2026-11-16"))
		set(rateURL, `{"amount_minor":12345,"currency":"THB","active":true}`, 200)
		// Missing rate on 'Unrated' continues to exclude its physical room.
		assertRooms(search(p.ID, "2026-11-15", "2026-11-16"), r1.ID, r2.ID)
		set(ratePath(other.ID, otherType.ID), `{"amount_minor":777,"currency":"THB","active":true}`, 201)
		otherResult := search(other.ID, "2026-03-07", "2026-03-09")
		if otherResult.TimeZone != "America/New_York" || len(otherResult.Rooms) != 1 || otherResult.Rooms[0].NightlyRate.AmountMinor != 777 {
			t.Fatal("property isolation failed")
		}
		assertRooms(search(p.ID, "2026-11-15", "2026-11-16"), r1.ID, r2.ID)
	})
	t.Run("hotel dates boundaries and explicit date independent eligibility", func(t *testing.T) {
		for _, zone := range []string{"Asia/Bangkok", "America/New_York", "Pacific/Auckland"} {
			p.TimeZone = zone
			if _, err := s.UpdateProperty(ctx, p); err != nil {
				t.Fatal(err)
			}
			for _, dates := range [][2]string{{"2026-03-07", "2026-03-08"}, {"2026-03-08", "2026-03-09"}, {"2026-10-31", "2026-11-02"}, {"2028-02-29", "2028-03-01"}, {"2000-01-01", "2000-01-02"}} {
				got := search(p.ID, dates[0], dates[1])
				assertRooms(got, r1.ID, r2.ID)
				if got.TimeZone != zone || got.CheckIn != dates[0] || got.CheckOut != dates[1] {
					t.Fatal("hotel dates shifted by time zone")
				}
			}
		}
		for _, q := range []string{"check_in=2026-01-01&check_out=2026-01-01", "check_in=2026-01-02&check_out=2026-01-01", "check_in=2026-02-29&check_out=2026-03-01", "check_in=2026-01-01T00:00:00Z&check_out=2026-01-02", "check_in=2026-01-01", "check_in=2026-01-01&check_in=2026-01-02&check_out=2026-01-03"} {
			get(fmt.Sprintf("/properties/%d/room-eligibility?%s", p.ID, q), 400)
		}
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM pms_room_type_rates").Scan(&count); err != nil || count != 2 {
			t.Fatal("read-only search changed rates")
		}
	})
	t.Run("database constraints protect direct writes atomically", func(t *testing.T) {
		before := parseResponse[domain.RoomTypeRate](t, get(rateURL, 200))
		for _, tt := range []struct{ sql, code string }{
			{"UPDATE pms_room_type_rates SET amount_minor=0 WHERE room_type_id=$1", "23514"},
			{"UPDATE pms_room_type_rates SET currency='USD' WHERE room_type_id=$1", "23514"},
			{"UPDATE pms_room_type_rates SET is_active=NULL WHERE room_type_id=$1", "23502"},
			{"UPDATE pms_room_type_rates SET property_id=999999 WHERE room_type_id=$1", "23503"},
		} {
			_, err := pool.Exec(ctx, tt.sql, typ.ID)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != tt.code {
				t.Fatalf("wrong constraint failure: %v", err)
			}
			if got := parseResponse[domain.RoomTypeRate](t, get(rateURL, 200)); got != before {
				t.Fatal("constraint failure changed rate")
			}
		}
	})
	t.Run("concurrent set has one current rate and complete values", func(t *testing.T) {
		path := ratePath(p.ID, unrated.ID)
		bodies := []string{`{"amount_minor":100,"currency":"THB","active":false}`, `{"amount_minor":200,"currency":"THB","active":true}`}
		start := make(chan struct{})
		codes := make([]int, 2)
		var wg sync.WaitGroup
		for i := range codes {
			wg.Add(1)
			go func(i int) { defer wg.Done(); <-start; codes[i], _ = jsonRequest(h, http.MethodPut, path, bodies[i]) }(i)
		}
		close(start)
		wg.Wait()
		if !((codes[0] == 201 && codes[1] == 200) || (codes[0] == 200 && codes[1] == 201)) {
			t.Fatalf("concurrent statuses: %v", codes)
		}
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM pms_room_type_rates WHERE room_type_id=$1", unrated.ID).Scan(&count); err != nil || count != 1 {
			t.Fatal("duplicate rate rows")
		}
		got := parseResponse[domain.RoomTypeRate](t, get(path, 200))
		if !((got.AmountMinor == 100 && !got.Active) || (got.AmountMinor == 200 && got.Active)) {
			t.Fatal("partial concurrent replacement")
		}
	})
	t.Run("rate migration rollback preserves earlier schema and records", func(t *testing.T) {
		var properties, rooms int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM pms_properties").Scan(&properties); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM pms_rooms").Scan(&rooms); err != nil {
			t.Fatal(err)
		}
		if v, err := migrations.Run(ctx, databaseURL, "down"); err != nil || v != 2 {
			t.Fatalf("rate rollback: %d %v", v, err)
		}
		var absent bool
		if err := pool.QueryRow(ctx, "SELECT to_regclass('pms_room_type_rates') IS NULL").Scan(&absent); err != nil || !absent {
			t.Fatal("rate table remains after rollback")
		}
		var pCount, rCount int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM pms_properties").Scan(&pCount); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM pms_rooms").Scan(&rCount); err != nil {
			t.Fatal(err)
		}
		if pCount != properties || rCount != rooms {
			t.Fatal("rate rollback destroyed earlier configuration")
		}
		if v, err := migrations.Run(ctx, databaseURL, "up"); err != nil || v != 3 {
			t.Fatalf("rate reapply: %d %v", v, err)
		}
		get(rateURL, 404) // Rolled-back rate data is intentionally not resurrected.
	})
	t.Run("persistence failure sanitized", func(t *testing.T) {
		pool.Close()
		code, b := jsonRequest(h, "PUT", rateURL, `{"amount_minor":100,"currency":"THB","active":true}`)
		if code != 500 || strings.Contains(string(b), "closed") || string(b) != "{\"error\":\"internal error\"}\n" {
			t.Fatalf("unsanitized failure: %d %s", code, b)
		}
	})
}
