package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/application"
	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/domain"
)

func registerReservations(mux *http.ServeMux, s *application.ReservationService) {
	registerCancellations(mux, s)
	mux.HandleFunc("PUT /properties/{propertyID}/room-types/{roomTypeID}/reservation-pool", func(w http.ResponseWriter, r *http.Request) {
		p, err := pathID(r, "propertyID")
		if err != nil {
			fail(w, err)
			return
		}
		t, err := pathID(r, "roomTypeID")
		if err != nil {
			fail(w, err)
			return
		}
		in, err := decode[struct {
			Scope          string `json:"scope"`
			Enabled        *bool  `json:"enabled"`
			AttestationRef string `json:"attestation_ref"`
			OperatorRef    string `json:"operator_ref"`
		}](w, r)
		if err != nil || in.Enabled == nil {
			fail(w, application.ErrInvalid)
			return
		}
		result, err := s.SetPool(r.Context(), domain.ReservationPool{PropertyID: p, RoomTypeID: t, Scope: in.Scope, Enabled: *in.Enabled, AttestationRef: in.AttestationRef, OperatorRef: in.OperatorRef})
		if err != nil {
			reservationFail(w, err)
			return
		}
		writeJSON(w, 200, result)
	})
	mux.HandleFunc("GET /properties/{propertyID}/room-types/{roomTypeID}/reservation-pool", func(w http.ResponseWriter, r *http.Request) {
		p, err := pathID(r, "propertyID")
		if err != nil {
			fail(w, err)
			return
		}
		t, err := pathID(r, "roomTypeID")
		if err != nil {
			fail(w, err)
			return
		}
		result, err := s.GetPool(r.Context(), p, t)
		if err != nil {
			reservationFail(w, err)
			return
		}
		writeJSON(w, 200, result)
	})
	mux.HandleFunc("POST /properties/{propertyID}/stay-reservations", func(w http.ResponseWriter, r *http.Request) {
		p, err := pathID(r, "propertyID")
		if err != nil {
			fail(w, err)
			return
		}
		in, err := decode[application.ConfirmReservation](w, r)
		if err != nil {
			fail(w, err)
			return
		}
		// Reject multiple keys instead of choosing an ambiguous header value.
		keys := r.Header.Values("Idempotency-Key")
		if len(keys) != 1 {
			fail(w, application.ErrInvalid)
			return
		}
		result, replay, err := s.Confirm(r.Context(), p, keys[0], in)
		if err != nil {
			reservationFail(w, err)
			return
		}
		code := 201
		if replay {
			code = 200
			w.Header().Set("Idempotency-Replayed", "true")
		}
		w.Header().Set("Location", "/pms/v1/properties/"+strconv.FormatInt(p, 10)+"/stay-reservations/"+strconv.FormatInt(result.ID, 10))
		writeJSON(w, code, result)
	})
	mux.HandleFunc("GET /properties/{propertyID}/stay-reservations/{reservationID}", func(w http.ResponseWriter, r *http.Request) {
		p, err := pathID(r, "propertyID")
		if err != nil {
			fail(w, err)
			return
		}
		id, err := pathID(r, "reservationID")
		if err != nil {
			fail(w, err)
			return
		}
		result, err := s.Get(r.Context(), p, id)
		if err != nil {
			reservationFail(w, err)
			return
		}
		writeJSON(w, 200, result)
	})
}
func reservationFail(w http.ResponseWriter, err error) {
	code := 0
	switch {
	case errors.Is(err, application.ErrPoolRequired):
		code = 412
	case errors.Is(err, application.ErrInactive), errors.Is(err, application.ErrPriceChanged), errors.Is(err, application.ErrInventoryExhausted), errors.Is(err, application.ErrIdempotencyConflict), errors.Is(err, application.ErrProtected):
		code = 409
	case errors.Is(err, application.ErrAlreadyCancelled), errors.Is(err, application.ErrCancellationCutoff):
		code = 409
	case errors.Is(err, application.ErrTechnicalLimit), errors.Is(err, application.ErrPastDate):
		code = 422
	case errors.Is(err, application.ErrUnavailable):
		code = 503
		w.Header().Set("Retry-After", "1")
	}
	if code == 0 {
		fail(w, err)
		return
	}
	writeJSON(w, code, map[string]string{"error": err.Error()})
}
