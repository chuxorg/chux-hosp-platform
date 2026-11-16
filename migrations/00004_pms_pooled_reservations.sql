-- +goose Up
CREATE TABLE pms_reservation_pools (
 room_type_id bigint PRIMARY KEY,
 property_id bigint NOT NULL,
 scope text NOT NULL CHECK (scope = 'homogeneous_segment_neutral'),
 enabled boolean NOT NULL DEFAULT false,
 attestation_ref varchar(80) NOT NULL CHECK (attestation_ref ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,79}$'),
 operator_ref varchar(80) NOT NULL CHECK (operator_ref ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,79}$'),
 revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY (room_type_id,property_id) REFERENCES pms_room_types(id,property_id)
);
CREATE TABLE pms_stay_reservations (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 property_id bigint NOT NULL,
 room_type_id bigint NOT NULL,
 idempotency_key uuid NOT NULL,
 fingerprint varchar(64) NOT NULL CHECK (fingerprint ~ '^[0-9a-f]{64}$'),
 fingerprint_version integer NOT NULL DEFAULT 1 CHECK (fingerprint_version = 1),
 guest_ref varchar(80) NOT NULL CHECK (guest_ref ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,79}$'),
 operator_ref varchar(80) NOT NULL CHECK (operator_ref ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,79}$'),
 check_in date NOT NULL, check_out date NOT NULL,
 time_zone varchar(64) NOT NULL,
 status text NOT NULL DEFAULT 'confirmed' CHECK (status = 'confirmed'),
 currency text NOT NULL CHECK (currency = 'THB'),
 nightly_amount_minor bigint NOT NULL CHECK (nightly_amount_minor > 0),
 night_count integer NOT NULL CHECK (night_count BETWEEN 1 AND 366 AND night_count = check_out-check_in),
 accommodation_base_subtotal_minor bigint NOT NULL CHECK (accommodation_base_subtotal_minor > 0 AND accommodation_base_subtotal_minor::numeric = nightly_amount_minor::numeric * night_count),
 pool_revision bigint NOT NULL CHECK (pool_revision > 0),
 attestation_ref varchar(80) NOT NULL,
 capacity_at_confirmation bigint NOT NULL CHECK (capacity_at_confirmation > 0),
 confirmed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE (property_id,idempotency_key), UNIQUE(id,property_id,room_type_id),
 FOREIGN KEY (room_type_id,property_id) REFERENCES pms_room_types(id,property_id)
);
CREATE TABLE pms_reservation_inventory_commitments (
 reservation_id bigint NOT NULL, property_id bigint NOT NULL, room_type_id bigint NOT NULL, night date NOT NULL,
 PRIMARY KEY (reservation_id,night),
 FOREIGN KEY (reservation_id,property_id,room_type_id) REFERENCES pms_stay_reservations(id,property_id,room_type_id)
);
CREATE INDEX pms_commitments_type_night ON pms_reservation_inventory_commitments(property_id,room_type_id,night);

