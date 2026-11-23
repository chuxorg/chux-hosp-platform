package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/application"
)

func registerCancellations(mux *http.ServeMux, s *application.ReservationService) {
	const path = "/properties/{propertyID}/stay-reservations/{reservationID}/cancellation"
	mux.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
		p, id, err := cancellationTarget(r)
		if err != nil {
			fail(w, err)
			return
		}
		event, err := s.GetCancellation(r.Context(), p, id)
		if err != nil {
			reservationFail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, event)
	})
	mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
		p, id, err := cancellationTarget(r)
		if err != nil {
			fail(w, err)
			return
		}
		keys := r.Header.Values("Idempotency-Key")
		if len(keys) != 1 {
			fail(w, application.ErrInvalid)
			return
		}
		input, err := decode[application.CancelReservation](w, r)
		if err != nil {
			fail(w, err)
			return
		}
		location := "/pms/v1/properties/" + strconv.FormatInt(p, 10) + "/stay-reservations/" + strconv.FormatInt(id, 10) + "/cancellation"
		input.OperatorRef = operatorRef(r, input.OperatorRef)
		event, replay, err := s.Cancel(r.Context(), p, id, keys[0], input)
		if err != nil {
			if errors.Is(err, application.ErrAlreadyCancelled) {
				w.Header().Set("Location", location)
			}
			reservationFail(w, err)
			return
		}
		code := http.StatusCreated
		if replay {
			code = http.StatusOK
			w.Header().Set("Idempotency-Replayed", "true")
		}
		w.Header().Set("Location", location)
		writeJSON(w, code, event)
	})
}

func cancellationTarget(r *http.Request) (int64, int64, error) {
	p, err := pathID(r, "propertyID")
	if err != nil {
		return 0, 0, err
	}
	id, err := pathID(r, "reservationID")
	return p, id, err
}
