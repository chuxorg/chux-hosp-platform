-- +goose Up
-- REQ-PMS-001 / TST-PMS-001-01. This is a new greenfield model, not legacy DDL.
CREATE TABLE pms_properties (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name VARCHAR(160) NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 160),
    time_zone VARCHAR(64) NOT NULL CHECK (length(btrim(time_zone)) BETWEEN 1 AND 64),
    is_active BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE pms_room_types (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    property_id BIGINT NOT NULL REFERENCES pms_properties(id) ON DELETE RESTRICT,
    name VARCHAR(100) NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
    max_occupancy INTEGER NOT NULL CHECK (max_occupancy > 0),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    CONSTRAINT pms_room_type_property_identity UNIQUE (id, property_id)
);
CREATE UNIQUE INDEX pms_room_types_property_name_unique
    ON pms_room_types(property_id, lower(btrim(name)));

CREATE TABLE pms_rooms (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    property_id BIGINT NOT NULL,
    room_type_id BIGINT NOT NULL,
    number VARCHAR(64) NOT NULL CHECK (length(btrim(number)) BETWEEN 1 AND 64),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    CONSTRAINT pms_rooms_type_same_property
        FOREIGN KEY (room_type_id, property_id) REFERENCES pms_room_types(id, property_id)
        ON DELETE RESTRICT
);
CREATE UNIQUE INDEX pms_rooms_property_number_unique
    ON pms_rooms(property_id, lower(btrim(number)));

-- +goose Down
DROP TABLE pms_rooms;
DROP TABLE pms_room_types;
DROP TABLE pms_properties;
