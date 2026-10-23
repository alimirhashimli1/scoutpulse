package server

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RateLimit guards an endpoint with a per-client token bucket.
//
// This lives here rather than at the gateway because Caddy's rate-limit module
// is a third-party plugin absent from the official image, and a control that
// only exists when someone remembers to build a custom gateway is not a
// control. In the service it is unconditional and unit-testable.
//
// The target is credential stuffing against POST /auth/login, which is
// otherwise unbounded: an attacker gets unlimited guesses, and every guess
// costs a bcrypt hash of server CPU whether or not the account exists.
type RateLimit struct {
	// Rate is the sustained requests per second allowed per client.
	Rate float64
	// Burst is how many requests may arrive at once before throttling.
	Burst int

	mu      sync.Mutex
	buckets map[string]*bucket
	// lastSwept bounds the cost of reclaiming idle buckets.
	lastSwept time.Time
}

type bucket struct {
	tokens   float64
	lastSeen time.Time
}

// NewRateLimit builds a limiter allowing rate requests per second per client,
// with the given burst.
func NewRateLimit(rate float64, burst int) *RateLimit {
	return &RateLimit{
		Rate:      rate,
		Burst:     burst,
		buckets:   make(map[string]*bucket),
		lastSwept: time.Now(),
	}
}

// idleBucketTTL is how long a client's bucket is retained after its last
// request. Buckets are dropped once full, so this only bounds memory for
// clients that stop mid-throttle.
const idleBucketTTL = 10 * time.Minute

// allow reports whether the client may proceed, and how long to wait if not.
func (rl *RateLimit) allow(key string) (bool, time.Duration) {
	now := time.Now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(rl.Burst)}
		rl.buckets[key] = b
	}

	// Refill for the elapsed time, capped at the burst size.
	b.tokens += now.Sub(b.lastSeen).Seconds() * rl.Rate
	if b.lastSeen.IsZero() {
		b.tokens = float64(rl.Burst)
	}
	if b.tokens > float64(rl.Burst) {
		b.tokens = float64(rl.Burst)
	}
	b.lastSeen = now

	rl.sweepLocked(now)

	if b.tokens < 1 {
		// Time until one whole token is available again.
		return false, time.Duration((1-b.tokens)/rl.Rate*float64(time.Second)) + time.Second
	}
	b.tokens--
	return true, 0
}

// sweepLocked drops buckets nobody has touched recently. Called with the lock
// held, and at most once a minute so a burst of distinct clients does not turn
// every request into a full map scan.
func (rl *RateLimit) sweepLocked(now time.Time) {
	if now.Sub(rl.lastSwept) < time.Minute {
		return
	}
	rl.lastSwept = now
	for k, b := range rl.buckets {
		if now.Sub(b.lastSeen) > idleBucketTTL {
			delete(rl.buckets, k)
		}
	}
}

// Middleware returns the limiter as request middleware, keyed by client IP.
func (rl *RateLimit) Middleware(next http.Handler) http.Handler {
	return rl.MiddlewareBy(ClientIP, next)
}

// MiddlewareBy is Middleware with the bucket chosen by key -- an account id,
// say, for a limit that should follow a signed-in user across addresses.
func (rl *RateLimit) MiddlewareBy(key func(*http.Request) string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, retryAfter := rl.allow(key(r))
		if !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":      "too many requests, slow down",
				"code":       "rate_limited",
				"request_id": RequestIDFrom(r.Context()),
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Limit wraps h with a limiter of the given rate and burst.
func Limit(rate float64, burst int, h http.Handler) http.Handler {
	return NewRateLimit(rate, burst).Middleware(h)
}

// Blocked reports whether key has exhausted its bucket, without spending a
// token. Paired with Record, it builds a limit that counts only some requests
// -- failed sign-ins, say -- while still refusing every request once the
// budget is gone.
func (rl *RateLimit) Blocked(key string) (bool, time.Duration) {
	now := time.Now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok {
		return false, 0
	}
	tokens := b.tokens + now.Sub(b.lastSeen).Seconds()*rl.Rate
	if tokens >= 1 {
		return false, 0
	}
	return true, time.Duration((1-tokens)/rl.Rate*float64(time.Second)) + time.Second
}

// Record spends one token for key, whether or not one is available.
func (rl *RateLimit) Record(key string) {
	_, _ = rl.allow(key)
}

// ClientIPSource names where the caller's address is read from. It is set
// once at startup from CLIENT_IP_HEADER and is deliberately not guessed per
// request: a forwarded-for header is only evidence of anything when the
// process knows which proxy wrote it, and a header any caller can send is a
// rate-limit key any caller can change.
//
//   - "" (the default): the TCP peer address. Correct for a process nothing
//     sits in front of, and safe everywhere -- at worst, everything behind one
//     proxy shares a bucket.
//   - "X-Real-IP": the value set by a proxy that overwrites it. Railway's edge
//     does this, so it is the default when RAILWAY_PROJECT_ID is present.
//   - "X-Forwarded-For": the first entry. Only for a proxy that discards the
//     client's own copy, such as Caddy without trusted_proxies -- the compose
//     gateway.
var clientIPSource = clientIPSourceFromEnv()

func clientIPSourceFromEnv() string {
	if v, ok := os.LookupEnv("CLIENT_IP_HEADER"); ok {
		return http.CanonicalHeaderKey(strings.TrimSpace(v))
	}
	if os.Getenv("RAILWAY_PROJECT_ID") != "" {
		return "X-Real-Ip"
	}
	return ""
}

// ClientIP identifies the caller for rate-limiting purposes, from the source
// configured in clientIPSource. A configured header that is missing falls back
// to the peer address rather than to some other header, so a request that
// skips the proxy cannot pick a key of its own.
func ClientIP(r *http.Request) string {
	switch clientIPSource {
	case "X-Forwarded-For":
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first, _, _ := strings.Cut(xff, ",")
			if ip := strings.TrimSpace(first); ip != "" {
				return ip
			}
		}
	case "":
	default:
		if ip := strings.TrimSpace(r.Header.Get(clientIPSource)); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
