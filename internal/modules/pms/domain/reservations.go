package domain

import "time"

const PoolScope = "homogeneous_segment_neutral"

// ReservationPool is an explicit synthetic development attestation, not a
// verified suitability finding, commercial policy, or authorization grant.
type ReservationPool struct {
	PropertyID     int64     `json:"property_id"`
	RoomTypeID     int64     `json:"room_type_id"`
	Scope          string    `json:"scope"`
	Enabled        bool      `json:"enabled"`
	AttestationRef string    `json:"attestation_ref"`
	OperatorRef    string    `json:"operator_ref"`
	Revision       int64     `json:"revision"`
	RecordedAt     time.Time `json:"recorded_at"`
}

type BookingTerms struct {
	Version                        int    `json:"version"`
	Currency                       string `json:"currency"`
	NightlyAmountMinor             int64  `json:"nightly_amount_minor"`
	NightCount                     int    `json:"night_count"`
	AccommodationBaseSubtotalMinor int64  `json:"accommodation_base_subtotal_minor"`
}

// StayReservation consumes pooled nights, without assigning a physical Room.
type StayReservation struct {
	ID                     int64        `json:"id"`
	PropertyID             int64        `json:"property_id"`
	RoomTypeID             int64        `json:"room_type_id"`
	GuestRef               string       `json:"guest_ref"`
	OperatorRef            string       `json:"operator_ref"`
	Status                 string       `json:"status"`
	InventoryBasis         string       `json:"inventory_basis"`
	CheckIn                string       `json:"check_in"`
	CheckOut               string       `json:"check_out"`
	TimeZone               string       `json:"time_zone"`
	BookingTerms           BookingTerms `json:"booking_terms"`
	PoolRevision           int64        `json:"pool_revision"`
	AttestationRef         string       `json:"attestation_ref"`
	CapacityAtConfirmation int64        `json:"capacity_at_confirmation"`
	ConfirmedAt            time.Time    `json:"confirmed_at"`
}
