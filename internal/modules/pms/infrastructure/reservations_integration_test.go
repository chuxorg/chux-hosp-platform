//go:build integration

package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/api"
	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/application"
	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/domain"
	"github.com/chuxorg/chux-hosp-platform/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type bookingFixture struct {
	t       *testing.T
	ctx     context.Context
	url     string
	pool    *pgxpool.Pool
	store   *Store
	catalog *application.Service
	service *application.ReservationService
	p       domain.Property
	typ     domain.RoomType
	rooms   []domain.Room
	input   application.ConfirmReservation
	h       http.Handler
	key     atomic.Int64
}

func newBookingFixture(t *testing.T, n int) *bookingFixture {
	t.Helper()
	url, pool := openIsolatedPMSDatabase(t)
	ctx := context.Background()
	latest, err := migrations.Run(ctx, url, "up")
	if err != nil || latest != 5 {
		t.Fatalf("migration %d: %v", latest, err)
	}
	f := &bookingFixture{t: t, ctx: ctx, url: url, pool: pool, store: NewStore(pool)}
	f.catalog = application.NewService(f.store)
	f.service = application.NewReservationService(f.store)
	f.h = api.Handler(f.catalog, f.service)
	f.p, err = f.catalog.CreateProperty(ctx, domain.Property{Name: "Synthetic hotel", TimeZone: "Asia/Bangkok", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	f.typ = f.addType("Synthetic neutral", n)
	day := time.Now().UTC().AddDate(0, 0, 40)
	f.input = application.ConfirmReservation{RoomTypeID: f.typ.ID, GuestRef: "SYN-GUEST-1", OperatorRef: "SYN-OP-1", CheckIn: day.Format("2006-01-02"), CheckOut: day.AddDate(0, 0, 2).Format("2006-01-02"), ExpectedNightlyRateMinor: 12500, Currency: "THB"}
	return f
}
func (f *bookingFixture) addType(name string, n int) domain.RoomType {
	f.t.Helper()
	v, e := f.catalog.CreateRoomType(f.ctx, domain.RoomType{PropertyID: f.p.ID, Name: name, MaxOccupancy: 2, Active: true})
	if e != nil {
		f.t.Fatal(e)
	}
	for i := 0; i < n; i++ {
		r, e := f.catalog.CreateRoom(f.ctx, domain.Room{PropertyID: f.p.ID, RoomTypeID: v.ID, Number: fmt.Sprintf("SYN-%d-%d", v.ID, i), Active: true})
		if e != nil {
			f.t.Fatal(e)
		}
		f.rooms = append(f.rooms, r)
	}
	_, _, e = f.catalog.SetRoomTypeRate(f.ctx, domain.RoomTypeRate{PropertyID: f.p.ID, RoomTypeID: v.ID, AmountMinor: 12500, Currency: "THB", Active: true})
	if e != nil {
		f.t.Fatal(e)
	}
	return v
}
func (f *bookingFixture) enroll(t int64, enabled bool) {
	f.t.Helper()
	_, err := f.service.SetPool(f.ctx, domain.ReservationPool{PropertyID: f.p.ID, RoomTypeID: t, Scope: domain.PoolScope, Enabled: enabled, AttestationRef: "SYN-NEUTRAL-ATTEST-1", OperatorRef: "SYN-OP-1"})
	if err != nil {
		f.t.Fatal(err)
	}
}
func (f *bookingFixture) nextKey() string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", f.key.Add(1))
}
func (f *bookingFixture) confirm(in application.ConfirmReservation) (domain.StayReservation, bool, error) {
	return f.service.Confirm(f.ctx, f.p.ID, f.nextKey(), in)
}
func (f *bookingFixture) counts() (int, int) {
	f.t.Helper()
	var r, c int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM pms_stay_reservations").Scan(&r); e != nil {
		f.t.Fatal(e)
	}
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM pms_reservation_inventory_commitments").Scan(&c); e != nil {
		f.t.Fatal(e)
	}
	return r, c
}
func (f *bookingFixture) request(method, path, key, body string) (int, []byte, http.Header) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, req)
	return w.Code, w.Body.Bytes(), w.Header()
}
func (f *bookingFixture) body(in application.ConfirmReservation) string {
	b, _ := json.Marshal(in)
	return string(b)
}

