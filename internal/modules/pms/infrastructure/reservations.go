package infrastructure

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/application"
	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var _ application.ReservationStore = (*Store)(nil)

// Catalog writers take the property EXCLUSIVE lock first, then types ascending,
// then child rows. Confirmation takes property SHARE, then its one type UPDATE.
func lockCatalog(ctx context.Context, tx pgx.Tx, p int64, types ...int64) error {
	var id int64
	if err := tx.QueryRow(ctx, "SELECT id FROM pms_properties WHERE id=$1 FOR UPDATE", p).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.ErrReference
		}
		return err
	}
	if len(types) > 0 {
		rows, err := tx.Query(ctx, "SELECT id FROM pms_room_types WHERE property_id=$1 AND id=ANY($2) ORDER BY id FOR UPDATE", p, types)
		if err != nil {
			return err
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) SetReservationPool(ctx context.Context, p domain.ReservationPool) (domain.ReservationPool, error) {
	ctx, cancel := context.WithTimeout(ctx, application.ReservationDeadline)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return p, err
	}
	defer tx.Rollback(context.Background())
	if err = lockCatalog(ctx, tx, p.PropertyID, p.RoomTypeID); err != nil {
		return p, classify(err)
	}
	err = tx.QueryRow(ctx, `INSERT INTO pms_reservation_pools(property_id,room_type_id,scope,enabled,attestation_ref,operator_ref)
 VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(room_type_id) DO UPDATE SET scope=EXCLUDED.scope,enabled=EXCLUDED.enabled,attestation_ref=EXCLUDED.attestation_ref,operator_ref=EXCLUDED.operator_ref
 WHERE pms_reservation_pools.property_id=EXCLUDED.property_id
 RETURNING property_id,room_type_id,scope,enabled,attestation_ref,operator_ref,revision,recorded_at`, p.PropertyID, p.RoomTypeID, p.Scope, p.Enabled, p.AttestationRef, p.OperatorRef).Scan(&p.PropertyID, &p.RoomTypeID, &p.Scope, &p.Enabled, &p.AttestationRef, &p.OperatorRef, &p.Revision, &p.RecordedAt)
	if err != nil {
		return p, classify(err)
	}
	return p, classify(tx.Commit(ctx))
}
func (s *Store) GetReservationPool(ctx context.Context, p, t int64) (domain.ReservationPool, error) {
	var v domain.ReservationPool
	err := s.pool.QueryRow(ctx, `SELECT property_id,room_type_id,scope,enabled,attestation_ref,operator_ref,revision,recorded_at FROM pms_reservation_pools WHERE property_id=$1 AND room_type_id=$2`, p, t).Scan(&v.PropertyID, &v.RoomTypeID, &v.Scope, &v.Enabled, &v.AttestationRef, &v.OperatorRef, &v.Revision, &v.RecordedAt)
	return v, classify(err)
}

const reservationColumns = `id,property_id,room_type_id,guest_ref,operator_ref,status,check_in::text,check_out::text,time_zone,currency,nightly_amount_minor,night_count,accommodation_base_subtotal_minor,pool_revision,attestation_ref,capacity_at_confirmation,confirmed_at`

func scanReservation(row pgx.Row) (domain.StayReservation, error) {
	var r domain.StayReservation
	err := row.Scan(&r.ID, &r.PropertyID, &r.RoomTypeID, &r.GuestRef, &r.OperatorRef, &r.Status, &r.CheckIn, &r.CheckOut, &r.TimeZone, &r.BookingTerms.Currency, &r.BookingTerms.NightlyAmountMinor, &r.BookingTerms.NightCount, &r.BookingTerms.AccommodationBaseSubtotalMinor, &r.PoolRevision, &r.AttestationRef, &r.CapacityAtConfirmation, &r.ConfirmedAt)
	r.InventoryBasis = domain.PoolScope + "_pool"
	r.BookingTerms.Version = 1
	return r, err
}
func (s *Store) GetReservation(ctx context.Context, p, id int64) (domain.StayReservation, error) {
	r, err := scanReservation(s.pool.QueryRow(ctx, "SELECT "+reservationColumns+" FROM pms_stay_reservations WHERE property_id=$1 AND id=$2", p, id))
	return r, classify(err)
}
func lookupKey(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, c application.ReservationCommand) (domain.StayReservation, error) {
	var id int64
	var fp string
	err := q.QueryRow(ctx, "SELECT id,fingerprint FROM pms_stay_reservations WHERE property_id=$1 AND idempotency_key=$2", c.PropertyID, c.Key).Scan(&id, &fp)
	if err != nil {
		return domain.StayReservation{}, err
	}
	if fp != c.Fingerprint {
		return domain.StayReservation{}, application.ErrIdempotencyConflict
	}
	return scanReservation(q.QueryRow(ctx, "SELECT "+reservationColumns+" FROM pms_stay_reservations WHERE property_id=$1 AND id=$2", c.PropertyID, id))
}

