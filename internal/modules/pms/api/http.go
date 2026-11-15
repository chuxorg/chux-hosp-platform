// Package api exposes PMS-owned configuration endpoints.
// All configuration routes are development-only until property-scoped authorization
// and tenant policy are approved in a later story.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/application"
	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/domain"
)

type handler struct { service *application.Service }

// Handler is mounted by the composition root at /pms/v1/.
// It does not import PostgreSQL implementations or other business modules.
func Handler(service *application.Service) http.Handler {
	h := &handler{service: service}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"module":"pms","status":"bootstrap"})
	})
	mux.HandleFunc("POST /properties", h.createProperty)
	mux.HandleFunc("GET /properties/{propertyID}", h.getProperty)
	mux.HandleFunc("PUT /properties/{propertyID}", h.updateProperty)
	mux.HandleFunc("POST /properties/{propertyID}/room-types", h.createRoomType)
	mux.HandleFunc("GET /properties/{propertyID}/room-types/{roomTypeID}", h.getRoomType)
	mux.HandleFunc("PUT /properties/{propertyID}/room-types/{roomTypeID}", h.updateRoomType)
	mux.HandleFunc("POST /properties/{propertyID}/rooms", h.createRoom)
	mux.HandleFunc("GET /properties/{propertyID}/rooms/{roomID}", h.getRoom)
	mux.HandleFunc("PUT /properties/{propertyID}/rooms/{roomID}", h.updateRoom)
	return mux
}

type propertyInput struct {
	Name string `json:"name"`
	TimeZone string `json:"time_zone"`
	Active *bool `json:"active"`
}
type roomTypeInput struct {
	Name string `json:"name"`
	MaxOccupancy int `json:"max_occupancy"`
	Active *bool `json:"active"`
}
type roomInput struct {
	RoomTypeID int64 `json:"room_type_id"`
	Number string `json:"number"`
	Active *bool `json:"active"`
}

func decode[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	var input T
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return input, application.ErrInvalid
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&input); err != nil {
		return input, application.ErrInvalid
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return input, application.ErrInvalid
	}
	return input, nil
}
func pathID(r *http.Request, key string) (int64, error) {
	n, err := strconv.ParseInt(r.PathValue(key), 10, 64)
	if err != nil || n <= 0 { return 0, application.ErrInvalid }
	return n, nil
}
func requiredActive(active *bool) (bool, error) {
	if active == nil { return false, application.ErrInvalid }
	return *active, nil
}
func writeJSON(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, application.ErrInvalid):
		writeJSON(w, 400, map[string]string{"error":"invalid request"})
	case errors.Is(err, application.ErrNotFound):
		writeJSON(w, 404, map[string]string{"error":"not found"})
	case errors.Is(err, application.ErrReference):
		writeJSON(w, 422, map[string]string{"error":"invalid property or room type reference"})
	case errors.Is(err, application.ErrConflict):
		writeJSON(w, 409, map[string]string{"error":"duplicate or conflicting configuration"})
	default:
		// Never expose database/driver details or credentials.
		writeJSON(w, 500, map[string]string{"error":"internal error"})
	}
}

