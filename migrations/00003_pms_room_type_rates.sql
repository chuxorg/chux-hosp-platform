-- +goose Up
-- REQ-PMS-001 / TST-PMS-001-02: owner-approved Issue #23 first slice.
CREATE TABLE pms_room_type_rates (
    room_type_id BIGINT PRIMARY KEY,
    property_id BIGINT NOT NULL,
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    currency VARCHAR(3) NOT NULL CHECK (currency = 'THB'),
    is_active BOOLEAN NOT NULL,
    CONSTRAINT pms_rates_type_same_property
        FOREIGN KEY (room_type_id, property_id) REFERENCES pms_room_types(id, property_id)
        ON DELETE RESTRICT
);

-- +goose Down
DROP TABLE pms_room_type_rates;
