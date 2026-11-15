package application

import (
	"context"
	"errors"
	"testing"

	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/domain"
)

type fakeStore struct {
	props     []domain.Property
	roomTypes []domain.RoomType
	rooms     []domain.Room
	calls     int
	rates     []domain.RoomTypeRate
	eligible  []domain.EligibleRoom
	rateErr   error
}

func (f *fakeStore) CreateProperty(_ context.Context, p domain.Property) (domain.Property, error) {
	f.calls++
	p.ID = int64(len(f.props) + 1)
	f.props = append(f.props, p)
	return p, nil
}
func (f *fakeStore) GetProperty(_ context.Context, id int64) (domain.Property, error) {
	for _, p := range f.props {
		if p.ID == id {
			return p, nil
		}
	}
	return domain.Property{}, ErrNotFound
}
func (f *fakeStore) UpdateProperty(_ context.Context, p domain.Property) (domain.Property, error) {
	for i, v := range f.props {
		if v.ID == p.ID {
			f.props[i] = p
			return p, nil
		}
	}
	return domain.Property{}, ErrNotFound
}
func (f *fakeStore) CreateRoomType(_ context.Context, r domain.RoomType) (domain.RoomType, error) {
	f.calls++
	r.ID = int64(len(f.roomTypes) + 1)
	f.roomTypes = append(f.roomTypes, r)
	return r, nil
}
func (f *fakeStore) GetRoomType(_ context.Context, prop, id int64) (domain.RoomType, error) {
	for _, r := range f.roomTypes {
		if r.PropertyID == prop && r.ID == id {
			return r, nil
		}
	}
	return domain.RoomType{}, ErrNotFound
}
func (f *fakeStore) UpdateRoomType(_ context.Context, r domain.RoomType) (domain.RoomType, error) {
	for i, v := range f.roomTypes {
		if v.PropertyID == r.PropertyID && v.ID == r.ID {
			f.roomTypes[i] = r
			return r, nil
		}
	}
	return domain.RoomType{}, ErrNotFound
}
func (f *fakeStore) CreateRoom(_ context.Context, r domain.Room) (domain.Room, error) {
	f.calls++
	r.ID = int64(len(f.rooms) + 1)
	f.rooms = append(f.rooms, r)
	return r, nil
}
func (f *fakeStore) GetRoom(_ context.Context, prop, id int64) (domain.Room, error) {
	for _, r := range f.rooms {
		if r.PropertyID == prop && r.ID == id {
			return r, nil
		}
	}
	return domain.Room{}, ErrNotFound
}
func (f *fakeStore) UpdateRoom(_ context.Context, r domain.Room) (domain.Room, error) {
	for i, v := range f.rooms {
		if v.PropertyID == r.PropertyID && v.ID == r.ID {
			f.rooms[i] = r
			return r, nil
		}
	}
	return domain.Room{}, ErrNotFound
}

var _ Store = (*fakeStore)(nil)

// TST-PMS-001-01: a typed Store port can be satisfied with a fake, with no PG dependency.
func TestServiceWithFakeStore(t *testing.T) {
	ctx := context.Background()
	f := &fakeStore{}
	s := NewService(f)
	for _, bad := range []domain.Property{
		{Name: "", TimeZone: "Asia/Bangkok"},
		{Name: "Property", TimeZone: "Invalid/Zone"},
		{Name: "Property", TimeZone: "Local"},
	} {
		if _, err := s.CreateProperty(ctx, bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted bad property %+v: %v", bad, err)
		}
	}
	if f.calls != 0 {
		t.Fatal("invalid records reached persistence")
	}
	p, err := s.CreateProperty(ctx, domain.Property{Name: "  Pilot Hotel ", TimeZone: "Asia/Bangkok", Active: true})
	if err != nil || p.ID != 1 || p.Name != "Pilot Hotel" {
		t.Fatalf("property: %+v, %v", p, err)
	}
	got, err := s.GetProperty(ctx, p.ID)
	if err != nil || got != p {
		t.Fatalf("get property: %+v, %v", got, err)
	}
	p.Name = "Pilot Hotel 2"
	p.TimeZone = "UTC"
	p.Active = false
	got, err = s.UpdateProperty(ctx, p)
	if err != nil || got != p {
		t.Fatalf("update property: %+v, %v", got, err)
	}
	if _, err = s.GetProperty(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Fatal("missing property not reported")
	}
	if _, err = s.GetProperty(ctx, -1); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid property ID accepted")
	}
	for _, bad := range []domain.RoomType{
		{PropertyID: 0, Name: "Suite", MaxOccupancy: 2},
		{PropertyID: p.ID, Name: " ", MaxOccupancy: 2},
		{PropertyID: p.ID, Name: "Suite", MaxOccupancy: 0},
		{PropertyID: p.ID, Name: "Suite", MaxOccupancy: -1},
	} {
		if _, err := s.CreateRoomType(ctx, bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted bad room type %+v", bad)
		}
	}
	typ, err := s.CreateRoomType(ctx, domain.RoomType{PropertyID: p.ID, Name: "  Standard ", MaxOccupancy: 2, Active: true})
	if err != nil || typ.ID != 1 || typ.Name != "Standard" {
		t.Fatalf("room type %+v %v", typ, err)
	}
	typ.MaxOccupancy = 3
	typ.Active = false
	gotType, err := s.UpdateRoomType(ctx, typ)
	if err != nil || gotType != typ {
		t.Fatalf("type update %+v %v", gotType, err)
	}
	if _, err = s.GetRoomType(ctx, p.ID, 1); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []domain.Room{
		{PropertyID: p.ID, RoomTypeID: 0, Number: "101"},
		{PropertyID: 0, RoomTypeID: typ.ID, Number: "101"},
		{PropertyID: p.ID, RoomTypeID: typ.ID, Number: ""},
	} {
		if _, err := s.CreateRoom(ctx, bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted bad room %+v", bad)
		}
	}
	room, err := s.CreateRoom(ctx, domain.Room{PropertyID: p.ID, RoomTypeID: typ.ID, Number: "  A101 ", Active: true})
	if err != nil || room.ID != 1 || room.Number != "A101" {
		t.Fatalf("room %+v %v", room, err)
	}
	room.Number = "A102"
	room.Active = false
	gotRoom, err := s.UpdateRoom(ctx, room)
	if err != nil || gotRoom != room {
		t.Fatalf("update room %+v %v", gotRoom, err)
	}
	if _, err = s.GetRoom(ctx, p.ID, room.ID); err != nil {
		t.Fatal(err)
	}
	if f.calls != 3 {
		t.Fatalf("unexpected store calls %d", f.calls)
	}
}