func (h *handler) createProperty(w http.ResponseWriter, r *http.Request) {
	in, err := decode[propertyInput](w,r)
	if err != nil { fail(w, err); return }
	active, err := requiredActive(in.Active)
	if err != nil { fail(w, err); return }
	result, err := h.service.CreateProperty(r.Context(), domain.Property{Name:in.Name, TimeZone:in.TimeZone, Active:active})
	if err != nil { fail(w, err); return }
	w.Header().Set("Location", "/pms/v1/properties/"+strconv.FormatInt(result.ID,10))
	writeJSON(w, 201, result)
}
func (h *handler) getProperty(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r,"propertyID")
	if err != nil { fail(w, err); return }
	result, err := h.service.GetProperty(r.Context(), id)
	if err != nil { fail(w, err); return }
	writeJSON(w, 200, result)
}
func (h *handler) updateProperty(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r,"propertyID")
	if err != nil { fail(w, err); return }
	in, err := decode[propertyInput](w,r)
	if err != nil { fail(w, err); return }
	active, err := requiredActive(in.Active)
	if err != nil { fail(w, err); return }
	result, err := h.service.UpdateProperty(r.Context(), domain.Property{ID:id, Name:in.Name, TimeZone:in.TimeZone, Active:active})
	if err != nil { fail(w, err); return }
	writeJSON(w, 200, result)
}
func (h *handler) createRoomType(w http.ResponseWriter, r *http.Request) {
	propertyID, err := pathID(r,"propertyID")
	if err != nil { fail(w, err); return }
	in, err := decode[roomTypeInput](w,r)
	if err != nil { fail(w, err); return }
	active, err := requiredActive(in.Active)
	if err != nil { fail(w, err); return }
	result, err := h.service.CreateRoomType(r.Context(), domain.RoomType{PropertyID:propertyID, Name:in.Name, MaxOccupancy:in.MaxOccupancy, Active:active})
	if err != nil { fail(w, err); return }
	w.Header().Set("Location", r.URL.Path+"/"+strconv.FormatInt(result.ID,10))
	writeJSON(w, 201, result)
}
func (h *handler) getRoomType(w http.ResponseWriter, r *http.Request) {
	propertyID, err := pathID(r,"propertyID")
	if err != nil { fail(w, err); return }
	id, err := pathID(r,"roomTypeID")
	if err != nil { fail(w, err); return }
	result, err := h.service.GetRoomType(r.Context(), propertyID,id)
	if err != nil { fail(w, err); return }
	writeJSON(w, 200, result)
}
func (h *handler) updateRoomType(w http.ResponseWriter, r *http.Request) {
	propertyID, err := pathID(r,"propertyID")
	if err != nil { fail(w, err); return }
	id, err := pathID(r,"roomTypeID")
	if err != nil { fail(w, err); return }
	in, err := decode[roomTypeInput](w,r)
	if err != nil { fail(w, err); return }
	active, err := requiredActive(in.Active)
	if err != nil { fail(w, err); return }
	result, err := h.service.UpdateRoomType(r.Context(), domain.RoomType{ID:id, PropertyID:propertyID,Name:in.Name, MaxOccupancy:in.MaxOccupancy, Active:active})
	if err != nil { fail(w, err); return }
	writeJSON(w, 200, result)
}
func (h *handler) createRoom(w http.ResponseWriter, r *http.Request) {
	propertyID, err := pathID(r,"propertyID")
	if err != nil { fail(w, err); return }
	in, err := decode[roomInput](w,r)
	if err != nil { fail(w, err); return }
	active, err := requiredActive(in.Active)
	if err != nil { fail(w, err); return }
	result, err := h.service.CreateRoom(r.Context(), domain.Room{PropertyID:propertyID,RoomTypeID:in.RoomTypeID, Number:in.Number, Active:active})
	if err != nil { fail(w, err); return }
	w.Header().Set("Location", r.URL.Path+"/"+strconv.FormatInt(result.ID,10))
	writeJSON(w, 201, result)
}
func (h *handler) getRoom(w http.ResponseWriter, r *http.Request) {
	propertyID, err := pathID(r,"propertyID")
	if err != nil { fail(w, err); return }
	id, err := pathID(r,"roomID")
	if err != nil { fail(w, err); return }
	result, err := h.service.GetRoom(r.Context(), propertyID,id)
	if err != nil { fail(w, err); return }
	writeJSON(w, 200, result)
}
func (h *handler) updateRoom(w http.ResponseWriter, r *http.Request) {
	propertyID, err := pathID(r,"propertyID")
	if err != nil { fail(w, err); return }
	id, err := pathID(r,"roomID")
	if err != nil { fail(w, err); return }
	in, err := decode[roomInput](w,r)
	if err != nil { fail(w, err); return }
	active, err := requiredActive(in.Active)
	if err != nil { fail(w, err); return }
	result, err := h.service.UpdateRoom(r.Context(), domain.Room{ID:id,PropertyID:propertyID,RoomTypeID:in.RoomTypeID, Number:in.Number,Active:active})
	if err != nil { fail(w, err); return }
	writeJSON(w, 200, result)
}
