# Device Event Ingest

An HTTP service in Go that ingests events from alarm panels and reports each
device's current state. Postgres stores the events and the device state.

## Run

```sh
docker compose up --build
```

The service listens on `localhost:8080`, and the database on `localhost:5432`.
If port 8080 is taken, use another: `APP_PORT=18080 docker compose up --build`.

Configuration, read from environment variables:

| Variable            | Default            | Meaning                                        |
| ------------------- | ------------------ | ---------------------------------------------- |
| `DEVICE_TOKEN`      | `dev-shared-token` | shared token for `X-Device-Token`              |
| `OFFLINE_THRESHOLD` | `5m`               | time without events before a device is offline |
| `DATABASE_URL`      | set in compose     | Postgres connection string                     |
| `PORT`              | `8080`             | port inside the container                      |

## Test

```sh
docker compose up -d --wait db
TEST_DATABASE_URL='postgres://m2m:m2m@localhost:5432/m2m?sslmode=disable' go test -race ./...
```

Tests that use the database create their own device ids and delete their rows
when they finish.

Without `TEST_DATABASE_URL`, the tests that need the database are skipped.
Tests never wait in real time: the server's clock is replaced in tests.

`TestWorkedExample` replays the worked example from the assignment.

CI (`.github/workflows/ci.yml`) runs lint, the tests against Postgres, and
`docker build` on every push to `main` and every pull request.

## API

Every `/devices` request needs the `X-Device-Token` header.

```sh
T='X-Device-Token: dev-shared-token'

# Record an event: 201 when new, 200 with the stored event when it is a duplicate
curl -H "$T" -d '{"event_id":"7c1e9a52-3b0f-4d8e-9a61-2f4b8c0d1e77","type":"sensor_opened","occurred_at":"2026-09-29T14:02:00Z","payload":{"sensor_id":4}}' \
  localhost:8080/devices/panel-0042/events

# Current state
curl -H "$T" localhost:8080/devices/panel-0042

# Devices by status; without status, every device
curl -H "$T" 'localhost:8080/devices?status=offline'

# Event history, oldest first, page by page
curl -H "$T" 'localhost:8080/devices/panel-0042/events?since=2026-09-29T13:00:00Z&limit=2'

# Health and metrics, no token needed
curl localhost:8080/health
curl localhost:8080/metrics
```

Errors always have the body `{"error": true, "message": "..."}`.

**Recording an event** happens in one transaction:

1. Create the device if it is new.
2. Lock the device row (`SELECT ... FOR UPDATE`), so that events for the same
   device are applied one at a time and none is lost.
3. Insert the event. `UNIQUE (device_id, event_id)` with `ON CONFLICT DO NOTHING`
   makes duplicates a no-op.
4. Only if the event is new: apply it to the device state and save the state.

The device state (`last_seen_at`, `arm_state`, `arm_mode`, `open_sensors`) is
kept in the `devices` table, so reading a device is a single row lookup.

**Online or offline** is computed when it is read: a device is offline once more
than `OFFLINE_THRESHOLD` has passed since `last_seen_at`. `GET /devices?status=`
uses the same rule in SQL, with an index on `last_seen_at`.

## Trade-offs

**Every event is written synchronously, in one transaction.** A device gets 201
only after the event is stored and the device state is updated, so duplicates
change nothing and concurrent events are never lost. The cost is write
throughput: every event, heartbeats included, takes a transaction, a row lock and
updates to four indexes.

How I would solve it, in this order:

1. Run several instances of the service and put PgBouncer in front of Postgres.
   The service keeps no state of its own, so this works as is.
2. If Postgres is still the bottleneck, put a queue (such as Kafka, keyed by
   device id so each device's events stay in order) in front of the writes, and
   write events in batches. The service would then answer 202 Accepted instead of
   201, because the event is not stored yet when it answers.

**Every event is kept, heartbeats included.** The history is complete, but it
grows fast: heartbeats are most of the traffic, about 14 million rows a day for
10,000 devices.

How I would solve it:

1. Do not store heartbeats in `events`; they only update `last_seen_at`. This
   removes most of the rows. The history would then show only events that change
   the device state.
2. Split `events` into one partition per day or month, and drop partitions older
   than the retention period. Dropping a partition is instant, unlike deleting
   millions of rows.

## Assumptions

- Events are applied in the order they arrive. The assignment says they arrive in
  the order they occurred.
- `last_seen_at` is the server time when the latest new event arrived, not the
  device's `occurred_at`.
- A device exactly at the threshold (5 minutes) is still online; it is offline
  only after more than 5 minutes.
- `GET /devices` without `status` lists every device. An unknown or empty `status`
  returns 400.
- `occurred_at` must be ISO 8601 with a time zone. Offsets such as `+03:00` are
  accepted and returned in UTC.
- `/health` and `/metrics` need no token, so health checks and Prometheus can use
  them. In production they would be reachable only from the internal network.

## Extensions

**History API**: `GET /devices/{id}/events?since=&limit=`. I chose it because it
builds on the core data and needs a real pagination design.

- Events are ordered by `occurred_at`, then by `seq`, the order they were stored.
- `next` holds a cursor: the `(occurred_at, seq)` of the last event on the page,
  base64-encoded. The next page starts right after it. Two events with the same
  `occurred_at` are never skipped or repeated, because `seq` is unique.
- The cursor reads from an index on `(device_id, occurred_at, seq)`, so every page
  is equally fast. `OFFSET` would get slower with every page.
- `since` is inclusive. `limit` defaults to 50, maximum 500.
- `since` and `cursor` cannot be used together.
- In a URL, `+` in `since` must be written as `%2B`.

**Observe it**: I chose it because a service that devices depend on must show when
it is unhealthy.

- `GET /health`: 200 when the database answers, 503 otherwise.
- `GET /metrics`, in Prometheus format:
  - `events_ingested_total`: new events recorded
  - `events_duplicate_total`: duplicates received
  - `http_request_duration_seconds`: latency, by route and status code

## Where I used AI

I used Claude Code throughout: to plan the steps, write code and tests, and
explain things I had not used before (GitHub Actions, Prometheus). I made
the design decisions, reviewed each change, and asked for changes where I
disagreed.

## At 10x scale

- Run several instances of the service behind a load balancer. They keep no
  state of their own, so this works as is.
- Use a connection pooler (PgBouncer) and tune the pool size.
- Partition `events` by time, and move or delete old events.
- Paginate `GET /devices` the same way as the history.
- If the database becomes the bottleneck, put a queue (such as Kafka) in front of
  writes and apply events in batches.
