// Package domain defines PMS-owned configuration data without persistence or HTTP.
package domain

type Property struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	TimeZone string `json:"time_zone"`
	Active   bool   `json:"active"`
}

type RoomType struct {
	ID           int64  `json:"id"`
	PropertyID   int64  `json:"property_id"`
	Name         string `json:"name"`
	MaxOccupancy int    `json:"max_occupancy"`
	Active       bool   `json:"active"`
}

// Room denotes an individually addressable accommodation unit, not a room rate.
type Room struct {
	ID         int64  `json:"id"`
	PropertyID int64  `json:"property_id"`
	RoomTypeID int64  `json:"room_type_id"`
	Number     string `json:"number"`
	Active     bool   `json:"active"`
}