// transactionAttempt is kept typed and local so failure tests can exercise the
// real retry/recovery boundary without a generic transaction framework.
type transactionAttempt func(context.Context, application.ReservationCommand) (domain.StayReservation, bool, error)

func (s *Store) ConfirmReservation(ctx context.Context, c application.ReservationCommand) (domain.StayReservation, bool, error) {
	return s.confirmWithRetry(ctx, c, s.confirmOnce)
}
func retryable(err error) bool {
	var p *pgconn.PgError
	return errors.As(err, &p) && (p.Code == "40001" || p.Code == "40P01" || p.Code == "55P03")
}

type unknownCommit struct{ cause error }

func (e *unknownCommit) Error() string { return "commit outcome unknown" }
func (e *unknownCommit) Unwrap() error { return e.cause }
func (s *Store) confirmWithRetry(ctx context.Context, c application.ReservationCommand, attempt transactionAttempt) (domain.StayReservation, bool, error) {
	for n := 0; n < 3; n++ {
		r, replay, err := attempt(ctx, c)
		if err == nil {
			return r, replay, nil
		}
		var unknown *unknownCommit
		if errors.As(err, &unknown) {
			// Use the original key only. A fresh connection lookup can recover a durable
			// success. If still absent, rerun the SAME command; uniqueness waits for any
			// original transaction still finishing on the server.
			recovered, e := lookupKey(ctx, s.pool, c)
			if e == nil {
				return recovered, true, nil
			}
			if errors.Is(e, application.ErrIdempotencyConflict) {
				return r, false, e
			}
			if !errors.Is(e, pgx.ErrNoRows) {
				return r, false, application.ErrUnavailable
			}
		} else if !retryable(err) {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return r, false, application.ErrUnavailable
			}
			return r, false, classify(err)
		}
		if n < 2 {
			timer := time.NewTimer(time.Duration(n+1) * 20 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return r, false, application.ErrUnavailable
			case <-timer.C:
			}
		}
	}
	return domain.StayReservation{}, false, application.ErrUnavailable
}
func (s *Store) confirmOnce(ctx context.Context, c application.ReservationCommand) (domain.StayReservation, bool, error) {
	var empty domain.StayReservation
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return empty, false, err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SET LOCAL lock_timeout='1500ms'"); err != nil {
		return empty, false, err
	}
	var zone string
	var propertyActive, typeActive bool
	err = tx.QueryRow(ctx, "SELECT time_zone,is_active FROM pms_properties WHERE id=$1 FOR SHARE", c.PropertyID).Scan(&zone, &propertyActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return empty, false, application.ErrReference
	}
	if err != nil {
		return empty, false, err
	}
	err = tx.QueryRow(ctx, "SELECT is_active FROM pms_room_types WHERE property_id=$1 AND id=$2 FOR UPDATE", c.PropertyID, c.Input.RoomTypeID).Scan(&typeActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return empty, false, application.ErrReference
	}
	if err != nil {
		return empty, false, err
	}
	// Fresh post-lock lookup precedes ALL current configuration/date checks.
	if r, e := lookupKey(ctx, tx, c); e == nil {
		return r, true, nil
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return empty, false, e
	}
	if !propertyActive || !typeActive {
		return empty, false, application.ErrInactive
	}
	var enabled bool
	var rev int64
	var attestation string
	err = tx.QueryRow(ctx, "SELECT enabled,revision,attestation_ref FROM pms_reservation_pools WHERE property_id=$1 AND room_type_id=$2", c.PropertyID, c.Input.RoomTypeID).Scan(&enabled, &rev, &attestation)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !enabled {
		return empty, false, application.ErrPoolRequired
	}
	if err != nil {
		return empty, false, err
	}
	var amount int64
	var active bool
	err = tx.QueryRow(ctx, "SELECT amount_minor,is_active FROM pms_room_type_rates WHERE property_id=$1 AND room_type_id=$2", c.PropertyID, c.Input.RoomTypeID).Scan(&amount, &active)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !active {
		return empty, false, application.ErrInactive
	}
	if err != nil {
		return empty, false, err
	}
	if amount != c.Input.ExpectedNightlyRateMinor {
		return empty, false, application.ErrPriceChanged
	}
	if amount > math.MaxInt64/int64(c.NightCount) {
		return empty, false, application.ErrInvalid
	}
	var today string
	if err = tx.QueryRow(ctx, "SELECT (clock_timestamp() AT TIME ZONE $1)::date::text", zone).Scan(&today); err != nil {
		return empty, false, err
	}
	if c.Input.CheckIn < today {
		return empty, false, application.ErrPastDate
	}
	// Separate READ COMMITTED statement AFTER the mutex, never a pre-lock CTE or
	// process cache. Check every requested night, including empty ledger dates.
	var stock int64
	var exhausted bool
	err = tx.QueryRow(ctx, `SELECT count(*) FROM pms_rooms WHERE property_id=$1 AND room_type_id=$2 AND is_active`, c.PropertyID, c.Input.RoomTypeID).Scan(&stock)
	if err != nil {
		return empty, false, err
	}
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pms_reservation_inventory_commitments WHERE property_id=$1 AND room_type_id=$2 AND night >= $3::date AND night < $4::date GROUP BY night HAVING count(*) >= $5)`, c.PropertyID, c.Input.RoomTypeID, c.Input.CheckIn, c.Input.CheckOut, stock).Scan(&exhausted)
	if err != nil {
		return empty, false, err
	}
	if stock == 0 || exhausted {
		return empty, false, application.ErrInventoryExhausted
	}
	// Cross-type requests share the property key. ON CONFLICT waits, then a NEW
	// statement sees the winner even though it was invisible at INSERT start.
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO pms_stay_reservations(property_id,room_type_id,idempotency_key,fingerprint,guest_ref,operator_ref,check_in,check_out,time_zone,currency,nightly_amount_minor,night_count,accommodation_base_subtotal_minor,pool_revision,attestation_ref,capacity_at_confirmation)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'THB',$10,$11,$12,$13,$14,$15) ON CONFLICT(property_id,idempotency_key) DO NOTHING RETURNING id`, c.PropertyID, c.Input.RoomTypeID, c.Key, c.Fingerprint, c.Input.GuestRef, c.Input.OperatorRef, c.Input.CheckIn, c.Input.CheckOut, zone, amount, c.NightCount, amount*int64(c.NightCount), rev, attestation, stock).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		r, e := lookupKey(ctx, tx, c)
		return r, true, e
	}
	if err != nil {
		return empty, false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO pms_reservation_inventory_commitments(reservation_id,property_id,room_type_id,night)
 SELECT $1,$2,$3,$4::date+n FROM generate_series(0,$5::integer-1) n`, id, c.PropertyID, c.Input.RoomTypeID, c.Input.CheckIn, c.NightCount)
	if err != nil {
		return empty, false, err
	}
	r, err := scanReservation(tx.QueryRow(ctx, "SELECT "+reservationColumns+" FROM pms_stay_reservations WHERE id=$1", id))
	if err != nil {
		return empty, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		var p *pgconn.PgError
		if errors.As(err, &p) || errors.Is(err, pgx.ErrTxCommitRollback) {
			return empty, false, err
		}
		return empty, false, &unknownCommit{cause: err}
	}
	return r, false, nil
}