// REQ-PMS-002 / TST-PMS-001-03: complete confirmation/readback, lost response
// replay, historical money, API isolation, dates and all-or-nothing inventory.
func TestPooledReservationContract(t *testing.T) {
	f := newBookingFixture(t, 1)
	in := f.input
	if _, _, e := f.confirm(in); !errors.Is(e, application.ErrPoolRequired) {
		t.Fatalf("unenrolled: %v", e)
	}
	// Empty migration rollback/reapply must preserve previous catalog/rates.
	for _, version := range []int64{4, 3} {
		if v, e := migrations.Run(f.ctx, f.url, "down"); e != nil || v != version {
			t.Fatalf("empty down: %d %v", v, e)
		}
	}
	if v, e := migrations.Run(f.ctx, f.url, "up"); e != nil || v != 5 {
		t.Fatalf("reapply: %d %v", v, e)
	}
	f.enroll(f.typ.ID, true)
	path := fmt.Sprintf("/properties/%d/stay-reservations", f.p.ID)
	key := f.nextKey()
	code, b, headers := f.request("POST", path, key, f.body(in))
	assertStatus(t, code, 201, b)
	r := parseResponse[domain.StayReservation](t, b)
	if r.ID == 0 || r.Status != "confirmed" || r.BookingTerms.AccommodationBaseSubtotalMinor != 25000 || r.BookingTerms.NightCount != 2 || r.BookingTerms.Version != 1 || r.TimeZone != "Asia/Bangkok" || r.CapacityAtConfirmation != 1 || r.ConfirmedAt.IsZero() || strings.Contains(string(b), "room_id") || strings.Contains(string(b), "payable") || headers.Get("Location") == "" {
		t.Fatalf("snapshot: %s", b)
	}
	if count, nights := f.counts(); count != 1 || nights != 2 {
		t.Fatalf("incomplete %d %d", count, nights)
	}
	code, read, _ := f.request("GET", fmt.Sprintf("%s/%d", path, r.ID), "", "")
	assertStatus(t, code, 200, read)
	if !bytes.Equal(read, b) {
		t.Fatal("lookup changed snapshot")
	}
	code, read, _ = f.request("GET", fmt.Sprintf("/properties/%d/stay-reservations/%d", f.p.ID+1, r.ID), "", "")
	assertStatus(t, code, 404, read)
	_, _, err := f.catalog.SetRoomTypeRate(f.ctx, domain.RoomTypeRate{PropertyID: f.p.ID, RoomTypeID: f.typ.ID, AmountMinor: 15000, Currency: "THB", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	f.enroll(f.typ.ID, false) // Existing original replay precedes closed-pool/rate checks.
	code, again, headers := f.request("POST", path, key, f.body(in))
	assertStatus(t, code, 200, again)
	if !bytes.Equal(again, b) || headers.Get("Idempotency-Replayed") != "true" {
		t.Fatal("lost-response replay changed")
	}
	in.GuestRef = "SYN-OTHER"
	code, b, _ = f.request("POST", path, key, f.body(in))
	assertStatus(t, code, 409, b)
	f.enroll(f.typ.ID, true)
	in = f.input
	in.ExpectedNightlyRateMinor = 15000
	if _, _, e := f.confirm(in); !errors.Is(e, application.ErrInventoryExhausted) {
		t.Fatalf("full: %v", e)
	}
	adjacent := in
	adjacent.CheckIn = in.CheckOut
	end, _ := time.Parse("2006-01-02", adjacent.CheckIn)
	adjacent.CheckOut = end.AddDate(0, 0, 1).Format("2006-01-02")
	if _, _, e := f.confirm(adjacent); e != nil {
		t.Fatalf("adjacent: %v", e)
	}
	partial := in
	start, _ := time.Parse("2006-01-02", in.CheckIn)
	partial.CheckIn = start.AddDate(0, 0, -1).Format("2006-01-02")
	beforeR, beforeC := f.counts()
	failedKey := f.nextKey()
	if _, _, e := f.service.Confirm(f.ctx, f.p.ID, failedKey, partial); !errors.Is(e, application.ErrInventoryExhausted) {
		t.Fatalf("interior full: %v", e)
	}
	if a, c := f.counts(); a != beforeR || c != beforeC {
		t.Fatal("partial request wrote rows/key")
	}
	partial.CheckOut = partial.CheckIn
	partial.CheckIn = start.AddDate(0, 0, -2).Format("2006-01-02")
	if _, _, e := f.service.Confirm(f.ctx, f.p.ID, failedKey, partial); e != nil {
		t.Fatalf("failed key retained: %v", e)
	}
	if v, e := migrations.Run(f.ctx, f.url, "down"); e != nil || v != 4 {
		t.Fatalf("no-cancellation rollback: %d %v", v, e)
	}
	if _, e := migrations.Run(f.ctx, f.url, "down"); e == nil {
		t.Fatal("populated rollback destroyed history")
	}
	if a, c := f.counts(); a != beforeR+1 || c != beforeC+1 {
		t.Fatal("rollback lost data")
	}
}

func TestReservationNegativeAPIAndMoney(t *testing.T) {
	f := newBookingFixture(t, 2)
	f.enroll(f.typ.ID, true)
	path := fmt.Sprintf("/properties/%d/stay-reservations", f.p.ID)
	for _, body := range []string{"{}", "null", f.body(f.input) + " {}", strings.Replace(f.body(f.input), "12500", "1.2", 1), strings.Replace(f.body(f.input), "12500", "9223372036854775808", 1), strings.TrimSuffix(f.body(f.input), "}") + `,"room_id":1}`, strings.Replace(f.body(f.input), "THB", "USD", 1), strings.Replace(f.body(f.input), f.input.CheckIn, "2028-02-30", 1)} {
		code, b, _ := f.request("POST", path, f.nextKey(), body)
		assertStatus(t, code, 400, b)
	}
	for _, key := range []string{"", "bad-key"} {
		code, b, _ := f.request("POST", path, key, f.body(f.input))
		assertStatus(t, code, 400, b)
	}
	long := f.input
	start, _ := time.Parse("2006-01-02", long.CheckIn)
	long.CheckOut = start.AddDate(0, 0, 367).Format("2006-01-02")
	code, b, _ := f.request("POST", path, f.nextKey(), f.body(long))
	assertStatus(t, code, 422, b)
	past := f.input
	past.CheckIn = "2001-01-01"
	past.CheckOut = "2001-01-02"
	if _, _, e := f.confirm(past); !errors.Is(e, application.ErrPastDate) {
		t.Fatalf("past %v", e)
	}
	drift := f.input
	drift.ExpectedNightlyRateMinor++
	code, b, _ = f.request("POST", path, f.nextKey(), f.body(drift))
	assertStatus(t, code, 409, b)
	unknown := f.input
	unknown.RoomTypeID = 999999
	if _, _, e := f.confirm(unknown); !errors.Is(e, application.ErrReference) {
		t.Fatalf("missing type %v", e)
	}
	other, e := f.catalog.CreateProperty(f.ctx, domain.Property{Name: "Other", TimeZone: "UTC", Active: true})
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = f.service.Confirm(f.ctx, other.ID, f.nextKey(), f.input); !errors.Is(e, application.ErrReference) {
		t.Fatalf("cross property %v", e)
	}
	for _, amount := range []int64{math.MaxInt64} {
		_, _, e = f.catalog.SetRoomTypeRate(f.ctx, domain.RoomTypeRate{PropertyID: f.p.ID, RoomTypeID: f.typ.ID, AmountMinor: amount, Currency: "THB", Active: true})
		if e != nil {
			t.Fatal(e)
		}
		in := f.input
		in.ExpectedNightlyRateMinor = amount
		if _, _, e = f.confirm(in); !errors.Is(e, application.ErrInvalid) {
			t.Fatalf("subtotal overflow %v", e)
		}
	}
	if r, c := f.counts(); r != 0 || c != 0 {
		t.Fatal("invalid requests persisted")
	}

	f.store.pool.Close()
	code, b, _ = f.request("GET", path+"/1", "", "")
	assertStatus(t, code, 500, b)
	if strings.Contains(string(b), "closed") {
		t.Fatal("driver details leaked")
	}
}

// Each goroutine acquires a distinct pooled PostgreSQL connection. A start
// barrier creates >capacity overlapping demand; no Go mutex protects inventory.
func TestReservationConcurrencyAndIdempotency(t *testing.T) {
	for _, stock := range []int{1, 3} {
		t.Run(fmt.Sprint(stock), func(t *testing.T) {
			f := newBookingFixture(t, stock)
			f.enroll(f.typ.ID, true)
			start := make(chan struct{})
			var wg sync.WaitGroup
			var successes atomic.Int64
			errs := make(chan error, 10)
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					_, _, e := f.confirm(f.input)
					if e == nil {
						successes.Add(1)
					} else {
						errs <- e
					}
				}()
			}
			close(start)
			wg.Wait()
			close(errs)
			if successes.Load() != int64(stock) {
				t.Fatalf("confirmed %d wanted %d", successes.Load(), stock)
			}
			for e := range errs {
				if !errors.Is(e, application.ErrInventoryExhausted) {
					t.Fatalf("race error %v", e)
				}
			}
			if r, c := f.counts(); r != stock || c != stock*2 {
				t.Fatal("oversell/partial ledger")
			}
		})
	}
	t.Run("parallel same key and different types", func(t *testing.T) {
		f := newBookingFixture(t, 4)
		f.enroll(f.typ.ID, true)
		key := f.nextKey()
		var wg sync.WaitGroup
		ids := make(chan int64, 8)
		errs := make(chan error, 8)
		start := make(chan struct{})
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				r, _, e := f.service.Confirm(f.ctx, f.p.ID, key, f.input)
				if e != nil {
					errs <- e
				} else {
					ids <- r.ID
				}
			}()
		}
		close(start)
		wg.Wait()
		close(ids)
		close(errs)
		for e := range errs {
			t.Fatal(e)
		}
		var id int64
		for v := range ids {
			if id != 0 && id != v {
				t.Fatal("duplicate key committed twice")
			}
			id = v
		}
		if r, c := f.counts(); r != 1 || c != 2 {
			t.Fatal("duplicate consumption")
		}
		other := f.addType("Other neutral", 4)
		f.enroll(other.ID, true)
		in := f.input
		in.RoomTypeID = other.ID
		if _, _, e := f.service.Confirm(f.ctx, f.p.ID, key, in); !errors.Is(e, application.ErrIdempotencyConflict) {
			t.Fatalf("cross-type key: %v", e)
		}
		key = f.nextKey()
		start = make(chan struct{})
		codes := make(chan error, 2)
		for _, input := range []application.ConfirmReservation{f.input, in} {
			wg.Add(1)
			go func(v application.ConfirmReservation) {
				defer wg.Done()
				<-start
				_, _, e := f.service.Confirm(f.ctx, f.p.ID, key, v)
				codes <- e
			}(input)
		}
		close(start)
		wg.Wait()
		close(codes)
		ok, conflicts := 0, 0
		for e := range codes {
			if e == nil {
				ok++
			} else if errors.Is(e, application.ErrIdempotencyConflict) {
				conflicts++
			} else {
				t.Fatal(e)
			}
		}
		if ok != 1 || conflicts != 1 {
			t.Fatal("cross-type parallel key")
		}
	})
}

