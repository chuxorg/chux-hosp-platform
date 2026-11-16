// Package infrastructure adapts PMS-owned configuration persistence to PostgreSQL.
// No module reads or mutates another module's tables.
package infrastructure

import (
	"context"
	"errors"

	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/application"
	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

var _ application.Store = (*Store)(nil)

func (s *Store) CreateProperty(ctx context.Context, p domain.Property) (domain.Property, error) {
	err := s.pool.QueryRow(ctx,
		"INSERT INTO pms_properties (name,time_zone,is_active) VALUES ($1,$2,$3) RETURNING id,name,time_zone,is_active",
		p.Name, p.TimeZone, p.Active,
	).Scan(&p.ID, &p.Name, &p.TimeZone, &p.Active)
	return p, classify(err)
}
func (s *Store) GetProperty(ctx context.Context, id int64) (domain.Property, error) {
	var p domain.Property
	err := s.pool.QueryRow(ctx,
		"SELECT id,name,time_zone,is_active FROM pms_properties WHERE id=$1", id,
	).Scan(&p.ID, &p.Name, &p.TimeZone, &p.Active)
	return p, classify(err)
}
func (s *Store) UpdateProperty(ctx context.Context, p domain.Property) (domain.Property, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return p, err
	}
	defer tx.Rollback(context.Background())
	if err = lockCatalog(ctx, tx, p.ID); err != nil {
		return p, classify(err)
	}
	err = tx.QueryRow(ctx,
		"UPDATE pms_properties SET name=$2,time_zone=$3,is_active=$4 WHERE id=$1 RETURNING id,name,time_zone,is_active",
		p.ID, p.Name, p.TimeZone, p.Active,
	).Scan(&p.ID, &p.Name, &p.TimeZone, &p.Active)
	if err != nil {
		return p, classify(err)
	}
	return p, classify(tx.Commit(ctx))
}
func (s *Store) CreateRoomType(ctx context.Context, t domain.RoomType) (domain.RoomType, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return t, err
	}
	defer tx.Rollback(context.Background())
	if err = lockCatalog(ctx, tx, t.PropertyID); err != nil {
		return t, classify(err)
	}
	err = tx.QueryRow(ctx,
		"INSERT INTO pms_room_types (property_id,name,max_occupancy,is_active) VALUES ($1,$2,$3,$4) RETURNING id,property_id,name,max_occupancy,is_active",
		t.PropertyID, t.Name, t.MaxOccupancy, t.Active,
	).Scan(&t.ID, &t.PropertyID, &t.Name, &t.MaxOccupancy, &t.Active)
	if err != nil {
		return t, classify(err)
	}
	return t, classify(tx.Commit(ctx))
}
func (s *Store) GetRoomType(ctx context.Context, propertyID, id int64) (domain.RoomType, error) {
	var t domain.RoomType
	err := s.pool.QueryRow(ctx,
		"SELECT id,property_id,name,max_occupancy,is_active FROM pms_room_types WHERE property_id=$1 AND id=$2",
		propertyID, id,
	).Scan(&t.ID, &t.PropertyID, &t.Name, &t.MaxOccupancy, &t.Active)
	return t, classify(err)
}
func (s *Store) UpdateRoomType(ctx context.Context, t domain.RoomType) (domain.RoomType, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return t, err
	}
	defer tx.Rollback(context.Background())
	if err = lockCatalog(ctx, tx, t.PropertyID, t.ID); err != nil {
		return t, classify(err)
	}
	err = tx.QueryRow(ctx,
		"UPDATE pms_room_types SET name=$3,max_occupancy=$4,is_active=$5 WHERE property_id=$1 AND id=$2 RETURNING id,property_id,name,max_occupancy,is_active",
		t.PropertyID, t.ID, t.Name, t.MaxOccupancy, t.Active,
	).Scan(&t.ID, &t.PropertyID, &t.Name, &t.MaxOccupancy, &t.Active)
	if err != nil {
		return t, classify(err)
	}
	return t, classify(tx.Commit(ctx))
}
func (s *Store) CreateRoom(ctx context.Context, r domain.Room) (domain.Room, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return r, err
	}
	defer tx.Rollback(context.Background())
	if err = lockCatalog(ctx, tx, r.PropertyID, r.RoomTypeID); err != nil {
		return r, classify(err)
	}
	err = tx.QueryRow(ctx,
		"INSERT INTO pms_rooms (property_id,room_type_id,number,is_active) VALUES ($1,$2,$3,$4) RETURNING id,property_id,room_type_id,number,is_active",
		r.PropertyID, r.RoomTypeID, r.Number, r.Active,
	).Scan(&r.ID, &r.PropertyID, &r.RoomTypeID, &r.Number, &r.Active)
	if err != nil {
		return r, classify(err)
	}
	return r, classify(tx.Commit(ctx))
}
func (s *Store) GetRoom(ctx context.Context, propertyID, id int64) (domain.Room, error) {
	var r domain.Room
	err := s.pool.QueryRow(ctx,
		"SELECT id,property_id,room_type_id,number,is_active FROM pms_rooms WHERE property_id=$1 AND id=$2",
		propertyID, id,
	).Scan(&r.ID, &r.PropertyID, &r.RoomTypeID, &r.Number, &r.Active)
	return r, classify(err)
}
func (s *Store) UpdateRoom(ctx context.Context, r domain.Room) (domain.Room, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return r, err
	}
	defer tx.Rollback(context.Background())
	if err = lockCatalog(ctx, tx, r.PropertyID); err != nil {
		return r, classify(err)
	}
	rows, err := tx.Query(ctx, "SELECT id FROM pms_room_types WHERE property_id=$1 ORDER BY id FOR UPDATE", r.PropertyID)
	if err != nil {
		return r, err
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return r, err
	}
	err = tx.QueryRow(ctx,
		"UPDATE pms_rooms SET room_type_id=$3,number=$4,is_active=$5 WHERE property_id=$1 AND id=$2 RETURNING id,property_id,room_type_id,number,is_active",
		r.PropertyID, r.ID, r.RoomTypeID, r.Number, r.Active,
	).Scan(&r.ID, &r.PropertyID, &r.RoomTypeID, &r.Number, &r.Active)
	if err != nil {
		return r, classify(err)
	}
	return r, classify(tx.Commit(ctx))
}

func classify(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ErrNotFound
	}
	var pge *pgconn.PgError
	if errors.As(err, &pge) {
		switch pge.Code {
		case "P2403":
			return application.ErrProtected
		case "P2402":
			return application.ErrInventoryExhausted
		case "P2401":
			return application.ErrInactive
		case "23505":
			return application.ErrConflict
		case "23503":
			return application.ErrReference
		case "23514", "23502", "22001", "22003":
			return application.ErrInvalid
		}
	}
	// Internal database failures are sanitized by the HTTP adapter.
	return err
}
