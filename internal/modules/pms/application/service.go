// Package application owns PMS configuration validation and use cases.
package application

import (
	"context"
	"errors"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"

	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/domain"
)

var (
	ErrInvalid   = errors.New("invalid PMS configuration")
	ErrNotFound  = errors.New("PMS configuration not found")
	ErrConflict  = errors.New("PMS configuration conflicts with existing record")
	ErrReference = errors.New("invalid or cross-property PMS reference")
)

// Store is a consumer-owned, typed persistence port. PostgreSQL is an adapter,
// not a dependency of PMS application/domain logic.
type Store interface {
	CreateProperty(context.Context, domain.Property) (domain.Property, error)
	GetProperty(context.Context, int64) (domain.Property, error)
	UpdateProperty(context.Context, domain.Property) (domain.Property, error)
	CreateRoomType(context.Context, domain.RoomType) (domain.RoomType, error)
	GetRoomType(context.Context, int64, int64) (domain.RoomType, error)
	UpdateRoomType(context.Context, domain.RoomType) (domain.RoomType, error)
	CreateRoom(context.Context, domain.Room) (domain.Room, error)
	GetRoom(context.Context, int64, int64) (domain.Room, error)
	UpdateRoom(context.Context, domain.Room) (domain.Room, error)
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) CreateProperty(ctx context.Context, p domain.Property) (domain.Property, error) {
	if err := validateProperty(&p); err != nil {
		return domain.Property{}, err
	}
	p.ID = 0
	return s.store.CreateProperty(ctx, p)
}
func (s *Service) GetProperty(ctx context.Context, id int64) (domain.Property, error) {
	if id <= 0 {
		return domain.Property{}, ErrInvalid
	}
	return s.store.GetProperty(ctx, id)
}
func (s *Service) UpdateProperty(ctx context.Context, p domain.Property) (domain.Property, error) {
	if p.ID <= 0 {
		return domain.Property{}, ErrInvalid
	}
	if err := validateProperty(&p); err != nil {
		return domain.Property{}, err
	}
	return s.store.UpdateProperty(ctx, p)
}
func validateProperty(p *domain.Property) error {
	p.Name = strings.TrimSpace(p.Name)
	p.TimeZone = strings.TrimSpace(p.TimeZone)
	if !nonempty(p.Name, 160) || !nonempty(p.TimeZone, 64) || p.TimeZone == "Local" {
		return ErrInvalid
	}
	if _, err := time.LoadLocation(p.TimeZone); err != nil {
		return ErrInvalid
	}
	return nil
}
func (s *Service) CreateRoomType(ctx context.Context, t domain.RoomType) (domain.RoomType, error) {
	if err := validateRoomType(&t); err != nil {
		return domain.RoomType{}, err
	}
	t.ID = 0
	return s.store.CreateRoomType(ctx, t)
}
func (s *Service) GetRoomType(ctx context.Context, propertyID, id int64) (domain.RoomType, error) {
	if propertyID <= 0 || id <= 0 {
		return domain.RoomType{}, ErrInvalid
	}
	return s.store.GetRoomType(ctx, propertyID, id)
}
func (s *Service) UpdateRoomType(ctx context.Context, t domain.RoomType) (domain.RoomType, error) {
	if t.ID <= 0 {
		return domain.RoomType{}, ErrInvalid
	}
	if err := validateRoomType(&t); err != nil {
		return domain.RoomType{}, err
	}
	return s.store.UpdateRoomType(ctx, t)
}
func validateRoomType(t *domain.RoomType) error {
	t.Name = strings.TrimSpace(t.Name)
	if t.PropertyID <= 0 || !nonempty(t.Name, 100) || t.MaxOccupancy < 1 || int64(t.MaxOccupancy) > 2147483647 {
		return ErrInvalid
	}
	return nil
}
func (s *Service) CreateRoom(ctx context.Context, r domain.Room) (domain.Room, error) {
	if err := validateRoom(&r); err != nil {
		return domain.Room{}, err
	}
	r.ID = 0
	return s.store.CreateRoom(ctx, r)
}
func (s *Service) GetRoom(ctx context.Context, propertyID, id int64) (domain.Room, error) {
	if propertyID <= 0 || id <= 0 {
		return domain.Room{}, ErrInvalid
	}
	return s.store.GetRoom(ctx, propertyID, id)
}
func (s *Service) UpdateRoom(ctx context.Context, r domain.Room) (domain.Room, error) {
	if r.ID <= 0 {
		return domain.Room{}, ErrInvalid
	}
	if err := validateRoom(&r); err != nil {
		return domain.Room{}, err
	}
	return s.store.UpdateRoom(ctx, r)
}
func validateRoom(r *domain.Room) error {
	r.Number = strings.TrimSpace(r.Number)
	if r.PropertyID <= 0 || r.RoomTypeID <= 0 || !nonempty(r.Number, 64) {
		return ErrInvalid
	}
	return nil
}
func nonempty(s string, max int) bool {
	n := utf8.RuneCountInString(s)
	return n > 0 && n <= max
}
