-- +goose Up
CREATE TABLE platform_access_groups (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name TEXT NOT NULL UNIQUE CHECK (btrim(name) <> '')
);

CREATE TABLE platform_access_roles (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name TEXT NOT NULL UNIQUE CHECK (btrim(name) <> '')
);

CREATE TABLE platform_group_memberships (
    group_id BIGINT NOT NULL REFERENCES platform_access_groups(id),
    user_id BIGINT NOT NULL REFERENCES platform_users(id),
    PRIMARY KEY (group_id, user_id)
);

CREATE TABLE platform_group_role_assignments (
    group_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    role_id BIGINT NOT NULL REFERENCES platform_access_roles(id),
    PRIMARY KEY (group_id, user_id, role_id),
    FOREIGN KEY (group_id, user_id)
        REFERENCES platform_group_memberships(group_id, user_id) ON DELETE CASCADE
);

CREATE TABLE platform_role_permissions (
    role_id BIGINT NOT NULL REFERENCES platform_access_roles(id),
    permission TEXT NOT NULL CHECK (permission ~ '^[a-z][a-z_]*[.][a-z][a-z_]*$'),
    PRIMARY KEY (role_id, permission)
);

CREATE TABLE platform_group_resource_grants (
    group_id BIGINT NOT NULL REFERENCES platform_access_groups(id),
    resource_type TEXT NOT NULL CHECK (resource_type ~ '^[a-z][a-z_]*$'),
    resource_id BIGINT NOT NULL CHECK (resource_id > 0),
    PRIMARY KEY (group_id, resource_type, resource_id)
);

CREATE INDEX platform_group_role_assignments_user_idx
    ON platform_group_role_assignments(user_id, group_id);

-- +goose Down
LOCK TABLE platform_access_groups, platform_access_roles, platform_group_memberships,
    platform_group_role_assignments, platform_role_permissions, platform_group_resource_grants
    IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM platform_access_groups)
       OR EXISTS (SELECT 1 FROM platform_access_roles) THEN
        RAISE EXCEPTION 'rollback refused: configured authorization data must be preserved';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE platform_group_resource_grants, platform_role_permissions,
    platform_group_role_assignments, platform_group_memberships,
    platform_access_roles, platform_access_groups;