// Direct SQL deliberately bypasses application helpers. The database must still
// reject an incomplete/excess/cross-property ledger at durable commit.
func (f *bookingFixture) rawInsert(tx pgx.Tx, key string, offset int) (int64, error) {
	var id int64
	e := tx.QueryRow(f.ctx, `INSERT INTO pms_stay_reservations(property_id,room_type_id,idempotency_key,fingerprint,guest_ref,operator_ref,check_in,check_out,time_zone,currency,nightly_amount_minor,night_count,accommodation_base_subtotal_minor,pool_revision,attestation_ref,capacity_at_confirmation)
 SELECT $1,$2,$3,repeat('a',64),'SYN-RAW','SYN-OP',$4::date+$6::int,$5::date+$6::int,'Asia/Bangkok','THB',12500,2,25000,revision,attestation_ref,(SELECT count(*) FROM pms_rooms WHERE property_id=$1 AND room_type_id=$2 AND is_active) FROM pms_reservation_pools WHERE property_id=$1 AND room_type_id=$2 RETURNING id`, f.p.ID, f.typ.ID, key, f.input.CheckIn, f.input.CheckOut, offset).Scan(&id)
	return id, e
}
func (f *bookingFixture) rawNights(tx pgx.Tx, id int64, nights []int, property int64) error {
	for _, n := range nights {
		_, e := tx.Exec(f.ctx, `INSERT INTO pms_reservation_inventory_commitments(reservation_id,property_id,room_type_id,night) VALUES($1,$2,$3,$4::date+$5::int)`, id, property, f.typ.ID, f.input.CheckIn, n)
		if e != nil {
			return e
		}
	}
	return nil
}
func TestReservationDatabaseBackstops(t *testing.T) {
	f := newBookingFixture(t, 1)
	f.enroll(f.typ.ID, true)
	for _, tc := range []struct {
		name     string
		nights   []int
		property int64
	}{{"missing all", nil, f.p.ID}, {"partial", []int{0}, f.p.ID}, {"out of range", []int{0, 2}, f.p.ID}, {"duplicate", []int{0, 0}, f.p.ID}, {"cross property", []int{0, 1}, f.p.ID + 1}} {
		t.Run(tc.name, func(t *testing.T) {
			tx, e := f.pool.Begin(f.ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(f.ctx)
			id, e := f.rawInsert(tx, f.nextKey(), 0)
			if e != nil {
				t.Fatal(e)
			}
			e = f.rawNights(tx, id, tc.nights, tc.property)
			if e == nil {
				e = tx.Commit(f.ctx)
			}
			if e == nil {
				t.Fatal("raw integrity bypass")
			}
		})
	}
	if r, c := f.counts(); r != 0 || c != 0 {
		t.Fatal("raw failed transactions persisted")
	}
	// Two independent raw writers serialize at the type lock, with capacity checked
	// against a fresh snapshot at commit, even though neither called the Go use case.
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			tx, e := f.pool.Begin(f.ctx)
			if e != nil {
				results <- e
				return
			}
			defer tx.Rollback(f.ctx)
			id, e := f.rawInsert(tx, f.nextKey(), 0)
			if e == nil {
				e = f.rawNights(tx, id, []int{0, 1}, f.p.ID)
			}
			if e == nil {
				e = tx.Commit(f.ctx)
			}
			results <- e
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	ok, bad := 0, 0
	for e := range results {
		if e == nil {
			ok++
		} else {
			var pg *pgconn.PgError
			if !errors.As(e, &pg) || pg.Code != "P2402" {
				t.Fatalf("raw race: %v", e)
			}
			bad++
		}
	}
	if ok != 1 || bad != 1 {
		t.Fatal("raw overbooking")
	}
	var id int64
	if e := f.pool.QueryRow(f.ctx, "SELECT id FROM pms_stay_reservations").Scan(&id); e != nil {
		t.Fatal(e)
	}
	for _, q := range []string{
		"UPDATE pms_stay_reservations SET nightly_amount_minor=1",
		"DELETE FROM pms_stay_reservations",
		"UPDATE pms_reservation_inventory_commitments SET night=night+1",
		"DELETE FROM pms_reservation_inventory_commitments",
		"TRUNCATE pms_stay_reservations CASCADE",
		"TRUNCATE pms_reservation_inventory_commitments",
		"TRUNCATE pms_rooms CASCADE",
		"DELETE FROM pms_rooms",
		"UPDATE pms_rooms SET is_active=false",
		"UPDATE pms_properties SET time_zone='UTC'",
		"UPDATE pms_room_types SET max_occupancy=3",
	} {
		if _, e := f.pool.Exec(f.ctx, q); e == nil {
			t.Fatalf("unsafe raw mutation accepted: %s", q)
		}
	}
	if r, c := f.counts(); r != 1 || c != 2 {
		t.Fatal("history changed")
	}
	if _, _, e := f.confirm(f.input); !errors.Is(e, application.ErrInventoryExhausted) {
		t.Fatalf("raw stock not counted: %v", e)
	}
}

func TestReservationCatalogProtectionAndRaces(t *testing.T) {
	f := newBookingFixture(t, 1)
	other := f.addType("Other", 1)
	f.enroll(f.typ.ID, true)
	room := f.rooms[0]
	changed := room
	changed.Active = false
	if _, e := f.catalog.UpdateRoom(f.ctx, changed); !errors.Is(e, application.ErrProtected) {
		t.Fatalf("enrolled deactivation %v", e)
	}
	changed = room
	changed.RoomTypeID = other.ID
	if _, e := f.catalog.UpdateRoom(f.ctx, changed); !errors.Is(e, application.ErrProtected) {
		t.Fatalf("reassign %v", e)
	}
	if _, e := f.catalog.CreateRoom(f.ctx, domain.Room{PropertyID: f.p.ID, RoomTypeID: f.typ.ID, Number: "SYN-EXTRA", Active: true}); !errors.Is(e, application.ErrProtected) {
		t.Fatalf("unattested addition %v", e)
	}
	// Harmless display edits and sales closure still work.
	room.Number = "SYN-RENAMED"
	if _, e := f.catalog.UpdateRoom(f.ctx, room); e != nil {
		t.Fatal(e)
	}
	f.typ.Name = "Display name"
	if _, e := f.catalog.UpdateRoomType(f.ctx, f.typ); e != nil {
		t.Fatal(e)
	}
	f.p.Name = "New display"
	if _, e := f.catalog.UpdateProperty(f.ctx, f.p); e != nil {
		t.Fatal(e)
	}
	// Race confirmation and deactivation through independent transactions.
	start := make(chan struct{})
	var wg sync.WaitGroup
	var bookingErr, editErr error
	wg.Add(2)
	go func() { defer wg.Done(); <-start; _, _, bookingErr = f.confirm(f.input) }()
	go func() { defer wg.Done(); <-start; _, editErr = f.catalog.UpdateRoom(f.ctx, changed) }()
	close(start)
	wg.Wait()
	if bookingErr != nil || !errors.Is(editErr, application.ErrProtected) {
		t.Fatalf("catalog race: booking=%v edit=%v", bookingErr, editErr)
	}
	f.enroll(f.typ.ID, false)
	changed = room
	changed.Active = false
	if _, e := f.catalog.UpdateRoom(f.ctx, changed); !errors.Is(e, application.ErrProtected) {
		t.Fatalf("closed pool released stock %v", e)
	}
	f.p.TimeZone = "UTC"
	if _, e := f.catalog.UpdateProperty(f.ctx, f.p); !errors.Is(e, application.ErrProtected) {
		t.Fatalf("timezone %v", e)
	}
	f.p.TimeZone = "Asia/Bangkok"
	f.p.Active = false
	if _, e := f.catalog.UpdateProperty(f.ctx, f.p); e != nil {
		t.Fatal(e)
	}
	if _, _, e := f.confirm(f.input); !errors.Is(e, application.ErrInactive) {
		t.Fatalf("closed property %v", e)
	}
	// Closed and never booked inventory can be edited and explicitly reattested.
	f.enroll(other.ID, true)
	f.enroll(other.ID, false)
	room = f.rooms[1]
	room.Active = false
	if _, e := f.catalog.UpdateRoom(f.ctx, room); e != nil {
		t.Fatalf("unencumbered closed pool %v", e)
	}
}

func TestReservationRetryAndCommitRecovery(t *testing.T) {
	f := newBookingFixture(t, 4)
	f.enroll(f.typ.ID, true)
	for _, state := range []string{"40001", "40P01"} {
		t.Run(state, func(t *testing.T) {
			cmd, e := application.NormalizeReservation(f.p.ID, f.nextKey(), f.input)
			if e != nil {
				t.Fatal(e)
			}
			calls := 0
			r, replay, e := f.store.confirmWithRetry(f.ctx, cmd, func(ctx context.Context, c application.ReservationCommand) (domain.StayReservation, bool, error) {
				calls++
				if calls == 1 {
					return domain.StayReservation{}, false, &pgconn.PgError{Code: state}
				}
				return f.store.confirmOnce(ctx, c)
			})
			if e != nil || replay || r.ID == 0 || calls != 2 {
				t.Fatalf("whole-command retry %d %+v %v", calls, r, e)
			}
		})
	}
	t.Run("lost durable commit acknowledgment", func(t *testing.T) {
		cmd, _ := application.NormalizeReservation(f.p.ID, f.nextKey(), f.input)
		calls := 0
		var committed domain.StayReservation
		r, replay, e := f.store.confirmWithRetry(f.ctx, cmd, func(ctx context.Context, c application.ReservationCommand) (domain.StayReservation, bool, error) {
			calls++
			v, _, e := f.store.confirmOnce(ctx, c)
			if e != nil {
				return v, false, e
			}
			committed = v
			return domain.StayReservation{}, false, &unknownCommit{cause: errors.New("lost connection after durable commit")}
		})
		if e != nil || !replay || calls != 1 || !reflect.DeepEqual(r, committed) {
			t.Fatalf("unknown durable result %+v %v", r, e)
		}
	})
	t.Run("unknown rolled back outcome retries original key", func(t *testing.T) {
		cmd, _ := application.NormalizeReservation(f.p.ID, f.nextKey(), f.input)
		calls := 0
		r, _, e := f.store.confirmWithRetry(f.ctx, cmd, func(ctx context.Context, c application.ReservationCommand) (domain.StayReservation, bool, error) {
			calls++
			if calls == 1 {
				return domain.StayReservation{}, false, &unknownCommit{cause: errors.New("connection lost before commit")}
			}
			return f.store.confirmOnce(ctx, c)
		})
		if e != nil || r.ID == 0 || calls != 2 {
			t.Fatalf("unknown absent result %d %v", calls, e)
		}
	})
	t.Run("bounded retry exhaustion", func(t *testing.T) {
		cmd, _ := application.NormalizeReservation(f.p.ID, f.nextKey(), f.input)
		calls := 0
		_, _, e := f.store.confirmWithRetry(f.ctx, cmd, func(context.Context, application.ReservationCommand) (domain.StayReservation, bool, error) {
			calls++
			return domain.StayReservation{}, false, &pgconn.PgError{Code: "40001"}
		})
		if calls != 3 || !errors.Is(e, application.ErrUnavailable) {
			t.Fatalf("retry bound %d %v", calls, e)
		}
	})
	if r, c := f.counts(); r != 4 || c != 8 {
		t.Fatal("retry duplicated commitments")
	}
}

func TestReservationLocalCalendarAndTechnicalBudget(t *testing.T) {
	for _, zone := range []string{"Pacific/Kiritimati", "America/Los_Angeles", "America/New_York"} {
		t.Run(zone, func(t *testing.T) {
			f := newBookingFixture(t, 1)
			f.p.TimeZone = zone
			if _, e := f.catalog.UpdateProperty(f.ctx, f.p); e != nil {
				t.Fatal(e)
			}
			f.enroll(f.typ.ID, true)
			loc, _ := time.LoadLocation(zone)
			today := time.Now().In(loc)
			in := f.input
			in.CheckIn = today.Format("2006-01-02")
			in.CheckOut = today.AddDate(0, 0, 1).Format("2006-01-02")
			r, _, e := f.confirm(in)
			if e != nil || r.TimeZone != zone || r.BookingTerms.NightCount != 1 {
				t.Fatalf("local today %+v %v", r, e)
			}
			in.CheckIn = today.AddDate(0, 0, -1).Format("2006-01-02")
			in.CheckOut = today.Format("2006-01-02")
			if _, _, e = f.confirm(in); !errors.Is(e, application.ErrPastDate) {
				t.Fatalf("local yesterday %v", e)
			}
			// An upcoming DST transition still consumes calendar dates, not 23/25h units.
			march := time.Date(today.Year(), 3, 1, 0, 0, 0, 0, time.UTC)
			if march.Before(time.Now().UTC().AddDate(0, 0, 2)) {
				march = march.AddDate(1, 0, 0)
			}
			in.CheckIn = march.Format("2006-01-02")
			in.CheckOut = march.AddDate(0, 0, 15).Format("2006-01-02")
			r, _, e = f.confirm(in)
			if e != nil || r.BookingTerms.NightCount != 15 || r.BookingTerms.AccommodationBaseSubtotalMinor != 187500 {
				t.Fatalf("calendar nights %+v %v", r, e)
			}
		})
	}
	t.Run("expanded-night processing bound", func(t *testing.T) {
		f := newBookingFixture(t, 1)
		f.enroll(f.typ.ID, true)
		in := f.input
		start, _ := time.Parse("2006-01-02", in.CheckIn)
		in.CheckOut = start.AddDate(0, 0, 366).Format("2006-01-02")
		began := time.Now()
		r, _, e := f.confirm(in)
		if e != nil || r.BookingTerms.NightCount != 366 {
			t.Fatalf("366-night boundary %+v %v", r, e)
		}
		t.Logf("366-night confirmation and deferred guards: %s", time.Since(began))
		if _, c := f.counts(); c != 366 {
			t.Fatal("expanded ledger incomplete")
		}
	})
}

func TestReservationRealLockTimeoutAndFreshSnapshot(t *testing.T) {
	f := newBookingFixture(t, 1)
	f.enroll(f.typ.ID, true)
	tx, e := f.pool.Begin(f.ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(f.ctx)
	// Lock only the type, as another confirmation would. Requests can obtain the
	// property share lock, then must obey their own bounded type lock wait/retry.
	if _, e = tx.Exec(f.ctx, "SELECT id FROM pms_room_types WHERE id=$1 FOR UPDATE", f.typ.ID); e != nil {
		t.Fatal(e)
	}
	path := fmt.Sprintf("/properties/%d/stay-reservations", f.p.ID)
	key := f.nextKey()
	started := time.Now()
	code, b, header := f.request("POST", path, key, f.body(f.input))
	assertStatus(t, code, 503, b)
	if header.Get("Retry-After") == "" || time.Since(started) > application.ReservationDeadline+time.Second {
		t.Fatal("unbounded contention")
	}
	if r, c := f.counts(); r != 0 || c != 0 {
		t.Fatal("timed out command persisted")
	}
	if e = tx.Commit(f.ctx); e != nil {
		t.Fatal(e)
	}
	code, b, _ = f.request("POST", path, key, f.body(f.input))
	assertStatus(t, code, 201, b)
	// A stale request issued while an exclusive catalog write holds the property
	// lock must see the new price AFTER waiting, and roll back without a key.
	in := f.input
	start, _ := time.Parse("2006-01-02", in.CheckIn)
	in.CheckIn = start.AddDate(0, 0, 10).Format("2006-01-02")
	in.CheckOut = start.AddDate(0, 0, 11).Format("2006-01-02")
	tx, e = f.pool.Begin(f.ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(f.ctx)
	if e = lockCatalog(f.ctx, tx, f.p.ID, f.typ.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(f.ctx, "UPDATE pms_room_type_rates SET amount_minor=15000 WHERE room_type_id=$1", f.typ.ID); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, _, e := f.confirm(in); done <- e }()
	// Wait until PG observes a real blocked waiter; no scheduling/sleep assumption.
	deadline := time.Now().Add(2 * time.Second)
	waiting := false
	for time.Now().Before(deadline) {
		if e = f.pool.QueryRow(f.ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT time_zone%')").Scan(&waiting); e != nil {
			t.Fatal(e)
		}
		if waiting {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("request did not reach lock barrier")
	}
	if e = tx.Commit(f.ctx); e != nil {
		t.Fatal(e)
	}
	if e = <-done; !errors.Is(e, application.ErrPriceChanged) {
		t.Fatalf("stale pre-lock price: %v", e)
	}
}

func TestReservationRejectsStaleSnapshotWriters(t *testing.T) {
	f := newBookingFixture(t, 1)
	f.enroll(f.typ.ID, true)
	for _, level := range []pgx.TxIsoLevel{pgx.RepeatableRead, pgx.Serializable} {
		tx, e := f.pool.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: level})
		if e != nil {
			t.Fatal(e)
		}
		_, e = f.rawInsert(tx, f.nextKey(), 0)
		var pg *pgconn.PgError
		if !errors.As(e, &pg) || pg.Code != "0A000" {
			t.Fatalf("stale snapshot booking accepted: %v", e)
		}
		_ = tx.Rollback(f.ctx)
		tx, e = f.pool.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: level})
		if e != nil {
			t.Fatal(e)
		}
		_, e = tx.Exec(f.ctx, "UPDATE pms_rooms SET is_active=false WHERE id=$1", f.rooms[0].ID)
		if !errors.As(e, &pg) || pg.Code != "0A000" {
			t.Fatalf("stale snapshot catalog accepted: %v", e)
		}
		_ = tx.Rollback(f.ctx)
	}
}

