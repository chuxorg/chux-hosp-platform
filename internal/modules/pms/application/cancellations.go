package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/domain"
)

var (
	ErrAlreadyCancelled   = errors.New("already_cancelled")
	ErrCancellationCutoff = errors.New("cancellation_cutoff_reached")
)

type CancelReservation struct {
	OperatorRef string `json:"operator_ref"`
	ReasonRef   string `json:"reason_ref"`
}

type CancellationCommand struct {
	PropertyID    int64
	ReservationID int64
	Key           string
	Fingerprint   string
	Input         CancelReservation
}

// NormalizeCancellation implements the REQ-PMS-003 version 1 command fingerprint.
// The SQL insertion guard independently verifies the same length-prefixed encoding.
func NormalizeCancellation(p, id int64, key string, in CancelReservation) (CancellationCommand, error) {
	in.OperatorRef = strings.TrimSpace(in.OperatorRef)
	in.ReasonRef = strings.TrimSpace(in.ReasonRef)
	if p <= 0 || id <= 0 || !uuidKey.MatchString(key) || !syntheticRef.MatchString(in.OperatorRef) || !syntheticRef.MatchString(in.ReasonRef) {
		return CancellationCommand{}, ErrInvalid
	}
	semantic := fmt.Sprintf("pms-cancellation:v1:%d:%d:%d:%s:%d:%s", p, id, len(in.OperatorRef), in.OperatorRef, len(in.ReasonRef), in.ReasonRef)
	sum := sha256.Sum256([]byte(semantic))
	return CancellationCommand{PropertyID: p, ReservationID: id, Key: strings.ToLower(key), Fingerprint: hex.EncodeToString(sum[:]), Input: in}, nil
}

func (s *ReservationService) Cancel(ctx context.Context, p, id int64, key string, in CancelReservation) (domain.ReservationCancellation, bool, error) {
	cmd, err := NormalizeCancellation(p, id, key, in)
	if err != nil {
		return domain.ReservationCancellation{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, ReservationDeadline)
	defer cancel()
	return s.store.CancelReservation(ctx, cmd)
}

func (s *ReservationService) GetCancellation(ctx context.Context, p, id int64) (domain.ReservationCancellation, error) {
	if p <= 0 || id <= 0 {
		return domain.ReservationCancellation{}, ErrInvalid
	}
	return s.store.GetReservationCancellation(ctx, p, id)
}
