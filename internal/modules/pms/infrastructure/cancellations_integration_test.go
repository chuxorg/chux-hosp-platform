//go:build integration

package infrastructure

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/application"
	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/domain"
	"github.com/chuxorg/chux-hosp-platform/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const cancellationBody = `{"operator_ref":"SYN-OP-2","reason_ref":"SYN-REASON"}`

var cancellationInput = application.CancelReservation{OperatorRef: "SYN-OP-2", ReasonRef: "SYN-REASON"}

func confirmedStay(t *testing.T, f *bookingFixture, in application.ConfirmReservation, key string) domain.StayReservation {
	t.Helper()
	r, replay, err := f.service.Confirm(f.ctx, f.p.ID, key, in)
	if err != nil || replay {
		t.Fatalf("confirm: %v, replay %v", err, replay)
	}
	return r
}

func countCancellationState(t *testing.T, f *bookingFixture, events, nights int) {
	t.Helper()
	var actualEvents, actualNights int
	err := f.pool.QueryRow(f.ctx, "SELECT (SELECT count(*) FROM pms_reservation_cancellations), (SELECT count(*) FROM pms_consuming_inventory_commitments)").Scan(&actualEvents, &actualNights)
	if err != nil || actualEvents != events || actualNights != nights {
		t.Fatalf("events %d, consuming nights %d: %v", actualEvents, actualNights, err)
	}
}

func stayEvidence(t *testing.T, f *bookingFixture, id int64) string {
	t.Helper()
	var evidence string
	err := f.pool.QueryRow(f.ctx, `SELECT jsonb_build_array(to_jsonb(r), (SELECT jsonb_agg(to_jsonb(n) ORDER BY n.night)
 FROM pms_reservation_inventory_commitments n WHERE n.reservation_id=r.id))::text FROM pms_stay_reservations r WHERE r.id=$1`, id).Scan(&evidence)
	if err != nil {
		t.Fatal(err)
	}
	return evidence
}

// REQ-PMS-003 / TST-PMS-001-07-A/B/E/L: preserve evidence, release and resell.
func TestCancellationReleaseResaleAndAPI(t *testing.T) {
	f := newBookingFixture(t, 1)
	f.enroll(f.typ.ID, true)
	bookingKey := f.nextKey()
	a := confirmedStay(t, f, f.input, bookingKey)
	evidence := stayEvidence(t, f, a.ID)
	if _, _, err := f.confirm(f.input); !errors.Is(err, application.ErrInventoryExhausted) {
		t.Fatalf("before cancellation: %v", err)
	}
	path := fmt.Sprintf("/properties/%d/stay-reservations/%d/cancellation", f.p.ID, a.ID)
	for _, body := range []string{
		`{"operator_ref":"SYN-1","reason_ref":"SYN-2","fee":0}`,
		cancellationBody + "{}",
		`{"operator_ref":"x","reason_ref":"guest@example.org"}`,
		strings.Repeat(" ", 17<<10) + cancellationBody,
	} {
		code, body, _ := f.request(http.MethodPost, path, bookingKey, body)
		assertStatus(t, code, 400, body)
	}
	code, body, _ := f.request(http.MethodPost, path, "bad-key", cancellationBody)
	assertStatus(t, code, 400, body)
	foreign := fmt.Sprintf("/properties/%d/stay-reservations/%d/cancellation", f.p.ID+1, a.ID)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		code, body, _ = f.request(method, foreign, bookingKey, cancellationBody)
		assertStatus(t, code, 404, body)
	}

	// Reuse the textual booking key to prove independent operation namespaces.
	code, first, headers := f.request(http.MethodPost, path, bookingKey, cancellationBody)
	assertStatus(t, code, 201, first)
	event := parseResponse[domain.ReservationCancellation](t, first)
	if event.ReservationID != a.ID || event.CancelledAt.Before(a.ConfirmedAt) || event.Status != "cancelled" ||
		event.ContractVersion != 1 || headers.Get("Location") != "/pms/v1"+path {
		t.Fatalf("event: %s", first)
	}
	countCancellationState(t, f, 1, 0)
	code, body, headers = f.request(http.MethodPost, path, bookingKey, `{"reason_ref":" SYN-REASON ","operator_ref":"SYN-OP-2"}`)
	assertStatus(t, code, 200, body)
	if !bytes.Equal(first, body) || headers.Get("Idempotency-Replayed") != "true" {
		t.Fatalf("changed replay: %s", body)
	}
	code, body, _ = f.request(http.MethodGet, path, "", "")
	assertStatus(t, code, 200, body)
	if !bytes.Equal(first, body) {
		t.Fatal("GET changed the event")
	}
	code, body, headers = f.request(http.MethodPost, path, f.nextKey(), cancellationBody)
	assertStatus(t, code, 409, body)
	if headers.Get("Location") != "/pms/v1"+path || !strings.Contains(string(body), "already_cancelled") {
		t.Fatalf("already cancelled response: %s", body)
	}
	b := confirmedStay(t, f, f.input, f.nextKey())
	if b.ID == a.ID || b.Status != "confirmed" {
		t.Fatal("resale did not create a distinct confirmed booking")
	}
	countCancellationState(t, f, 1, 2)
	if got := stayEvidence(t, f, a.ID); got != evidence {
		t.Fatal("historical confirmation or nights changed")
	}
	var rooms []int64
	if err := f.pool.QueryRow(f.ctx, "SELECT array_agg(id ORDER BY id) FROM pms_rooms").Scan(&rooms); err != nil || !reflect.DeepEqual(rooms, []int64{f.rooms[0].ID}) {
		t.Fatalf("physical stock changed: %v %v", rooms, err)
	}
	got, replay, err := f.service.Confirm(f.ctx, f.p.ID, bookingKey, f.input)
	a.Status = "cancelled"
	if err != nil || !replay || !reflect.DeepEqual(got, a) {
		t.Fatalf("confirmation replay: %+v %v", got, err)
	}
	got, err = f.service.Get(f.ctx, f.p.ID, a.ID)
	if err != nil || !reflect.DeepEqual(got, a) {
		t.Fatalf("current lookup: %+v %v", got, err)
	}
}

