CREATE TABLE devices (
    device_id    text PRIMARY KEY,
    last_seen_at timestamptz NOT NULL,
    arm_state    text NOT NULL DEFAULT 'disarmed',
    arm_mode     text,
    open_sensors smallint[] NOT NULL DEFAULT '{}'
);

CREATE INDEX devices_last_seen_at_idx ON devices (last_seen_at);

CREATE TABLE events (
    seq         bigserial PRIMARY KEY,
    device_id   text NOT NULL REFERENCES devices (device_id),
    event_id    uuid NOT NULL,
    type        text NOT NULL,
    occurred_at timestamptz NOT NULL,
    payload     jsonb NOT NULL,
    received_at timestamptz NOT NULL,
    UNIQUE (device_id, event_id)
);

CREATE INDEX events_history_idx ON events (device_id, occurred_at, seq);
