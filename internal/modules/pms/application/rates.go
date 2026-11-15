package application

import (
	"context"
	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/domain"
)

// SetRoomTypeRate permits configuration under inactive parents. Persistence enforces ownership.
func (s *Service) SetRoomTypeRate(ctx context.Context, rate domain.RoomTypeRate) (domain.RoomTypeRate, bool, error) {
	if rate.PropertyID <= 0 || rate.RoomTypeID <= 0 || rate.AmountMinor <= 0 || rate.Currency != "THB" {
		return domain.RoomTypeRate{}, false, ErrInvalid
	}
	return s.store.SetRoomTypeRate(ctx, rate)
}

func (s *Service) GetRoomTypeRate(ctx context.Context, propertyID, roomTypeID int64) (domain.RoomTypeRate, error) {
	if propertyID <= 0 || roomTypeID <= 0 {
		return domain.RoomTypeRate{}, ErrInvalid
	}
	return s.store.GetRoomTypeRate(ctx, propertyID, roomTypeID)
}

func (s *Service) RoomEligibility(ctx context.Context, propertyID int64, checkIn, checkOut string) (domain.Eligibility, error) {
	if propertyID <= 0 {
		return domain.Eligibility{}, ErrInvalid
	}
	interval, err := domain.NewStayInterval(checkIn, checkOut)
	if err != nil {
		return domain.Eligibility{}, ErrInvalid
	}
	property, err := s.store.GetProperty(ctx, propertyID)
	if err != nil {
		return domain.Eligibility{}, err
	}
	rooms, err := s.store.EligibleRooms(ctx, propertyID)
	if err != nil {
		return domain.Eligibility{}, err
	}
	if rooms == nil {
		rooms = make([]domain.EligibleRoom, 0)
	}
	return domain.Eligibility{PropertyID: propertyID, TimeZone: property.TimeZone, CheckIn: interval.CheckIn, CheckOut: interval.CheckOut, SearchMode: "inventory_eligibility", ReservationConflictsChecked: false, Rooms: rooms}, nil
}