// REQ-PMS-003 / TST-PMS-001-07-C/D: concurrent duplicate commands and property key collisions.
func TestCancellationKeyRaces(t *testing.T) {
	for _, mode := range []string{"same key", "different keys", "cross type"} {
		t.Run(mode, func(t *testing.T) {
			f := newBookingFixture(t, 1)
			f.enroll(f.typ.ID, true)
			a := confirmedStay(t, f, f.input, f.nextKey())
			b := a
			key := f.nextKey()
			second := key
			want := application.ErrAlreadyCancelled
			switch mode {
			case "different keys":
				second = f.nextKey()
			case "cross type":
				in := f.input
				in.RoomTypeID = f.addType("Second synthetic type", 1).ID
				f.enroll(in.RoomTypeID, true)
				b = confirmedStay(t, f, in, f.nextKey())
				want = application.ErrIdempotencyConflict
			}
			type result struct {
				event  domain.ReservationCancellation
				replay bool
				err    error
			}
			start := make(chan struct{})
			results := make(chan result, 2)
			for _, command := range []struct {
				id  int64
				key string
			}{{a.ID, key}, {b.ID, second}} {
				go func() {
					<-start
					e, replay, err := f.service.Cancel(f.ctx, f.p.ID, command.id, command.key, cancellationInput)
					results <- result{e, replay, err}
				}()
			}
			close(start)
			first, other := <-results, <-results
			if first.err != nil || first.replay {
				first, other = other, first
			}
			if first.err != nil || first.replay {
				t.Fatalf("no winner: %+v %+v", first, other)
			}
			if mode == "same key" {
				if other.err != nil || !other.replay || !reflect.DeepEqual(first.event, other.event) {
					t.Fatalf("same-key replay: %+v", other)
				}
			} else if !errors.Is(other.err, want) {
				t.Fatalf("conflict: %v", other.err)
			}
			changed := cancellationInput
			changed.ReasonRef = "SYN-OTHER"
			winnerKey := key
			if mode == "different keys" && !errors.Is(other.err, application.ErrAlreadyCancelled) {
				t.Fatal(other.err)
			}
			if _, err := f.store.GetReservationCancellation(f.ctx, f.p.ID, first.event.ReservationID); err != nil {
				t.Fatal(err)
			}
			if mode == "different keys" {
				if err := f.pool.QueryRow(f.ctx, "SELECT idempotency_key::text FROM pms_reservation_cancellations").Scan(&winnerKey); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := f.service.Cancel(f.ctx, f.p.ID, first.event.ReservationID, winnerKey, changed); !errors.Is(err, application.ErrIdempotencyConflict) {
				t.Fatalf("changed payload: %v", err)
			}
			nights := 0
			if mode == "cross type" {
				nights = 2
			}
			countCancellationState(t, f, 1, nights)
		})
	}
}

// REQ-PMS-003 / TST-PMS-001-07-H: retry the complete command and verify durable recovery.
func TestCancellationRetryAndCommitRecovery(t *testing.T) {
	f := newBookingFixture(t, 3)
	f.enroll(f.typ.ID, true)
	for _, state := range []string{"40001", "40P01", "lost acknowledgment", "disconnect before commit"} {
		t.Run(state, func(t *testing.T) {
			a := confirmedStay(t, f, f.input, f.nextKey())
			cmd, err := application.NormalizeCancellation(f.p.ID, a.ID, f.nextKey(), cancellationInput)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			event, replay, err := f.store.cancelWithRetry(f.ctx, cmd, func(ctx context.Context, c application.CancellationCommand) (domain.ReservationCancellation, bool, error) {
				calls++
				if c != cmd {
					t.Fatal("retry changed command")
				}
				if calls == 1 && state != "lost acknowledgment" {
					if state == "disconnect before commit" {
						tx, err := f.pool.Begin(ctx)
						if err != nil {
							return domain.ReservationCancellation{}, false, err
						}
						defer tx.Rollback(ctx)
						if _, err = tx.Exec(ctx, rawCancellation, a.ID, f.p.ID, f.typ.ID, c.Key, c.Fingerprint); err != nil {
							return domain.ReservationCancellation{}, false, err
						}
						return domain.ReservationCancellation{}, false, &unknownCommit{cause: errors.New("connection lost before commit")}
					}
					return domain.ReservationCancellation{}, false, &pgconn.PgError{Code: state}
				}
				event, replay, err := f.store.cancelOnce(ctx, c)
				if err == nil && state == "lost acknowledgment" {
					return domain.ReservationCancellation{}, false, &unknownCommit{cause: errors.New("lost acknowledgment")}
				}
				return event, replay, err
			})
			if err != nil || event.ReservationID != a.ID || replay != (state == "lost acknowledgment") || (!replay && calls != 2) || (replay && calls != 1) {
				t.Fatalf("recovery: %+v %v calls %d replay %v", event, err, calls, replay)
			}
		})
	}
	countCancellationState(t, f, 4, 0)
}

// REQ-PMS-003 / TST-PMS-001-07-I/J/K: the database protects the original calendar and evidence.
func TestCancellationDBGuardsAndSalesIndependence(t *testing.T) {
	f := newBookingFixture(t, 2)
	f.enroll(f.typ.ID, true)
	a := confirmedStay(t, f, f.input, f.nextKey())
	var today, tomorrow string
	if err := f.pool.QueryRow(f.ctx, "SELECT local_day::text,(local_day+1)::text FROM (SELECT (clock_timestamp() AT TIME ZONE 'Asia/Bangkok')::date AS local_day) d").Scan(&today, &tomorrow); err != nil {
		t.Fatal(err)
	}
	in := f.input
	in.CheckIn, in.CheckOut = today, tomorrow
	b := confirmedStay(t, f, in, f.nextKey())
	key := f.nextKey()
	cmd, _ := application.NormalizeCancellation(f.p.ID, b.ID, key, cancellationInput)
	const raw = `INSERT INTO pms_reservation_cancellations(reservation_id,property_id,room_type_id,idempotency_key,fingerprint,operator_ref,reason_ref,cancelled_at)
 VALUES($1,$2,$3,$4,$5,'SYN-OP-2','SYN-REASON','2000-01-01')`
	_, err := f.pool.Exec(f.ctx, raw, b.ID, f.p.ID, f.typ.ID, cmd.Key, cmd.Fingerprint)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "P4301" {
		t.Fatalf("backdated cutoff: %v", err)
	}
	if _, _, err = f.service.Cancel(f.ctx, f.p.ID, b.ID, key, cancellationInput); !errors.Is(err, application.ErrCancellationCutoff) {
		t.Fatalf("same local date: %v", err)
	}
	if _, err = f.pool.Exec(f.ctx, raw, a.ID, f.p.ID, f.typ.ID, key, strings.Repeat("0", 64)); err == nil {
		t.Fatal("forged fingerprint accepted")
	}
	cmd, _ = application.NormalizeCancellation(f.p.ID, a.ID, key, cancellationInput)
	if _, err = f.pool.Exec(f.ctx, raw, a.ID, f.p.ID+1, f.typ.ID, key, cmd.Fingerprint); err == nil {
		t.Fatal("foreign ownership accepted")
	}
	tx, err := f.pool.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(f.ctx, raw, a.ID, f.p.ID, f.typ.ID, key, cmd.Fingerprint); !errors.As(err, &pgErr) || pgErr.Code != "0A000" {
		t.Errorf("strong isolation: %v", err)
	}
	_ = tx.Rollback(f.ctx)

	f.enroll(f.typ.ID, false)
	for _, mutation := range []string{
		"UPDATE pms_room_type_rates SET is_active=false,amount_minor=9999",
		"UPDATE pms_room_types SET is_active=false",
		"UPDATE pms_properties SET is_active=false",
	} {
		if _, err = f.pool.Exec(f.ctx, mutation); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err = f.confirm(f.input); !errors.Is(err, application.ErrInactive) {
		t.Fatalf("inactive sale: %v", err)
	}
	event, _, err := f.service.Cancel(f.ctx, f.p.ID, a.ID, key, cancellationInput)
	if err != nil || !event.CancelledAt.After(a.ConfirmedAt) {
		t.Fatalf("valid cancellation with closed sales: %+v %v", event, err)
	}
	for _, mutation := range []string{
		"UPDATE pms_reservation_cancellations SET reason_ref='SYN-OTHER'",
		"DELETE FROM pms_reservation_cancellations",
		"TRUNCATE pms_reservation_cancellations",
		"UPDATE pms_stay_reservations SET status='cancelled'",
		"UPDATE pms_properties SET time_zone='UTC'",
		"UPDATE pms_rooms SET is_active=false",
	} {
		if _, err = f.pool.Exec(f.ctx, mutation); err == nil {
			t.Fatalf("unsafe mutation accepted: %s", mutation)
		}
	}
	if _, err = f.pool.Exec(f.ctx, raw, a.ID, f.p.ID, f.typ.ID, f.nextKey(), cmd.Fingerprint); err == nil {
		t.Fatal("duplicate raw event accepted")
	}
	countCancellationState(t, f, 1, 1)
}

// REQ-PMS-003 / TST-PMS-001-07-M: upgrade actual 00004 evidence and reject destructive down.
func TestCancellationMigration(t *testing.T) {
	f := newBookingFixture(t, 1)
	f.enroll(f.typ.ID, true)
	down := func() {
		t.Helper()
		if version, err := migrations.Run(f.ctx, f.url, "down"); err != nil || version != 5 {
			t.Fatalf("auth down: %d %v", version, err)
		}
		if version, err := migrations.Run(f.ctx, f.url, "down"); err != nil || version != 4 {
			t.Fatalf("down: %d %v", version, err)
		}
	}
	up := func() {
		t.Helper()
		if version, err := migrations.Run(f.ctx, f.url, "up"); err != nil || version != 6 {
			t.Fatalf("up: %d %v", version, err)
		}
	}
	down()
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	id, err := f.rawInsert(tx, f.nextKey(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.rawNights(tx, id, []int{0, 1}, f.p.ID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	before := stayEvidence(t, f, id)
	up()
	countCancellationState(t, f, 0, 2)
	down()
	up()
	countCancellationState(t, f, 0, 2)
	if stayEvidence(t, f, id) != before {
		t.Fatal("upgrade/rollback altered 00004 evidence")
	}
	if _, _, err = f.service.Cancel(f.ctx, f.p.ID, id, f.nextKey(), cancellationInput); err != nil {
		t.Fatal(err)
	}
	confirmedStay(t, f, f.input, f.nextKey())
	if version, downErr := migrations.Run(f.ctx, f.url, "down"); downErr != nil || version != 5 {
		t.Fatalf("auth down before destructive check: %d %v", version, downErr)
	}
	if _, err = migrations.Run(f.ctx, f.url, "down"); err == nil || !strings.Contains(err.Error(), "cancellation history must be preserved") {
		t.Fatalf("event populated down: %v", err)
	}
	countCancellationState(t, f, 1, 2)
	if stayEvidence(t, f, id) != before {
		t.Fatal("refused rollback altered evidence")
	}
}

// waitForCancellationLock observes the waiting PostgreSQL session, independent
// of goroutine scheduling. Callers hold the blocker until the waiter is visible.
func waitForCancellationLock(t *testing.T, f *bookingFixture, query string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		var waiting bool
		err := f.pool.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity
 WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE $1)`, query).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("PostgreSQL waiter did not reach %s", query)
}

const rawCancellation = `INSERT INTO pms_reservation_cancellations
 (reservation_id,property_id,room_type_id,idempotency_key,fingerprint,operator_ref,reason_ref)
 VALUES($1,$2,$3,$4,$5,'SYN-OP-2','SYN-REASON')`

// REQ-PMS-003 / TST-PMS-001-07-F/G: serialize sale and catalog changes on real connections.
func TestCancellationInventoryAndCatalogRaces(t *testing.T) {
	for _, outcome := range []string{"booking first", "cancel commits", "cancel aborts", "catalog closure first"} {
		t.Run(outcome, func(t *testing.T) {
			f := newBookingFixture(t, 1)
			f.enroll(f.typ.ID, true)
			a := confirmedStay(t, f, f.input, f.nextKey())
			key := f.nextKey()
			tx, err := f.pool.Begin(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(f.ctx)
			_, err = tx.Exec(f.ctx, "SELECT id FROM pms_properties WHERE id=$1 FOR SHARE", f.p.ID)
			if err != nil {
				t.Fatal(err)
			}
			// The catalog branch must take the stronger property lock before its type.
			if outcome == "catalog closure first" {
				if err = lockCatalog(f.ctx, tx, f.p.ID, f.typ.ID); err != nil {
					t.Fatal(err)
				}
				if _, err = tx.Exec(f.ctx, "UPDATE pms_reservation_pools SET enabled=false WHERE room_type_id=$1", f.typ.ID); err != nil {
					t.Fatal(err)
				}
			} else if _, err = tx.Exec(f.ctx, "SELECT id FROM pms_room_types WHERE id=$1 FOR UPDATE", f.typ.ID); err != nil {
				t.Fatal(err)
			}

			done := make(chan error, 1)
			if outcome == "booking first" || outcome == "catalog closure first" {
				go func() {
					_, _, err := f.service.Cancel(f.ctx, f.p.ID, a.ID, key, cancellationInput)
					done <- err
				}()
				if outcome == "booking first" {
					waitForCancellationLock(t, f, "SELECT id FROM pms_room_types WHERE property_id=%")
					// The transaction holding the type lock sees the full last unit.
					var full bool
					err = tx.QueryRow(f.ctx, "SELECT count(*)=1 FROM pms_consuming_inventory_commitments WHERE room_type_id=$1 AND night=$2::date", f.typ.ID, f.input.CheckIn).Scan(&full)
					if err != nil || !full {
						t.Fatalf("booking-first snapshot: %v %v", full, err)
					}
					id, err := f.rawInsert(tx, f.nextKey(), 0)
					if err != nil {
						t.Fatal(err)
					}
					if err = f.rawNights(tx, id, []int{0, 1}, f.p.ID); err != nil {
						t.Fatal(err)
					}
					var pgErr *pgconn.PgError
					if err = tx.Commit(f.ctx); !errors.As(err, &pgErr) || pgErr.Code != "P2402" {
						t.Fatalf("booking-first oversell: %v", err)
					}
				} else {
					waitForCancellationLock(t, f, "SELECT id FROM pms_properties WHERE id=%")
					if err = tx.Commit(f.ctx); err != nil {
						t.Fatal(err)
					}
				}
				if err = <-done; err != nil {
					t.Fatal(err)
				}
				countCancellationState(t, f, 1, 0)
				room := f.rooms[0]
				room.Active = false
				_, err = f.catalog.UpdateRoom(f.ctx, room)
				if outcome == "booking first" && !errors.Is(err, application.ErrProtected) || outcome == "catalog closure first" && err != nil {
					t.Fatalf("backing-stock freeze: %v", err)
				}
				return
			}

			cmd, _ := application.NormalizeCancellation(f.p.ID, a.ID, key, cancellationInput)
			if _, err = tx.Exec(f.ctx, rawCancellation, a.ID, f.p.ID, f.typ.ID, key, cmd.Fingerprint); err != nil {
				t.Fatal(err)
			}
			countCancellationState(t, f, 0, 2)
			go func() { _, _, err := f.confirm(f.input); done <- err }()
			waitForCancellationLock(t, f, "SELECT is_active FROM pms_room_types WHERE property_id=%")
			if outcome == "cancel commits" {
				if err = tx.Commit(f.ctx); err != nil {
					t.Fatal(err)
				}
				if err = <-done; err != nil {
					t.Fatalf("fresh booking did not see release: %v", err)
				}
				countCancellationState(t, f, 1, 2)
			} else {
				if err = tx.Rollback(f.ctx); err != nil {
					t.Fatal(err)
				}
				if err = <-done; !errors.Is(err, application.ErrInventoryExhausted) {
					t.Fatalf("booking saw rolled-back release: %v", err)
				}
				countCancellationState(t, f, 0, 2)
				if _, _, err = f.service.Cancel(f.ctx, f.p.ID, a.ID, key, cancellationInput); err != nil {
					t.Fatalf("rolled-back key unusable: %v", err)
				}
			}
		})
	}
}

// REQ-PMS-003 / TST-PMS-001-07-H: a real lock timeout exhausts the bounded
// retry budget without retaining the cancellation key.
func TestCancellationRealLockTimeout(t *testing.T) {
	f := newBookingFixture(t, 1)
	f.enroll(f.typ.ID, true)
	a := confirmedStay(t, f, f.input, f.nextKey())
	key := f.nextKey()
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	if _, err = tx.Exec(f.ctx, "SELECT id FROM pms_room_types WHERE id=$1 FOR UPDATE", f.typ.ID); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/properties/%d/stay-reservations/%d/cancellation", f.p.ID, a.ID)
	start := time.Now()
	code, body, headers := f.request(http.MethodPost, path, key, cancellationBody)
	assertStatus(t, code, 503, body)
	if headers.Get("Retry-After") != "1" || time.Since(start) > application.ReservationDeadline+time.Second {
		t.Fatal("unexpected contention contract")
	}
	countCancellationState(t, f, 0, 2)
	if err = tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	code, body, _ = f.request(http.MethodPost, path, key, cancellationBody)
	assertStatus(t, code, 201, body)
}

// REQ-PMS-003 / TST-PMS-001-07-J: feed fixed instants into the actual PostgreSQL
// guard in an isolated child DB. Hold the type mutex across simulated local
// midnight on the DST transition date, without altering the server clock.
func TestCancellationCutoffAcrossLocalMidnight(t *testing.T) {
	f := newBookingFixture(t, 2)
	if _, err := f.pool.Exec(f.ctx, "UPDATE pms_properties SET time_zone='America/New_York'"); err != nil {
		t.Fatal(err)
	}
	f.enroll(f.typ.ID, true)
	in := f.input
	in.CheckIn, in.CheckOut = "2099-11-01", "2099-11-03"
	a := confirmedStay(t, f, in, f.nextKey())
	b := confirmedStay(t, f, in, f.nextKey())
	var guard string
	if err := f.pool.QueryRow(f.ctx, "SELECT pg_get_functiondef('pms_cancellation_guard()'::regprocedure)").Scan(&guard); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"CREATE TABLE test_decision_clock (instant timestamptz NOT NULL)",
		"INSERT INTO test_decision_clock VALUES ('2099-11-01 03:59:59+00')",
		"CREATE FUNCTION test_cancellation_clock() RETURNS timestamptz LANGUAGE sql VOLATILE AS 'SELECT instant FROM test_decision_clock'",
	} {
		if _, err := f.pool.Exec(f.ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Count(guard, "pg_catalog.clock_timestamp()") != 1 {
		t.Fatal("cannot substitute test clock into decision guard")
	}
	if _, err := f.pool.Exec(f.ctx, strings.Replace(guard, "pg_catalog.clock_timestamp()", "test_cancellation_clock()", 1)); err != nil {
		t.Fatal(err)
	}
	keyA, keyB := f.nextKey(), f.nextKey()
	cmd, err := application.NormalizeCancellation(f.p.ID, a.ID, keyA, cancellationInput)
	if err != nil {
		t.Fatal(err)
	}
	blocker, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(f.ctx)
	// The first decision is recorded before midnight and commits after it.
	if _, err = blocker.Exec(f.ctx, rawCancellation, a.ID, f.p.ID, f.typ.ID, keyA, cmd.Fingerprint); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, _, err := f.service.Cancel(f.ctx, f.p.ID, b.ID, keyB, cancellationInput)
		done <- err
	}()
	waitForCancellationLock(t, f, "SELECT id FROM pms_room_types WHERE property_id=%")
	if _, err = f.pool.Exec(f.ctx, "UPDATE test_decision_clock SET instant='2099-11-01 04:00:00+00'"); err != nil {
		t.Fatal(err)
	}
	if err = blocker.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	event, err := f.service.GetCancellation(f.ctx, f.p.ID, a.ID)
	if err != nil || !event.CancelledAt.Equal(time.Date(2099, 11, 1, 3, 59, 59, 0, time.UTC)) {
		t.Fatalf("pre-midnight decision committed after midnight: %+v %v", event, err)
	}
	if err = <-done; !errors.Is(err, application.ErrCancellationCutoff) {
		t.Fatalf("waiter did not use post-lock cutoff: %v", err)
	}
	for _, instant := range []string{"2099-11-01 05:30:00+00", "2099-11-01 06:30:00+00", "2099-11-04 04:00:00+00"} {
		if _, err = f.pool.Exec(f.ctx, "UPDATE test_decision_clock SET instant=$1::text::timestamptz", instant); err != nil {
			t.Fatal(err)
		}
		if _, _, err = f.service.Cancel(f.ctx, f.p.ID, b.ID, keyB, cancellationInput); !errors.Is(err, application.ErrCancellationCutoff) {
			t.Fatalf("cutoff at %s: %v", instant, err)
		}
	}
	got, replay, err := f.service.Cancel(f.ctx, f.p.ID, a.ID, keyA, cancellationInput)
	if err != nil || !replay || !reflect.DeepEqual(got, event) {
		t.Fatalf("after-cutoff replay: %+v %v", got, err)
	}
	countCancellationState(t, f, 1, 2)
}

// REQ-PMS-003 / TST-PMS-001-07-M: a cancellation committed while down
// waits on locks is visible to the downgrade's refusal check.
func TestCancellationDowngradeRace(t *testing.T) {
	f := newBookingFixture(t, 1)
	f.enroll(f.typ.ID, true)
	a := confirmedStay(t, f, f.input, f.nextKey())
	if version, downErr := migrations.Run(f.ctx, f.url, "down"); downErr != nil || version != 5 {
		t.Fatalf("auth down before race: %d %v", version, downErr)
	}
	cmd, _ := application.NormalizeCancellation(f.p.ID, a.ID, f.nextKey(), cancellationInput)
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	if _, err = tx.Exec(f.ctx, rawCancellation, a.ID, f.p.ID, f.typ.ID, cmd.Key, cmd.Fingerprint); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithTimeout(f.ctx, 8*time.Second)
	defer stop()
	result := make(chan error, 1)
	go func() {
		_, err := migrations.Run(ctx, f.url, "down")
		result <- err
	}()
	waitForCancellationLock(t, f, "%LOCK TABLE pms_properties, pms_room_types, pms_stay_reservations, pms_reservation_cancellations%")
	if err = tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-result; err == nil || !strings.Contains(err.Error(), "cancellation history must be preserved") {
		t.Fatalf("downgrade raced cancellation: %v", err)
	}
	countCancellationState(t, f, 1, 0)
}
