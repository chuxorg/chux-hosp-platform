package api

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/application"
	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/domain"
)

type rateInput struct {
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
	Active      *bool  `json:"active"`
}

func (h *handler) setRoomTypeRate(w http.ResponseWriter, r *http.Request) {
	propertyID, err := pathID(r, "propertyID")
	if err != nil {
		fail(w, err)
		return
	}
	roomTypeID, err := pathID(r, "roomTypeID")
	if err != nil {
		fail(w, err)
		return
	}
	in, err := decode[rateInput](w, r)
	if err != nil {
		fail(w, err)
		return
	}
	active, err := requiredActive(in.Active)
	if err != nil {
		fail(w, err)
		return
	}
	rate, created, err := h.service.SetRoomTypeRate(r.Context(), domain.RoomTypeRate{PropertyID: propertyID, RoomTypeID: roomTypeID, AmountMinor: in.AmountMinor, Currency: in.Currency, Active: active})
	if err != nil {
		fail(w, err)
		return
	}
	code := http.StatusOK
	if created {
		code = http.StatusCreated
		w.Header().Set("Location", fmt.Sprintf("/pms/v1/properties/%d/room-types/%d/rate", propertyID, roomTypeID))
	}
	writeJSON(w, code, rate)
}

func (h *handler) getRoomTypeRate(w http.ResponseWriter, r *http.Request) {
	propertyID, err := pathID(r, "propertyID")
	if err != nil {
		fail(w, err)
		return
	}
	roomTypeID, err := pathID(r, "roomTypeID")
	if err != nil {
		fail(w, err)
		return
	}
	rate, err := h.service.GetRoomTypeRate(r.Context(), propertyID, roomTypeID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rate)
}

func (h *handler) roomEligibility(w http.ResponseWriter, r *http.Request) {
	propertyID, err := pathID(r, "propertyID")
	if err != nil {
		fail(w, err)
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	// Reject ambiguous duplicate, missing and unknown parameters rather than
	// silently selecting one date or pretending to implement additional filters.
	if err != nil || len(query) != 2 || len(query["check_in"]) != 1 || len(query["check_out"]) != 1 {
		err = application.ErrInvalid
	}
	if err != nil {
		fail(w, err)
		return
	}
	result, err := h.service.RoomEligibility(r.Context(), propertyID, query.Get("check_in"), query.Get("check_out"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
