package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/domain"
)

type reservationFake struct {
	calls   int
	command ReservationCommand
}

func (f *reservationFake) SetReservationPool(_ context.Context, p domain.ReservationPool) (domain.ReservationPool, error) {
	f.calls++
	return p, nil
}
func (f *reservationFake) GetReservationPool(context.Context, int64, int64) (domain.ReservationPool, error) {
	f.calls++
	return domain.ReservationPool{}, nil
}
func (f *reservationFake) GetReservation(context.Context, int64, int64) (domain.StayReservation, error) {
	f.calls++
	return domain.StayReservation{}, nil
}
func (f *reservationFake) ConfirmReservation(ctx context.Context, c ReservationCommand) (domain.StayReservation, bool, error) {
	f.calls++
	f.command = c
	if _, ok := ctx.Deadline(); !ok {
		panic("confirmation requires a deadline")
	}
	return domain.StayReservation{ID: 7}, false, nil
}

func (f *reservationFake) CancelReservation(ctx context.Context, c CancellationCommand) (domain.ReservationCancellation, bool, error) {
	f.calls++
	if _, ok := ctx.Deadline(); !ok {
		panic("cancellation requires a deadline")
	}
	return domain.ReservationCancellation{ReservationID: c.ReservationID}, false, nil
}
func (f *reservationFake) GetReservationCancellation(context.Context, int64, int64) (domain.ReservationCancellation, error) {
	f.calls++
	return domain.ReservationCancellation{}, nil
}

func validConfirmation() ConfirmReservation {
	return ConfirmReservation{RoomTypeID: 2, GuestRef: "SYN-GUEST-1", OperatorRef: "SYN-OP-1", CheckIn: "2028-02-28", CheckOut: "2028-03-01", Currency: "THB", ExpectedNightlyRateMinor: 12500}
}

const testKey = "de305d54-75b4-431b-adb2-eb6b9e546014"

// REQ-PMS-002 / TST-PMS-001-03: typed transaction port, normalized command and
// bounded calendar work. Price multiplication/date-now checks remain in the tx.
func TestReservationUseCase(t *testing.T) {
	f := &reservationFake{}
	s := NewReservationService(f)
	in := validConfirmation()
	got, replay, err := s.Confirm(context.Background(), 1, strings.ToUpper(testKey), in)
	if err != nil || replay || got.ID != 7 || f.calls != 1 || f.command.NightCount != 2 || f.command.Key != testKey {
		t.Fatalf("command: %+v %v", f.command, err)
	}
	original := f.command.Fingerprint
	in.GuestRef = "  SYN-GUEST-1 "
	_, _, _ = s.Confirm(context.Background(), 1, testKey, in)
	if f.command.Fingerprint != original {
		t.Fatal("semantic whitespace changed fingerprint")
	}
	in.OperatorRef = "SYN-OP-2"
	_, _, _ = s.Confirm(context.Background(), 1, testKey, in)
	if f.command.Fingerprint == original {
		t.Fatal("operator omitted from fingerprint")
	}
	for _, tc := range []struct {
		name string
		edit func(*ConfirmReservation)
		want error
	}{
		{"zero type", func(v *ConfirmReservation) { v.RoomTypeID = 0 }, ErrInvalid},
		{"PII shaped reference", func(v *ConfirmReservation) { v.GuestRef = "guest@example.com" }, ErrInvalid},
		{"oversized ref", func(v *ConfirmReservation) { v.OperatorRef = strings.Repeat("x", 81) }, ErrInvalid},
		{"zero price", func(v *ConfirmReservation) { v.ExpectedNightlyRateMinor = 0 }, ErrInvalid},
		{"currency", func(v *ConfirmReservation) { v.Currency = "USD" }, ErrInvalid},
		{"invalid date", func(v *ConfirmReservation) { v.CheckIn = "2028-02-30" }, ErrInvalid},
		{"reversed", func(v *ConfirmReservation) { v.CheckOut = v.CheckIn }, ErrInvalid},
		{"technical bound", func(v *ConfirmReservation) { v.CheckOut = "2030-01-01" }, ErrTechnicalLimit},
		{"duration saturation", func(v *ConfirmReservation) { v.CheckIn = "0001-01-01"; v.CheckOut = "9999-01-01" }, ErrTechnicalLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := validConfirmation()
			tc.edit(&v)
			before := f.calls
			_, _, e := s.Confirm(context.Background(), 1, testKey, v)
			if !errors.Is(e, tc.want) || f.calls != before {
				t.Fatalf("invalid reached store: %v", e)
			}
		})
	}
	for _, key := range []string{"", "not-a-key", testKey + ", " + testKey} {
		if _, _, e := s.Confirm(context.Background(), 1, key, validConfirmation()); !errors.Is(e, ErrInvalid) {
			t.Fatal("invalid key")
		}
	}
	p := domain.ReservationPool{PropertyID: 1, RoomTypeID: 2, Scope: domain.PoolScope, AttestationRef: " SYN-ATTEST ", OperatorRef: "SYN-OP"}
	if got, e := s.SetPool(context.Background(), p); e != nil || got.AttestationRef != "SYN-ATTEST" {
		t.Fatal("pool normalization")
	}
	p.Scope = "market_segment"
	if _, e := s.SetPool(context.Background(), p); !errors.Is(e, ErrInvalid) {
		t.Fatal("unsupported pool")
	}
}
