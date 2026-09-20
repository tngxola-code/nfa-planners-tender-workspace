-- audit_log records who did what, to what, when.
-- actor_sub is NOT NULL: a row with no actor is not an audit row, and the
-- schema refuses to store one.
CREATE TABLE IF NOT EXISTS audit_log (
	id             BIGSERIAL   PRIMARY KEY,
	at             TIMESTAMPTZ NOT NULL,
	actor_sub      TEXT        NOT NULL,
	actor_username TEXT        NOT NULL DEFAULT '',
	actor_client   TEXT        NOT NULL DEFAULT '',
	action         TEXT        NOT NULL,
	target_type    TEXT        NOT NULL DEFAULT '',
	target_id      TEXT        NOT NULL DEFAULT '',
	metadata       JSONB
);

CREATE INDEX IF NOT EXISTS audit_log_at_idx     ON audit_log (at DESC);
CREATE INDEX IF NOT EXISTS audit_log_actor_idx  ON audit_log (actor_sub, at DESC);
CREATE INDEX IF NOT EXISTS audit_log_target_idx ON audit_log (target_type, target_id, at DESC);