func TestReservationPoolHTTPGate(t *testing.T) {
	f := newBookingFixture(t, 1)
	path := fmt.Sprintf("/properties/%d/room-types/%d/reservation-pool", f.p.ID, f.typ.ID)
	code, b, _ := f.request("GET", path, "", "")
	assertStatus(t, code, 404, b)
	for _, body := range []string{
		`{"scope":"homogeneous_segment_neutral","attestation_ref":"SYN-A","operator_ref":"SYN-O"}`,
		`{"scope":"market_segment","enabled":true,"attestation_ref":"SYN-A","operator_ref":"SYN-O"}`,
		`{"scope":"homogeneous_segment_neutral","enabled":true,"attestation_ref":"","operator_ref":"SYN-O"}`,
		`{"scope":"homogeneous_segment_neutral","enabled":true,"attestation_ref":"SYN-A","operator_ref":"SYN-O","capacity":999}`,
	} {
		code, b, _ = f.request("PUT", path, "", body)
		assertStatus(t, code, 400, b)
	}
	code, b, _ = f.request("PUT", path, "", `{"scope":"homogeneous_segment_neutral","enabled":true,"attestation_ref":"SYN-A","operator_ref":"SYN-O"}`)
	assertStatus(t, code, 200, b)
	p := parseResponse[domain.ReservationPool](t, b)
	if p.Revision != 1 || p.RecordedAt.IsZero() {
		t.Fatal("missing attestation evidence")
	}
	code, read, _ := f.request("GET", path, "", "")
	assertStatus(t, code, 200, read)
	if !bytes.Equal(b, read) {
		t.Fatal("pool readback")
	}
	code, b, _ = f.request("PUT", path, "", `{"scope":"homogeneous_segment_neutral","enabled":false,"attestation_ref":"SYN-B","operator_ref":"SYN-O"}`)
	assertStatus(t, code, 200, b)
	p = parseResponse[domain.ReservationPool](t, b)
	if p.Revision != 2 || p.Enabled {
		t.Fatal("revision/closure")
	}
	body := f.body(f.input)
	code, b, _ = f.request("POST", fmt.Sprintf("/properties/%d/stay-reservations", f.p.ID), f.nextKey(), body)
	assertStatus(t, code, 412, b)
	// Explicit enrollment under inactive parents is configuration only.
	f.p.Active = false
	if _, e := f.catalog.UpdateProperty(f.ctx, f.p); e != nil {
		t.Fatal(e)
	}
	code, b, _ = f.request("PUT", path, "", `{"scope":"homogeneous_segment_neutral","enabled":true,"attestation_ref":"SYN-C","operator_ref":"SYN-O"}`)
	assertStatus(t, code, 200, b)
	code, b, _ = f.request("POST", fmt.Sprintf("/properties/%d/stay-reservations", f.p.ID), f.nextKey(), body)
	assertStatus(t, code, 409, b)
}
