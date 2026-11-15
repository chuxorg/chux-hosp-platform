package application

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/domain"
)

func (f *fakeStore) SetRoomTypeRate(ctx context.Context, r domain.RoomTypeRate) (domain.RoomTypeRate, bool, error) {
	f.calls++
	if f.rateErr != nil {
		return domain.RoomTypeRate{}, false, f.rateErr
	}
	if _, err := f.GetRoomType(ctx, r.PropertyID, r.RoomTypeID); err != nil {
		return domain.RoomTypeRate{}, false, ErrReference
	}
	for i, existing := range f.rates {
		if existing.RoomTypeID == r.RoomTypeID {
			f.rates[i] = r
			return r, false, nil
		}
	}
	f.rates = append(f.rates, r)
	return r, true, nil
}
func (f *fakeStore) GetRoomTypeRate(_ context.Context, p, id int64) (domain.RoomTypeRate, error) {
	if f.rateErr != nil {
		return domain.RoomTypeRate{}, f.rateErr
	}
	for _, r := range f.rates {
		if r.PropertyID == p && r.RoomTypeID == id {
			return r, nil
		}
	}
	return domain.RoomTypeRate{}, ErrNotFound
}
func (f *fakeStore) EligibleRooms(_ context.Context, _ int64) ([]domain.EligibleRoom, error) {
	return f.eligible, f.rateErr
}

// TST-PMS-001-02 / REQ-PMS-001: accepted input policy, inactive preparation and typed port.
func TestRateService(t *testing.T) {
	ctx := context.Background()
	f := &fakeStore{props: []domain.Property{{ID: 1, TimeZone: "Asia/Bangkok", Active: false}}, roomTypes: []domain.RoomType{{ID: 2, PropertyID: 1, Active: false}}}
	s := NewService(f)
	valid := domain.RoomTypeRate{PropertyID: 1, RoomTypeID: 2, AmountMinor: 12345, Currency: "THB", Active: true}
	for _, bad := range []domain.RoomTypeRate{
		{PropertyID: 1, RoomTypeID: 2, AmountMinor: 0, Currency: "THB"},
		{PropertyID: 1, RoomTypeID: 2, AmountMinor: -1, Currency: "THB"},
		{PropertyID: 1, RoomTypeID: 2, AmountMinor: 100, Currency: "USD"},
		{PropertyID: 1, RoomTypeID: 2, AmountMinor: 100, Currency: "thb"},
		{PropertyID: 1, RoomTypeID: 2, AmountMinor: 100, Currency: " THB"},
		{PropertyID: 0, RoomTypeID: 2, AmountMinor: 100, Currency: "THB"},
		{PropertyID: 1, RoomTypeID: 0, AmountMinor: 100, Currency: "THB"},
	} {
		if _, _, err := s.SetRoomTypeRate(ctx, bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted invalid rate: %+v %v", bad, err)
		}
	}
	if f.calls != 0 {
		t.Fatal("invalid rate reached persistence")
	}
	got, created, err := s.SetRoomTypeRate(ctx, valid)
	if err != nil || !created || got != valid {
		t.Fatalf("inactive-parent preparation: %+v %v %v", got, created, err)
	}
	got, err = s.GetRoomTypeRate(ctx, 1, 2)
	if err != nil || got != valid {
		t.Fatal("read roundtrip failed")
	}
	valid.AmountMinor = math.MaxInt64
	valid.Active = false
	got, created, err = s.SetRoomTypeRate(ctx, valid)
	if err != nil || created || got != valid {
		t.Fatal("replacement/max int64/deactivation failed")
	}
	for _, ids := range [][2]int64{{0, 2}, {1, -1}} {
		if _, err := s.GetRoomTypeRate(ctx, ids[0], ids[1]); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid read IDs accepted")
		}
	}
	if _, err := s.GetRoomTypeRate(ctx, 9, 2); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-property read succeeded")
	}
	other := valid
	other.PropertyID = 9
	if _, _, err := s.SetRoomTypeRate(ctx, other); !errors.Is(err, ErrReference) {
		t.Fatal("reference failure lost")
	}
	f.rateErr = errors.New("storage failure")
	if _, _, err := s.SetRoomTypeRate(ctx, valid); !errors.Is(err, f.rateErr) {
		t.Fatal("write error lost")
	}
	if _, err := s.GetRoomTypeRate(ctx, 1, 2); !errors.Is(err, f.rateErr) {
		t.Fatal("read error lost")
	}
}

func TestEligibilityService(t *testing.T) {
	ctx := context.Background()
	f := &fakeStore{props: []domain.Property{{ID: 1, TimeZone: "Asia/Bangkok"}, {ID: 2, TimeZone: "America/New_York"}}}
	s := NewService(f)
	for _, id := range []int64{1, 2} {
		got, err := s.RoomEligibility(ctx, id, "2026-03-07", "2026-03-09")
		if err != nil || got.PropertyID != id || got.TimeZone != f.props[id-1].TimeZone || got.CheckIn != "2026-03-07" || got.CheckOut != "2026-03-09" || got.SearchMode != "inventory_eligibility" || got.ReservationConflictsChecked || got.Rooms == nil || len(got.Rooms) != 0 {
			t.Fatalf("bad eligibility metadata/empty list: %+v %v", got, err)
		}
	}
	for _, tt := range []struct {
		id      int64
		in, out string
	}{{0, "2026-01-01", "2026-01-02"}, {1, "2026-01-02", "2026-01-01"}, {1, "bad", "2026-01-01"}} {
		if _, err := s.RoomEligibility(ctx, tt.id, tt.in, tt.out); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid search accepted")
		}
	}
	if _, err := s.RoomEligibility(ctx, 999, "2026-01-01", "2026-01-02"); !errors.Is(err, ErrNotFound) {
		t.Fatal("missing property accepted")
	}
	f.rateErr = errors.New("query failed")
	if _, err := s.RoomEligibility(ctx, 1, "2026-01-01", "2026-01-02"); !errors.Is(err, f.rateErr) {
		t.Fatal("query error lost")
	}
}
