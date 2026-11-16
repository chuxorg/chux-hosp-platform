package infrastructure

import (
	"context"
	"errors"

	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/application"
	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/domain"
	"github.com/jackc/pgx/v5"
)

// The transaction distinguishes create/replace without relying on PostgreSQL's
// internal tuple metadata. The PK serializes competing writes to the same rate.
func (s *Store) SetRoomTypeRate(ctx context.Context, rate domain.RoomTypeRate) (domain.RoomTypeRate, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.RoomTypeRate{}, false, err
	}
	defer tx.Rollback(context.Background())
	if err = lockCatalog(ctx, tx, rate.PropertyID, rate.RoomTypeID); err != nil {
		return domain.RoomTypeRate{}, false, classify(err)
	}
	err = tx.QueryRow(ctx, `INSERT INTO pms_room_type_rates (property_id,room_type_id,amount_minor,currency,is_active)
		VALUES ($1,$2,$3,$4,$5) ON CONFLICT (room_type_id) DO NOTHING
		RETURNING property_id,room_type_id,amount_minor,currency,is_active`, rate.PropertyID, rate.RoomTypeID, rate.AmountMinor, rate.Currency, rate.Active).
		Scan(&rate.PropertyID, &rate.RoomTypeID, &rate.AmountMinor, &rate.Currency, &rate.Active)
	created := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `UPDATE pms_room_type_rates SET amount_minor=$3,currency=$4,is_active=$5
			WHERE property_id=$1 AND room_type_id=$2 RETURNING property_id,room_type_id,amount_minor,currency,is_active`, rate.PropertyID, rate.RoomTypeID, rate.AmountMinor, rate.Currency, rate.Active).
			Scan(&rate.PropertyID, &rate.RoomTypeID, &rate.AmountMinor, &rate.Currency, &rate.Active)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.RoomTypeRate{}, false, application.ErrReference
		}
	}
	if err != nil {
		return domain.RoomTypeRate{}, false, classify(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.RoomTypeRate{}, false, classify(err)
	}
	return rate, created, nil
}

func (s *Store) GetRoomTypeRate(ctx context.Context, propertyID, roomTypeID int64) (domain.RoomTypeRate, error) {
	var rate domain.RoomTypeRate
	err := s.pool.QueryRow(ctx, `SELECT property_id,room_type_id,amount_minor,currency,is_active
		FROM pms_room_type_rates WHERE property_id=$1 AND room_type_id=$2`, propertyID, roomTypeID).
		Scan(&rate.PropertyID, &rate.RoomTypeID, &rate.AmountMinor, &rate.Currency, &rate.Active)
	return rate, classify(err)
}

// Dates intentionally do not enter SQL until reservation-aware policy exists (#24).
func (s *Store) EligibleRooms(ctx context.Context, propertyID int64) ([]domain.EligibleRoom, error) {
	rows, err := s.pool.Query(ctx, `SELECT r.id,r.property_id,r.room_type_id,r.number,r.is_active,
		rate.property_id,rate.room_type_id,rate.amount_minor,rate.currency,rate.is_active
		FROM pms_rooms r
		JOIN pms_room_types t ON (t.id,t.property_id)=(r.room_type_id,r.property_id)
		JOIN pms_properties p ON p.id=r.property_id
		JOIN pms_room_type_rates rate ON (rate.room_type_id,rate.property_id)=(t.id,t.property_id)
		WHERE r.property_id=$1 AND p.is_active AND t.is_active AND r.is_active AND rate.is_active
		ORDER BY r.id`, propertyID)
	if err != nil {
		return nil, classify(err)
	}
	defer rows.Close()
	rooms := make([]domain.EligibleRoom, 0)
	for rows.Next() {
		var item domain.EligibleRoom
		err = rows.Scan(&item.Room.ID, &item.Room.PropertyID, &item.Room.RoomTypeID, &item.Room.Number, &item.Room.Active,
			&item.NightlyRate.PropertyID, &item.NightlyRate.RoomTypeID, &item.NightlyRate.AmountMinor, &item.NightlyRate.Currency, &item.NightlyRate.Active)
		if err != nil {
			return nil, classify(err)
		}
		rooms = append(rooms, item)
	}
	return rooms, classify(rows.Err())
}
