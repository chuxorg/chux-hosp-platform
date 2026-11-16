package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/domain"
)

// Technical work budgets, not hotel stay/lead-time policies. 366 limits each
// request to 366 ledger inserts and capacity groups; 10s bounds lock/retry work.
const MaxReservationNights = 366
const ReservationDeadline = 10 * time.Second

var (
	ErrPoolRequired        = errors.New("reservation_pool_required")
	ErrInactive            = errors.New("inactive_inventory")
	ErrPriceChanged        = errors.New("price_changed")
	ErrInventoryExhausted  = errors.New("inventory_exhausted")
	ErrIdempotencyConflict = errors.New("idempotency_conflict")
	ErrTechnicalLimit      = errors.New("technical_processing_limit")
	ErrPastDate            = errors.New("past_property_local_date")
	ErrUnavailable         = errors.New("reservation_temporarily_unavailable")
	ErrProtected           = errors.New("protected_reservation_catalog")
)

var syntheticRef = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,79}$`)
var uuidKey = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type ConfirmReservation struct {
	RoomTypeID               int64  `json:"room_type_id"`
	GuestRef                 string `json:"guest_ref"`
	CheckIn                  string `json:"check_in"`
	CheckOut                 string `json:"check_out"`
	ExpectedNightlyRateMinor int64  `json:"expected_nightly_rate_minor"`
	Currency                 string `json:"currency"`
	OperatorRef              string `json:"operator_ref"`
}

// ReservationCommand is already normalized and fingerprinted by the use case.
// The PostgreSQL adapter owns the ENTIRE confirmation/replay transaction.
type ReservationCommand struct {
	PropertyID  int64
	Key         string
	Fingerprint string
	Input       ConfirmReservation
	NightCount  int
}

type ReservationStore interface {
	SetReservationPool(context.Context, domain.ReservationPool) (domain.ReservationPool, error)
	GetReservationPool(context.Context, int64, int64) (domain.ReservationPool, error)
	ConfirmReservation(context.Context, ReservationCommand) (domain.StayReservation, bool, error)
	GetReservation(context.Context, int64, int64) (domain.StayReservation, error)
}

type ReservationService struct{ store ReservationStore }

func NewReservationService(store ReservationStore) *ReservationService {
	return &ReservationService{store: store}
}
func (s *ReservationService) SetPool(ctx context.Context, p domain.ReservationPool) (domain.ReservationPool, error) {
	p.AttestationRef = strings.TrimSpace(p.AttestationRef)
	p.OperatorRef = strings.TrimSpace(p.OperatorRef)
	if p.PropertyID <= 0 || p.RoomTypeID <= 0 || p.Scope != domain.PoolScope || !syntheticRef.MatchString(p.AttestationRef) || !syntheticRef.MatchString(p.OperatorRef) {
		return domain.ReservationPool{}, ErrInvalid
	}
	return s.store.SetReservationPool(ctx, p)
}
func (s *ReservationService) GetPool(ctx context.Context, p, t int64) (domain.ReservationPool, error) {
	if p <= 0 || t <= 0 {
		return domain.ReservationPool{}, ErrInvalid
	}
	return s.store.GetReservationPool(ctx, p, t)
}
func (s *ReservationService) Get(ctx context.Context, p, id int64) (domain.StayReservation, error) {
	if p <= 0 || id <= 0 {
		return domain.StayReservation{}, ErrInvalid
	}
	return s.store.GetReservation(ctx, p, id)
}
func (s *ReservationService) Confirm(ctx context.Context, p int64, key string, in ConfirmReservation) (domain.StayReservation, bool, error) {
	cmd, err := NormalizeReservation(p, key, in)
	if err != nil {
		return domain.StayReservation{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, ReservationDeadline)
	defer cancel()
	return s.store.ConfirmReservation(ctx, cmd)
}

func NormalizeReservation(p int64, key string, in ConfirmReservation) (ReservationCommand, error) {
	in.GuestRef = strings.TrimSpace(in.GuestRef)
	in.OperatorRef = strings.TrimSpace(in.OperatorRef)
	if p <= 0 || in.RoomTypeID <= 0 || !uuidKey.MatchString(key) || !syntheticRef.MatchString(in.GuestRef) || !syntheticRef.MatchString(in.OperatorRef) || in.Currency != "THB" || in.ExpectedNightlyRateMinor <= 0 {
		return ReservationCommand{}, ErrInvalid
	}
	if _, err := domain.NewStayInterval(in.CheckIn, in.CheckOut); err != nil {
		return ReservationCommand{}, ErrInvalid
	}
	start, _ := time.Parse("2006-01-02", in.CheckIn)
	end, _ := time.Parse("2006-01-02", in.CheckOut)
	// Unix seconds avoid time.Duration saturation for distant date labels.
	nights := (end.Unix() - start.Unix()) / 86400
	if nights > MaxReservationNights {
		return ReservationCommand{}, ErrTechnicalLimit
	}
	b, _ := json.Marshal(in)
	sum := sha256.Sum256(b)
	return ReservationCommand{PropertyID: p, Key: strings.ToLower(key), Input: in, NightCount: int(nights), Fingerprint: hex.EncodeToString(sum[:])}, nil
}
