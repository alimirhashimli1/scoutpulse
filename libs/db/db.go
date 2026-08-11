// Package db opens the Postgres connections every ScoutPulse service uses.
//
// It is the database half of the shared bootstrap: as with the HTTP timeouts
// in libs/platform/server, the limits here are applied centrally so that no
// service can forget them. An unpooled *sql.DB defaults to unlimited open
// connections, which exhausts a shared Postgres long before it protects it.
package db

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// Connection pool defaults.
//
// MaxOpenConns is the important one. Postgres defaults to max_connections=100
// shared by every client, so each service must cap itself well below that or a
// load spike turns into "sorry, too many clients already" for everyone.
// MaxIdleConns matching MaxOpenConns avoids reopening a connection on every
// request once the pool has warmed; the database driver's own default of 2
// means most requests pay a fresh handshake.
const (
	DefaultMaxOpenConns    = 25
	DefaultMaxIdleConns    = 25
	DefaultConnMaxLifetime = 30 * time.Minute
	DefaultConnMaxIdleTime = 5 * time.Minute
	// ConnectTimeout bounds the initial reachability check, so a service
	// fails fast against an unresponsive database instead of hanging at
	// startup with no diagnostic.
	ConnectTimeout = 10 * time.Second
)

// Config holds the database connection parameters.
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	// SSLMode is passed through to lib/pq. It defaults to "disable" for local
	// compose, but is configurable precisely so a real deployment can require
	// TLS without editing this package.
	SSLMode string

	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// LoadConfigFromEnv reads database configuration from environment variables.
func LoadConfigFromEnv() Config {
	return Config{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     getEnv("DB_PORT", "5432"),
		User:     getEnv("DB_USER", "user"),
		Password: getEnv("DB_PASSWORD", "password"),
		DBName:   getEnv("DB_NAME", "database"),
		SSLMode:  getEnv("DB_SSLMODE", "disable"),

		MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", DefaultMaxOpenConns),
		MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", DefaultMaxIdleConns),
		ConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", DefaultConnMaxLifetime),
		ConnMaxIdleTime: getEnvDuration("DB_CONN_MAX_IDLE_TIME", DefaultConnMaxIdleTime),
	}
}

// DSN builds the connection string.
//
// It is assembled through net/url rather than by formatting a string, so that
// a password containing a space, a quote or an ampersand is escaped instead of
// truncating the DSN or injecting an extra connection parameter into it.
func (c Config) DSN() string {
	sslMode := c.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}

	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   c.Host + ":" + c.Port,
		Path:   "/" + c.DBName,
	}
	q := u.Query()
	q.Set("sslmode", sslMode)
	u.RawQuery = q.Encode()

	return u.String()
}

// withDefaults fills in the pool settings a zero-value Config leaves unset.
//
// This matters because database/sql reads zero as "unlimited" for
// MaxOpenConns, so a caller building a Config by hand -- tests, tools, anything
// not going through LoadConfigFromEnv -- would otherwise get exactly the
// unbounded pool these limits exist to prevent. Defaults belong here rather
// than only in the env loader for that reason.
func (c Config) withDefaults() Config {
	if c.SSLMode == "" {
		c.SSLMode = "disable"
	}
	if c.MaxOpenConns <= 0 {
		c.MaxOpenConns = DefaultMaxOpenConns
	}
	if c.MaxIdleConns <= 0 {
		c.MaxIdleConns = DefaultMaxIdleConns
	}
	if c.ConnMaxLifetime <= 0 {
		c.ConnMaxLifetime = DefaultConnMaxLifetime
	}
	if c.ConnMaxIdleTime <= 0 {
		c.ConnMaxIdleTime = DefaultConnMaxIdleTime
	}
	return c
}

// Connect opens a pooled connection and verifies the database is reachable.
func Connect(cfg Config) (*sqlx.DB, error) {
	cfg = cfg.withDefaults()

	database, err := sqlx.Open("postgres", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Applied before the first query, so no request can run against an
	// unbounded pool.
	database.SetMaxOpenConns(cfg.MaxOpenConns)
	database.SetMaxIdleConns(cfg.MaxIdleConns)
	database.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	database.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), ConnectTimeout)
	defer cancel()

	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("connecting to database %s at %s:%s: %w", cfg.DBName, cfg.Host, cfg.Port, err)
	}

	// slog, not log.Printf: every other line a service emits is structured
	// JSON, and one plain line will not parse in an aggregator.
	slog.Info("connected to database",
		"database", cfg.DBName,
		"host", cfg.Host,
		"port", cfg.Port,
		"sslmode", cfg.SSLMode,
		"max_open_conns", cfg.MaxOpenConns,
	)
	return database, nil
}

// ConnectFromEnv is a helper that connects to the database using environment variables.
func ConnectFromEnv() (*sqlx.DB, error) {
	return Connect(LoadConfigFromEnv())
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		slog.Warn("ignoring invalid value, using default", "var", key, "value", raw, "default", fallback)
		return fallback
	}
	return v
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return fallback
	}
	v, err := time.ParseDuration(raw)
	if err != nil || v <= 0 {
		slog.Warn("ignoring invalid value, using default", "var", key, "value", raw, "default", fallback)
		return fallback
	}
	return v
}