-- Each query in these VOLATILE functions obtains a fresh READ COMMITTED snapshot
-- AFTER acquiring the shared property / exclusive RoomType mutex. No cross-row CHECK.
-- +goose StatementBegin
CREATE FUNCTION pms_booking_guard() RETURNS trigger LANGUAGE plpgsql VOLATILE AS $$
DECLARE p pms_properties; t pms_room_types; pool pms_reservation_pools; rate pms_room_type_rates; stock bigint;
BEGIN
 IF current_setting('transaction_isolation') <> 'read committed' THEN
  RAISE EXCEPTION 'PMS reservation/catalog writes require READ COMMITTED' USING ERRCODE='0A000';
 END IF;
 PERFORM id FROM pms_properties WHERE id=NEW.property_id FOR SHARE;
 PERFORM id FROM pms_room_types WHERE id=NEW.room_type_id AND property_id=NEW.property_id FOR UPDATE;
 SELECT * INTO p FROM pms_properties WHERE id=NEW.property_id;
 SELECT * INTO t FROM pms_room_types WHERE id=NEW.room_type_id AND property_id=NEW.property_id;
 SELECT * INTO pool FROM pms_reservation_pools WHERE room_type_id=NEW.room_type_id AND property_id=NEW.property_id;
 SELECT * INTO rate FROM pms_room_type_rates WHERE room_type_id=NEW.room_type_id AND property_id=NEW.property_id;
 SELECT count(*) INTO stock FROM pms_rooms WHERE property_id=NEW.property_id AND room_type_id=NEW.room_type_id AND is_active;
 IF NOT coalesce(p.is_active AND t.is_active AND rate.is_active AND pool.enabled,false)
 OR NEW.time_zone IS DISTINCT FROM p.time_zone OR NEW.currency IS DISTINCT FROM rate.currency
 OR NEW.nightly_amount_minor IS DISTINCT FROM rate.amount_minor OR NEW.pool_revision IS DISTINCT FROM pool.revision
 OR NEW.attestation_ref IS DISTINCT FROM pool.attestation_ref OR NEW.capacity_at_confirmation <> stock
 OR NEW.check_in < (clock_timestamp() AT TIME ZONE p.time_zone)::date THEN
  RAISE EXCEPTION 'reservation configuration changed or ineligible' USING ERRCODE='P2401';
 END IF;
 NEW.confirmed_at := clock_timestamp();
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER pms_booking_guard BEFORE INSERT ON pms_stay_reservations FOR EACH ROW EXECUTE FUNCTION pms_booking_guard();

-- +goose StatementBegin
CREATE FUNCTION pms_commitment_guard() RETURNS trigger LANGUAGE plpgsql VOLATILE AS $$
DECLARE r pms_stay_reservations; stock bigint; demand bigint;
BEGIN
 IF TG_TABLE_NAME='pms_stay_reservations' THEN
  SELECT * INTO r FROM pms_stay_reservations WHERE id=NEW.id;
 ELSE
  SELECT * INTO r FROM pms_stay_reservations WHERE id=NEW.reservation_id;
 END IF;
 PERFORM id FROM pms_properties WHERE id=r.property_id FOR SHARE;
 PERFORM id FROM pms_room_types WHERE id=r.room_type_id AND property_id=r.property_id FOR UPDATE;
 IF (SELECT count(*) FROM pms_reservation_inventory_commitments WHERE reservation_id=r.id) <> r.night_count
 OR EXISTS (SELECT 1 FROM pms_reservation_inventory_commitments WHERE reservation_id=r.id AND (night<r.check_in OR night>=r.check_out)) THEN
  RAISE EXCEPTION 'incomplete or out of range nightly commitments' USING ERRCODE='23514';
 END IF;
 SELECT count(*) INTO stock FROM pms_rooms WHERE property_id=r.property_id AND room_type_id=r.room_type_id AND is_active;
 SELECT coalesce(max(n),0) INTO demand FROM (
  SELECT count(*) n FROM pms_reservation_inventory_commitments
  WHERE property_id=r.property_id AND room_type_id=r.room_type_id AND night>=r.check_in AND night<r.check_out GROUP BY night
 ) counts;
 IF demand>stock THEN RAISE EXCEPTION 'inventory exhausted' USING ERRCODE='P2402'; END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER pms_reservation_complete AFTER INSERT ON pms_stay_reservations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION pms_commitment_guard();
CREATE CONSTRAINT TRIGGER pms_commitments_complete AFTER INSERT ON pms_reservation_inventory_commitments DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION pms_commitment_guard();

-- +goose StatementBegin
CREATE FUNCTION pms_history_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'reservation history is immutable' USING ERRCODE='P2403'; END $$;
-- +goose StatementEnd
CREATE TRIGGER pms_reservation_immutable BEFORE UPDATE OR DELETE ON pms_stay_reservations FOR EACH ROW EXECUTE FUNCTION pms_history_immutable();
CREATE TRIGGER pms_commitments_immutable BEFORE UPDATE OR DELETE ON pms_reservation_inventory_commitments FOR EACH ROW EXECUTE FUNCTION pms_history_immutable();
CREATE TRIGGER pms_reservation_no_truncate BEFORE TRUNCATE ON pms_stay_reservations FOR EACH STATEMENT EXECUTE FUNCTION pms_history_immutable();
CREATE TRIGGER pms_commitments_no_truncate BEFORE TRUNCATE ON pms_reservation_inventory_commitments FOR EACH STATEMENT EXECUTE FUNCTION pms_history_immutable();

