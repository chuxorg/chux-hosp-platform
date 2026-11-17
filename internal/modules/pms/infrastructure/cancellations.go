package infrastructure

import (
	"context"
	"errors"
	"time"

	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/application"
	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const cancellationColumns = "reservation_id,property_id,room_type_id,operator_ref,reason_ref,cancelled_at,contract_version"

func scanCancellation(row pgx.Row, fingerprint ...*string) (domain.ReservationCancellation, error) {
	var event domain.ReservationCancellation
	columns := []any{&event.ReservationID, &event.PropertyID, &event.RoomTypeID, &event.OperatorRef, &event.ReasonRef, &event.CancelledAt, &event.ContractVersion}
	for _, value := range fingerprint {
		columns = append(columns, value)
	}
	err := row.Scan(columns...)
	event.Status = "cancelled"
	return event, err
}

func (s *Store) GetReservationCancellation(ctx context.Context, p, id int64) (domain.ReservationCancellation, error) {
	event, err := scanCancellation(s.pool.QueryRow(ctx, "SELECT "+cancellationColumns+" FROM pms_reservation_cancellations WHERE property_id=$1 AND reservation_id=$2", p, id))
	return event, classify(err)
}

func lookupCancellationKey(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, c application.CancellationCommand) (domain.ReservationCancellation, error) {
	var fingerprint string
	event, err := scanCancellation(q.QueryRow(ctx, "SELECT "+cancellationColumns+",fingerprint FROM pms_reservation_cancellations WHERE property_id=$1 AND idempotency_key=$2", c.PropertyID, c.Key), &fingerprint)
	if err == nil && (event.ReservationID != c.ReservationID || fingerprint != c.Fingerprint) {
		return domain.ReservationCancellation{}, application.ErrIdempotencyConflict
	}
	return event, err
}

type cancellationAttempt func(context.Context, application.CancellationCommand) (domain.ReservationCancellation, bool, error)

func (s *Store) CancelReservation(ctx context.Context, c application.CancellationCommand) (domain.ReservationCancellation, bool, error) {
	return s.cancelWithRetry(ctx, c, s.cancelOnce)
}

func (s *Store) cancelWithRetry(ctx context.Context, c application.CancellationCommand, attempt cancellationAttempt) (domain.ReservationCancellation, bool, error) {
	var empty domain.ReservationCancellation
	for n := 0; n < 3; n++ {
		event, replay, err := attempt(ctx, c)
		if err == nil {
			return event, replay, nil
		}
		var unknown *unknownCommit
		if errors.As(err, &unknown) {
			recovered, err := lookupCancellationKey(ctx, s.pool, c)
			switch {
			case err == nil:
				return recovered, true, nil
			case errors.Is(err, application.ErrIdempotencyConflict):
				return empty, false, err
			case !errors.Is(err, pgx.ErrNoRows):
				return empty, false, application.ErrUnavailable
			}
			// An absent event can still be in flight. Retry only this command.
		} else if !retryable(err) {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return empty, false, application.ErrUnavailable
			}
			return empty, false, classify(err)
		}
		if n < 2 {
			timer := time.NewTimer(time.Duration(n+1) * 20 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return empty, false, application.ErrUnavailable
			case <-timer.C:
			}
		}
	}
	return empty, false, application.ErrUnavailable
}

func (s *Store) cancelOnce(ctx context.Context, c application.CancellationCommand) (domain.ReservationCancellation, bool, error) {
	var empty domain.ReservationCancellation
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return empty, false, err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SET LOCAL lock_timeout='1500ms'"); err != nil {
		return empty, false, err
	}
	// Discover only immutable ownership before taking the canonical locks.
	var typeID, locked int64
	if err = tx.QueryRow(ctx, "SELECT room_type_id FROM pms_stay_reservations WHERE property_id=$1 AND id=$2", c.PropertyID, c.ReservationID).Scan(&typeID); err != nil {
		return empty, false, err
	}
	if err = tx.QueryRow(ctx, "SELECT id FROM pms_properties WHERE id=$1 FOR SHARE", c.PropertyID).Scan(&locked); err != nil {
		return empty, false, err
	}
	if err = tx.QueryRow(ctx, "SELECT id FROM pms_room_types WHERE property_id=$1 AND id=$2 FOR UPDATE", c.PropertyID, typeID).Scan(&locked); err != nil {
		return empty, false, err
	}
	if err = tx.QueryRow(ctx, "SELECT id FROM pms_stay_reservations WHERE property_id=$1 AND room_type_id=$2 AND id=$3 FOR UPDATE", c.PropertyID, typeID, c.ReservationID).Scan(&locked); err != nil {
		return empty, false, err
	}
	if event, err := lookupCancellationKey(ctx, tx, c); !errors.Is(err, pgx.ErrNoRows) {
		return event, err == nil, err
	}
	// Distinguish a different key on this reservation before evaluating cutoff.
	var existing bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pms_reservation_cancellations WHERE reservation_id=$1)", c.ReservationID).Scan(&existing); err != nil {
		return empty, false, err
	}
	if existing {
		return empty, false, application.ErrAlreadyCancelled
	}

	// The INSERT guard supplies the decision timestamp and checks the original
	// calendar and complete ledger under these same locks.
	event, err := scanCancellation(tx.QueryRow(ctx, `INSERT INTO pms_reservation_cancellations
 (reservation_id,property_id,room_type_id,idempotency_key,fingerprint,operator_ref,reason_ref)
 VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(property_id,idempotency_key) DO NOTHING RETURNING `+cancellationColumns,
		c.ReservationID, c.PropertyID, typeID, c.Key, c.Fingerprint, c.Input.OperatorRef, c.Input.ReasonRef))
	if errors.Is(err, pgx.ErrNoRows) {
		// A different RoomType can win the shared property key concurrently.
		result, err := lookupCancellationKey(ctx, tx, c)
		return result, err == nil, err
	}
	if err != nil {
		return empty, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) || errors.Is(err, pgx.ErrTxCommitRollback) {
			return empty, false, err
		}
		return empty, false, &unknownCommit{cause: err}
	}
	return event, false, nil
}
