-- +goose Up
CREATE TABLE pms_reservation_cancellations (
 reservation_id bigint PRIMARY KEY,
 property_id bigint NOT NULL,
 room_type_id bigint NOT NULL,
 idempotency_key uuid NOT NULL,
 fingerprint varchar(64) NOT NULL,
 contract_version integer NOT NULL DEFAULT 1 CHECK (contract_version = 1),
 operator_ref varchar(80) NOT NULL CHECK (operator_ref COLLATE "C" ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,79}$'),
 reason_ref varchar(80) NOT NULL CHECK (reason_ref COLLATE "C" ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,79}$'),
 cancelled_at timestamptz NOT NULL,
 UNIQUE (property_id,idempotency_key),
 FOREIGN KEY (reservation_id,property_id,room_type_id) REFERENCES pms_stay_reservations(id,property_id,room_type_id)
);

CREATE VIEW pms_current_stay_reservations AS
 SELECT r.id,r.property_id,r.room_type_id,r.idempotency_key,r.fingerprint,r.guest_ref,r.operator_ref,
 CASE WHEN c.reservation_id IS NULL THEN 'confirmed' ELSE 'cancelled' END AS status,
 r.check_in,r.check_out,r.time_zone,r.currency,r.nightly_amount_minor,r.night_count,
 r.accommodation_base_subtotal_minor,r.pool_revision,r.attestation_ref,r.capacity_at_confirmation,r.confirmed_at
 FROM pms_stay_reservations r LEFT JOIN pms_reservation_cancellations c ON c.reservation_id=r.id;

CREATE VIEW pms_consuming_inventory_commitments AS
 SELECT n.* FROM pms_reservation_inventory_commitments n
 WHERE NOT EXISTS (SELECT 1 FROM pms_reservation_cancellations c WHERE c.reservation_id=n.reservation_id);

-- +goose StatementBegin
CREATE FUNCTION pms_cancellation_guard() RETURNS trigger LANGUAGE plpgsql VOLATILE AS $$
DECLARE r pms_stay_reservations; command text;
BEGIN
 IF current_setting('transaction_isolation') <> 'read committed' THEN
  RAISE EXCEPTION 'PMS cancellation requires READ COMMITTED' USING ERRCODE='0A000';
 END IF;
 PERFORM id FROM pms_properties WHERE id=NEW.property_id FOR SHARE;
 PERFORM id FROM pms_room_types WHERE id=NEW.room_type_id AND property_id=NEW.property_id FOR UPDATE;
 PERFORM id FROM pms_stay_reservations
  WHERE id=NEW.reservation_id AND property_id=NEW.property_id AND room_type_id=NEW.room_type_id FOR UPDATE;
 SELECT * INTO r FROM pms_stay_reservations
  WHERE id=NEW.reservation_id AND property_id=NEW.property_id AND room_type_id=NEW.room_type_id;
 IF NOT FOUND THEN
  RAISE EXCEPTION 'reservation ownership mismatch' USING ERRCODE='23503';
 END IF;
 -- REQ-PMS-003: bind the exact normalized semantic command to the key.
 command := 'pms-cancellation:v1:' || r.property_id || ':' || r.id || ':' ||
  octet_length(NEW.operator_ref) || ':' || NEW.operator_ref || ':' ||
  octet_length(NEW.reason_ref) || ':' || NEW.reason_ref;
 IF NEW.fingerprint IS DISTINCT FROM encode(sha256(convert_to(command,'UTF8')),'hex') THEN
  RAISE EXCEPTION 'cancellation fingerprint mismatch' USING ERRCODE='23514';
 END IF;
 IF (SELECT count(*) FROM pms_reservation_inventory_commitments WHERE reservation_id=r.id) <> r.night_count
 OR EXISTS (SELECT 1 FROM pms_reservation_inventory_commitments WHERE reservation_id=r.id AND (night<r.check_in OR night>=r.check_out)) THEN
  RAISE EXCEPTION 'incomplete or out of range nightly commitments' USING ERRCODE='23514';
 END IF;
 NEW.cancelled_at := pg_catalog.clock_timestamp();
 IF (NEW.cancelled_at AT TIME ZONE r.time_zone)::date >= r.check_in THEN
  RAISE EXCEPTION 'cancellation cutoff reached' USING ERRCODE='P4301';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER pms_cancellation_guard BEFORE INSERT ON pms_reservation_cancellations FOR EACH ROW EXECUTE FUNCTION pms_cancellation_guard();
-- Recheck final ledger integrity and consuming demand even in multi-statement SQL transactions.
CREATE CONSTRAINT TRIGGER pms_cancellation_complete AFTER INSERT ON pms_reservation_cancellations
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION pms_commitment_guard();
CREATE TRIGGER pms_cancellation_immutable BEFORE UPDATE OR DELETE ON pms_reservation_cancellations FOR EACH ROW EXECUTE FUNCTION pms_history_immutable();
CREATE TRIGGER pms_cancellation_no_truncate BEFORE TRUNCATE ON pms_reservation_cancellations FOR EACH STATEMENT EXECUTE FUNCTION pms_history_immutable();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION pms_commitment_guard() RETURNS trigger LANGUAGE plpgsql VOLATILE AS $$
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
  SELECT count(*) n FROM pms_consuming_inventory_commitments
  WHERE property_id=r.property_id AND room_type_id=r.room_type_id AND night>=r.check_in AND night<r.check_out GROUP BY night
 ) counts;
 IF demand>stock THEN RAISE EXCEPTION 'inventory exhausted' USING ERRCODE='P2402'; END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION pms_stock_frozen(p bigint,t bigint) RETURNS boolean LANGUAGE sql VOLATILE AS $$
 SELECT EXISTS(SELECT 1 FROM pms_reservation_pools WHERE property_id=p AND room_type_id=t AND enabled)
 OR EXISTS(SELECT 1 FROM pms_consuming_inventory_commitments c JOIN pms_properties prop ON prop.id=c.property_id
 WHERE c.property_id=p AND c.room_type_id=t AND c.night >= (clock_timestamp() AT TIME ZONE prop.time_zone)::date)
$$;
-- +goose StatementEnd

-- +goose Down
-- Stop admission of concurrent events before checking for irreversible history.
LOCK TABLE pms_properties, pms_room_types, pms_stay_reservations, pms_reservation_cancellations IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pms_reservation_cancellations) THEN
  RAISE EXCEPTION 'rollback refused: cancellation history must be preserved';
 END IF;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION pms_commitment_guard() RETURNS trigger LANGUAGE plpgsql VOLATILE AS $$
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
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION pms_stock_frozen(p bigint,t bigint) RETURNS boolean LANGUAGE sql VOLATILE AS $$
 SELECT EXISTS(SELECT 1 FROM pms_reservation_pools WHERE property_id=p AND room_type_id=t AND enabled)
 OR EXISTS(SELECT 1 FROM pms_reservation_inventory_commitments c JOIN pms_properties prop ON prop.id=c.property_id
 WHERE c.property_id=p AND c.room_type_id=t AND c.night >= (clock_timestamp() AT TIME ZONE prop.time_zone)::date)
$$;
-- +goose StatementEnd

DROP VIEW pms_current_stay_reservations, pms_consuming_inventory_commitments;
DROP TABLE pms_reservation_cancellations;
DROP FUNCTION pms_cancellation_guard();
