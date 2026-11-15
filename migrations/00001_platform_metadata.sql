-- +goose Up
CREATE TABLE platform_metadata (
    key VARCHAR(80) PRIMARY KEY,
    value VARCHAR(250) NOT NULL
);

-- +goose Down
DROP TABLE platform_metadata;
