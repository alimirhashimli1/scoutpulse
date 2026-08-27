// Package mailer sends the few transactional emails this service produces.
//
// Two transports, chosen by whether SMTP is configured. That mirrors how
// external sign-in already behaves: absent credentials mean the feature runs
// in its degraded form rather than the service refusing to start.
package mailer

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Message is one email. Plain text only.
//
// No HTML alternative on purpose: these are a sentence and a link. HTML would
// add a templating surface, a sanitisation obligation, and a second body to
// keep in step, to make a one-line message look marginally nicer.
type Message struct {
	To      string
	Subject string
	Body    string
}

type Mailer interface {
	Send(ctx context.Context, msg Message) error
}

// Config is read from the environment. An empty Host selects the log transport.
type Config struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
	// Timeout bounds the whole conversation with the server. Without it a
	// hung SMTP connection holds the registration request open with it.
	Timeout time.Duration
}

// New returns an SMTP mailer when a host is configured, and a logging one
// otherwise.
func New(cfg Config, log *slog.Logger) Mailer {
	if cfg.Host == "" {
		log.Warn("no SMTP_HOST set, verification emails will be written to the log instead of sent")
		return &LogMailer{log: log}
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.Port == "" {
		cfg.Port = "587"
	}
	return &SMTPMailer{cfg: cfg, log: log}
}

// LogMailer writes the message where a developer can see it.
//
// This is what makes the flow testable with no mail account at all: the
// verification link appears in `docker compose logs identity-svc`, and can be
// pasted into a browser. It is emphatically not for production — an
// unconfigured deployment would silently deliver nothing while appearing to
// work, so `New` warns loudly when it selects this.
type LogMailer struct{ log *slog.Logger }

func (m *LogMailer) Send(_ context.Context, msg Message) error {
	m.log.Info("email not sent: no SMTP configured, printing instead",
		"to", msg.To, "subject", msg.Subject, "body", msg.Body)
	return nil
}

// SMTPMailer delivers over SMTP with STARTTLS.
type SMTPMailer struct {
	cfg Config
	log *slog.Logger
}

func (m *SMTPMailer) Send(ctx context.Context, msg Message) error {
	addr := net.JoinHostPort(m.cfg.Host, m.cfg.Port)

	dialer := &net.Dialer{Timeout: m.cfg.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial smtp %s: %w", addr, err)
	}

	client, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp handshake: %w", err)
	}
	defer func() { _ = client.Quit() }()

	// STARTTLS before authenticating, always. PlainAuth refuses to send
	// credentials over an unencrypted connection anyway, but failing here is
	// clearer than failing at the auth step.
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: m.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	} else if m.cfg.Password != "" {
		return fmt.Errorf("smtp server %s does not offer STARTTLS; refusing to send credentials in the clear", addr)
	}

	if m.cfg.Password != "" {
		auth := smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	if err := client.Mail(m.cfg.From); err != nil {
		return fmt.Errorf("smtp from: %w", err)
	}
	if err := client.Rcpt(msg.To); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write([]byte(compose(m.cfg.From, msg))); err != nil {
		_ = w.Close()
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}

	return nil
}

// compose builds the RFC 5322 message.
//
// Header values are stripped of CR and LF. A newline smuggled into a subject
// or address ends the header and begins another, which is how an attacker adds
// a Bcc to a message the application thinks it controls -- email's own
// injection flaw, and the reason the address is validated before it reaches
// here as well.
func compose(from string, msg Message) string {
	var b strings.Builder
	b.WriteString("From: " + header(from) + "\r\n")
	b.WriteString("To: " + header(msg.To) + "\r\n")
	b.WriteString("Subject: " + header(msg.Subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(msg.Body)
	return b.String()
}

func header(value string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(value)
}