-- Conservative temporary freeze: even adding sellable stock needs closed,
-- unencumbered inventory and a new attestation. Names/display numbers remain editable.
-- +goose StatementBegin
CREATE FUNCTION pms_stock_frozen(p bigint,t bigint) RETURNS boolean LANGUAGE sql VOLATILE AS $$
 SELECT EXISTS(SELECT 1 FROM pms_reservation_pools WHERE property_id=p AND room_type_id=t AND enabled)
 OR EXISTS(SELECT 1 FROM pms_reservation_inventory_commitments c JOIN pms_properties prop ON prop.id=c.property_id
 WHERE c.property_id=p AND c.room_type_id=t AND c.night >= (clock_timestamp() AT TIME ZONE prop.time_zone)::date)
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION pms_catalog_guard() RETURNS trigger LANGUAGE plpgsql VOLATILE AS $$
DECLARE p bigint; t bigint; prior bigint; protected boolean := false;
BEGIN
 IF current_setting('transaction_isolation') <> 'read committed' THEN
  RAISE EXCEPTION 'PMS reservation/catalog writes require READ COMMITTED' USING ERRCODE='0A000';
 END IF;
 IF TG_OP='DELETE' THEN p:=OLD.property_id; t:=OLD.room_type_id;
 ELSE p:=NEW.property_id; t:=NEW.room_type_id; END IF;
 IF TG_OP='UPDATE' AND TG_TABLE_NAME='pms_room_type_rates' THEN
  PERFORM id FROM pms_properties WHERE id IN(p,OLD.property_id) ORDER BY id FOR UPDATE;
 ELSE
  PERFORM id FROM pms_properties WHERE id=p FOR UPDATE;
 END IF;
 IF TG_OP='UPDATE' AND TG_TABLE_NAME<>'pms_room_type_rates' AND (OLD.property_id<>NEW.property_id OR (TG_TABLE_NAME='pms_reservation_pools' AND OLD.room_type_id<>NEW.room_type_id)) THEN
  RAISE EXCEPTION 'catalog ownership is immutable' USING ERRCODE='P2403';
 END IF;
 IF TG_TABLE_NAME='pms_rooms' AND TG_OP='UPDATE' THEN prior:=OLD.room_type_id; ELSE prior:=t; END IF;
 PERFORM id FROM pms_room_types WHERE property_id=p AND id IN(t,prior) ORDER BY id FOR UPDATE;
 IF TG_TABLE_NAME='pms_rooms' THEN
  IF TG_OP='INSERT' THEN protected:=NEW.is_active AND pms_stock_frozen(p,t);
  ELSIF TG_OP='DELETE' THEN protected:=OLD.is_active AND pms_stock_frozen(p,t);
  ELSE protected:=(OLD.is_active IS DISTINCT FROM NEW.is_active OR OLD.room_type_id<>NEW.room_type_id)
    AND (pms_stock_frozen(p,t) OR pms_stock_frozen(p,prior)); END IF;
 ELSIF TG_TABLE_NAME='pms_reservation_pools' THEN
  IF TG_OP='DELETE' THEN protected:=true;
  ELSE
   IF TG_OP='UPDATE' THEN NEW.revision:=OLD.revision+1; ELSE NEW.revision:=1; END IF;
   NEW.recorded_at:=clock_timestamp();
   IF NEW.enabled AND NOT EXISTS(SELECT 1 FROM pms_rooms WHERE property_id=p AND room_type_id=t AND is_active) THEN
    RAISE EXCEPTION 'pool has no active backing stock' USING ERRCODE='23514';
   END IF;
  END IF;
 END IF;
 IF protected THEN RAISE EXCEPTION 'protected reservation inventory' USING ERRCODE='P2403'; END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; ELSE RETURN NEW; END IF;
