# Service template

The skeleton every ScoutPulse service starts from. Copy it rather than copying
an existing service: this carries the shared platform wiring and none of the
football or identity domain.

## Create a service

```bash
make new-service NAME=transfer-feed PORT=8082
```

That copies this directory to `apps/<name>-svc`, substitutes the name and port,
and adds the module to `go.work` and the Makefile. It leaves you with a service
that builds, serves `/health` and `/metrics`, and does nothing else.

## What you get

Every service starts with the same wiring, so none of it is re-invented per app:

| Concern | Where it comes from |
|---|---|
| Server timeouts, graceful shutdown | `libs/platform/server` |
| Request IDs, structured logs, CORS, panic recovery | `libs/platform/server` |
| Prometheus `/metrics`, OTel tracing | `libs/platform/observability` |
| Error taxonomy and HTTP status mapping | `libs/platform/apperr` + `httpx` |
| JSON decode with size limits and unknown-field rejection | `libs/platform/httpx` |
| Token verification | `libs/auth` |
| Postgres connection | `libs/db` |
| Event publish/subscribe | `libs/platform/events` |

## Then

1. Add a `handle_path /api/<name>/*` block to `deploy/gateway/Caddyfile`.
2. Add a scrape target to `deploy/prometheus/prometheus.yml`.
3. Add the service to `docker-compose.yml`.
4. If it needs a database, add it to `deploy/postgres/init-databases.sh` and
   write migrations under `migrations/`.

## Layers

`route → handler → service → repository`. Handlers decode, delegate, and
encode; they hold no authorization logic. Services own the rules. Repositories
own SQL and translate database errors into `apperr`.

## Consuming events instead of being called

A service that reacts to football activity subscribes rather than being called,
so the football service never needs to know it exists:

```go
sub, err := events.Connect(os.Getenv(events.URLEnvVar), logger)
if err != nil {
    log.Fatalf("connecting to the event bus: %v", err)
}
err = sub.Subscribe(ctx, events.SubjectPlayerTransferred, "transfer-feed", func(ctx context.Context, e events.Envelope) error {
    var payload events.PlayerTransferred
    if err := e.Decode(&payload); err != nil {
        return err
    }
    return store.Append(ctx, payload)
})
```

The queue group name makes delivery competing rather than broadcast: every
replica of this app shares one group, so exactly one handles each event.
Delivery is at-least-once, so handlers must be idempotent — use `e.ID` to
recognise a repeat.
