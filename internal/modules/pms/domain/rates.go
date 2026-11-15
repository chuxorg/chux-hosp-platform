package domain

import (
	"errors"
	"time"
)

// RoomTypeRate is the current, unadjusted nightly configuration, not a booking quote.
type RoomTypeRate struct {
	PropertyID  int64  `json:"property_id"`
	RoomTypeID  int64  `json:"room_type_id"`
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
	Active      bool   `json:"active"`
}

type EligibleRoom struct {
	Room        Room         `json:"room"`
	NightlyRate RoomTypeRate `json:"nightly_rate"`
}

// StayInterval contains hotel calendar labels, not UTC instants or elapsed hours.
type StayInterval struct{ CheckIn, CheckOut string }

func NewStayInterval(checkIn, checkOut string) (StayInterval, error) {
	start, err := calendarDate(checkIn)
	if err != nil {
		return StayInterval{}, err
	}
	end, err := calendarDate(checkOut)
	if err != nil {
		return StayInterval{}, err
	}
	if !start.Before(end) {
		return StayInterval{}, errors.New("checkout must follow check-in")
	}
	return StayInterval{CheckIn: checkIn, CheckOut: checkOut}, nil
}

func calendarDate(raw string) (time.Time, error) {
	d, err := time.Parse("2006-01-02", raw)
	if err != nil || len(raw) != 10 || d.Year() < 1 {
		return time.Time{}, errors.New("expected a valid YYYY-MM-DD calendar date")
	}
	return d, nil
}

// Eligibility is a read-only inventory snapshot. It does not check or hold reservations.
type Eligibility struct {
	PropertyID                  int64          `json:"property_id"`
	TimeZone                    string         `json:"time_zone"`
	CheckIn                     string         `json:"check_in"`
	CheckOut                    string         `json:"check_out"`
	SearchMode                  string         `json:"search_mode"`
	ReservationConflictsChecked bool           `json:"reservation_conflicts_checked"`
	Rooms                       []EligibleRoom `json:"rooms"`
}