END $$;
-- +goose StatementEnd
-- RoomType rows have id instead of room_type_id, so expose a separate narrow guard.
-- +goose StatementBegin
CREATE FUNCTION pms_type_guard() RETURNS trigger LANGUAGE plpgsql VOLATILE AS $$
BEGIN
 IF current_setting('transaction_isolation') <> 'read committed' THEN
  RAISE EXCEPTION 'PMS reservation/catalog writes require READ COMMITTED' USING ERRCODE='0A000';
 END IF;
 PERFORM id FROM pms_properties WHERE id=OLD.property_id FOR UPDATE;
 IF TG_OP='DELETE' THEN
  IF pms_stock_frozen(OLD.property_id,OLD.id) THEN RAISE EXCEPTION 'protected type' USING ERRCODE='P2403'; END IF;
  RETURN OLD;
 END IF;
 IF NEW.property_id<>OLD.property_id OR (NEW.max_occupancy<>OLD.max_occupancy AND pms_stock_frozen(OLD.property_id,OLD.id)) THEN
  RAISE EXCEPTION 'protected type' USING ERRCODE='P2403';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER pms_rooms_guard BEFORE INSERT OR UPDATE OR DELETE ON pms_rooms FOR EACH ROW EXECUTE FUNCTION pms_catalog_guard();
CREATE TRIGGER pms_rates_guard BEFORE INSERT OR UPDATE OR DELETE ON pms_room_type_rates FOR EACH ROW EXECUTE FUNCTION pms_catalog_guard();
CREATE TRIGGER pms_pools_guard BEFORE INSERT OR UPDATE OR DELETE ON pms_reservation_pools FOR EACH ROW EXECUTE FUNCTION pms_catalog_guard();
CREATE TRIGGER pms_types_guard BEFORE UPDATE OR DELETE ON pms_room_types FOR EACH ROW EXECUTE FUNCTION pms_type_guard();
-- +goose StatementBegin
CREATE FUNCTION pms_property_guard() RETURNS trigger LANGUAGE plpgsql VOLATILE AS $$
BEGIN
 IF current_setting('transaction_isolation') <> 'read committed' THEN
  RAISE EXCEPTION 'PMS reservation/catalog writes require READ COMMITTED' USING ERRCODE='0A000';
 END IF;
 IF NEW.time_zone IS DISTINCT FROM OLD.time_zone AND EXISTS(SELECT 1 FROM pms_stay_reservations WHERE property_id=OLD.id) THEN
  RAISE EXCEPTION 'booking time zone is immutable' USING ERRCODE='P2403';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER pms_property_guard BEFORE UPDATE ON pms_properties FOR EACH ROW EXECUTE FUNCTION pms_property_guard();
-- TRUNCATE bypasses row triggers. Never allow it on backing catalogs while these
-- guards are installed (ordinary DELETE remains available when safe).
CREATE TRIGGER pms_rooms_no_truncate BEFORE TRUNCATE ON pms_rooms FOR EACH STATEMENT EXECUTE FUNCTION pms_history_immutable();
CREATE TRIGGER pms_types_no_truncate BEFORE TRUNCATE ON pms_room_types FOR EACH STATEMENT EXECUTE FUNCTION pms_history_immutable();
CREATE TRIGGER pms_pools_no_truncate BEFORE TRUNCATE ON pms_reservation_pools FOR EACH STATEMENT EXECUTE FUNCTION pms_history_immutable();

-- +goose Down
-- Block confirmations before inspecting history; no check/drop race.
LOCK TABLE pms_properties, pms_room_types, pms_stay_reservations IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pms_stay_reservations) THEN
  RAISE EXCEPTION 'rollback refused: reservation history must be preserved';
 END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER pms_property_guard ON pms_properties;
DROP TRIGGER pms_types_guard ON pms_room_types;
DROP TRIGGER pms_rooms_guard ON pms_rooms;
DROP TRIGGER pms_rates_guard ON pms_room_type_rates;
DROP TRIGGER pms_rooms_no_truncate ON pms_rooms;
DROP TRIGGER pms_types_no_truncate ON pms_room_types;
DROP TABLE pms_reservation_inventory_commitments;
DROP TABLE pms_stay_reservations;
DROP TABLE pms_reservation_pools;
DROP FUNCTION pms_booking_guard(), pms_commitment_guard(), pms_catalog_guard(), pms_property_guard(), pms_type_guard(), pms_stock_frozen(bigint,bigint), pms_history_immutable();
