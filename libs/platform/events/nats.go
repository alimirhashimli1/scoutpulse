package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/scoutpulse/libs/platform/observability"
	"github.com/scoutpulse/libs/platform/server"
)

// URLEnvVar configures the NATS server. When unset, services fall back to a
// no-op publisher so the stack runs without a broker.
const URLEnvVar = "NATS_URL"

// NATSPublisher publishes envelopes to a NATS subject.
type NATSPublisher struct {
	conn   *nats.Conn
	logger *slog.Logger
}

// Connect dials NATS. Callers should prefer PublisherFromEnv.
func Connect(url string, logger *slog.Logger) (*NATSPublisher, error) {
	if logger == nil {
		logger = slog.Default()
	}

	conn, err := nats.Connect(url,
		nats.Name("scoutpulse"),
		nats.MaxReconnects(-1), // reconnect indefinitely
		nats.ReconnectWait(2*time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			logger.Warn("nats disconnected", "error", err)
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			logger.Info("nats reconnected", "url", c.ConnectedUrl())
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("connecting to nats at %s: %w", url, err)
	}

	logger.Info("connected to nats", "url", conn.ConnectedUrl())
	return &NATSPublisher{conn: conn, logger: logger}, nil
}

// PublisherFromEnv returns a NATS publisher when NATS_URL is set, and a no-op
// publisher otherwise.
//
// The event bus is deliberately optional: a service must still start and serve
// requests when the broker is absent, because events are a secondary effect of
// a write, not part of it.
func PublisherFromEnv(logger *slog.Logger) (Publisher, error) {
	if logger == nil {
		logger = slog.Default()
	}

	url := os.Getenv(URLEnvVar)
	if url == "" {
		logger.Info("no " + URLEnvVar + " set, events are disabled")
		return NopPublisher{}, nil
	}

	publisher, err := Connect(url, logger)
	if err != nil {
		// Degrade rather than refuse to start. An unreachable broker must not
		// take down the read paths, which do not publish anything at all --
		// events are a secondary effect of a write, and that principle has to
		// hold at startup too, not only once the process is running.
		//
		// This is the one case the no-op fallback previously missed: it
		// covered an unset URL, which was never the risk, but not a broker
		// that is configured and down, which is.
		logger.Error("could not reach the event bus, continuing with events disabled",
			"url", url, "error", err)
		return NopPublisher{}, nil
	}
	return publisher, nil
}

func (p *NATSPublisher) Publish(ctx context.Context, subject string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling %s payload: %w", subject, err)
	}

	envelope := Envelope{
		ID:         uuid.NewString(),
		Subject:    subject,
		OccurredAt: time.Now().UTC(),
		RequestID:  server.RequestIDFrom(ctx),
		Payload:    body,
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshalling %s envelope: %w", subject, err)
	}

	if err := p.conn.Publish(subject, data); err != nil {
		return fmt.Errorf("publishing %s: %w", subject, err)
	}
	return nil
}

func (p *NATSPublisher) Close() error {
	// Drain flushes buffered messages before closing, so events published
	// during shutdown are not lost.
	return p.conn.Drain()
}

// Subscribe registers h for subject within a queue group.
func (p *NATSPublisher) Subscribe(ctx context.Context, subject, queueGroup string, h Handler) error {
	_, err := p.conn.QueueSubscribe(subject, queueGroup, func(msg *nats.Msg) {
		var envelope Envelope
		if err := json.Unmarshal(msg.Data, &envelope); err != nil {
			p.logger.Error("dropping malformed event", "subject", msg.Subject, "error", err)
			return
		}

		if err := h(ctx, envelope); err != nil {
			// A failed handler must not take the subscription down; the
			// next event still needs to be delivered.
			p.logger.Error("event handler failed",
				"subject", envelope.Subject,
				"event_id", envelope.ID,
				"request_id", envelope.RequestID,
				"error", err)
		}
	})
	if err != nil {
		return fmt.Errorf("subscribing to %s: %w", subject, err)
	}
	return nil
}

// NopPublisher discards every event. It is what services use when no broker is
// configured, and what tests use when they do not assert on events.
type NopPublisher struct{}

func (NopPublisher) Publish(context.Context, string, any) error { return nil }
func (NopPublisher) Close() error                               { return nil }

// SafePublisher wraps a Publisher so a transport failure is logged instead of
// returned.
//
// The write it describes has already been committed, so failing the caller
// would report an error for something that did succeed. A dropped event is a
// delivery problem to be surfaced in logs and metrics, not a reason to tell the
// client their transfer did not happen.
type SafePublisher struct {
	Inner  Publisher
	Logger *slog.Logger
	// Service labels the metrics this publisher records.
	Service string
}

func NewSafePublisher(inner Publisher, logger *slog.Logger) SafePublisher {
	return NewSafePublisherFor("", inner, logger)
}

// NewSafePublisherFor is NewSafePublisher with the service name used to label
// the events_published_total counter.
func NewSafePublisherFor(service string, inner Publisher, logger *slog.Logger) SafePublisher {
	if logger == nil {
		logger = slog.Default()
	}
	return SafePublisher{Inner: inner, Logger: logger, Service: service}
}

func (s SafePublisher) Publish(ctx context.Context, subject string, payload any) error {
	err := s.Inner.Publish(ctx, subject, payload)

	// Counted as well as logged. A dropped event is invisible to the caller by
	// design, so this counter is the only signal that the bus is failing --
	// alert on events_published_total{outcome="error"}.
	observability.RecordEventPublished(s.Service, subject, err)

	if err != nil {
		s.Logger.ErrorContext(ctx, "dropping event, publish failed",
			"subject", subject,
			"request_id", server.RequestIDFrom(ctx),
			"error", err)
	}
	return nil
}

func (s SafePublisher) Close() error { return s.Inner.Close() }
