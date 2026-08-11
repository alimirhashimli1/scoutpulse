// Package observability supplies the metrics and tracing every service
// exposes.
//
// It lives outside package server so that a service can opt in to the
// dependency weight, and so that the middleware chain stays composable: server
// defines the Middleware shape, this package supplies implementations.
package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/scoutpulse/libs/platform/server"
)

// MetricsPath is where every service exposes its Prometheus metrics.
const MetricsPath = "/metrics"

var (
	requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests, by service, method, route and status.",
		},
		// Labelled by route pattern rather than by raw path: labelling with
		// "/api/v1/players/{id}" keeps cardinality bounded, where the actual
		// path would create one time series per player.
		[]string{"service", "method", "route", "status"},
	)

	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "http_request_duration_seconds",
			Help: "HTTP request latency, by service, method and route.",
			// Buckets span a fast database read to a slow outlier.
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"service", "method", "route"},
	)

	requestsInFlight = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "HTTP requests currently being served.",
		},
		[]string{"service"},
	)

	eventsPublished = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "events_published_total",
			Help: "Domain events published, by subject and outcome.",
		},
		[]string{"service", "subject", "outcome"},
	)
)

func init() {
	prometheus.MustRegister(requestsTotal, requestDuration, requestsInFlight, eventsPublished)
}

// RecordEventPublished counts an event publish attempt. A dropped event is not
// an error the caller sees, so this counter is the signal that the bus is
// failing.
func RecordEventPublished(service, subject string, err error) {
	outcome := "ok"
	if err != nil {
		outcome = "error"
	}
	eventsPublished.WithLabelValues(service, subject, outcome).Inc()
}

// statusRecorder captures the status code for metrics.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Metrics returns middleware recording request counts and latency.
func Metrics(service string) server.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestsInFlight.WithLabelValues(service).Inc()
			defer requestsInFlight.WithLabelValues(service).Dec()

			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}

			next.ServeHTTP(rec, r)

			if rec.status == 0 {
				rec.status = http.StatusOK
			}

			route := routeOf(r)
			requestsTotal.WithLabelValues(service, r.Method, route, strconv.Itoa(rec.status)).Inc()
			requestDuration.WithLabelValues(service, r.Method, route).Observe(time.Since(start).Seconds())
		})
	}
}

// routeOf returns the matched route pattern, which is bounded in cardinality,
// falling back to a placeholder when no route matched.
func routeOf(r *http.Request) string {
	if pattern := r.Pattern; pattern != "" {
		return pattern
	}
	return "unmatched"
}

// Handler serves the Prometheus scrape endpoint.
func Handler() http.Handler { return promhttp.Handler() }
