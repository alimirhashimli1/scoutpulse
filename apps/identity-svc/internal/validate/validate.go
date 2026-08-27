// Package validate checks the fields an account is created from.
//
// The transport layer already refuses oversized bodies, unknown fields and
// trailing JSON (see httpx.DecodeJSON), and every query in this service is
// parameterised, so neither SQL nor JSON injection is the risk here. What was
// missing is plainer: the service accepted any non-empty string as an email
// address and any string at all as a username.
package validate

import (
	"fmt"
	"net/mail"
	"strings"
	"unicode"

	"github.com/scoutpulse/libs/platform/apperr"
)

const (
	// MinPasswordLength is the floor. Length beats composition rules: a long
	// passphrase is stronger than eight characters with a digit bolted on, and
	// composition rules mostly teach people to write "Password1!".
	MinPasswordLength = 8

	// MaxPasswordBytes is bcrypt's hard limit, not a policy choice.
	//
	// bcrypt hashes at most 72 bytes. Everything beyond is ignored, so two
	// passphrases sharing a 72-byte prefix are the same password to the
	// algorithm. Go's implementation now returns an error rather than
	// truncating silently — which, unchecked, surfaces as a 500 on a
	// registration that looked reasonable to the person typing it.
	//
	// Bytes, not characters: "é" is two bytes and an emoji is four, so a
	// 40-character passphrase can exceed this.
	MaxPasswordBytes = 72

	MinUsernameLength = 3
	MaxUsernameLength = 32

	// MaxEmailLength is the practical ceiling from RFC 5321 on a full address.
	MaxEmailLength = 254
)

// Username checks a username and returns it unchanged.
//
// The character set is deliberately narrow. A username appears in URLs, logs
// and audit trails, and allowing whitespace or control characters invites both
// display bugs and impersonation — "admin " and "admin" read identically in
// most interfaces.
func Username(raw string) (string, error) {
	name := strings.TrimSpace(raw)

	if name == "" {
		return "", apperr.Invalid("username is required")
	}
	// Counted in runes, so a name in a non-Latin script is measured the way its
	// writer would count it.
	if n := len([]rune(name)); n < MinUsernameLength || n > MaxUsernameLength {
		return "", apperr.Invalid(fmt.Sprintf(
			"username must be between %d and %d characters", MinUsernameLength, MaxUsernameLength))
	}

	for _, r := range name {
		if !isUsernameRune(r) {
			return "", apperr.Invalid(
				"username may contain only letters, digits, dots, hyphens and underscores")
		}
	}

	// A leading or trailing separator is almost always a typo, and makes two
	// visually similar names distinct.
	if strings.ContainsAny(name[:1], "._-") || strings.ContainsAny(name[len(name)-1:], "._-") {
		return "", apperr.Invalid("username must start and end with a letter or digit")
	}

	return name, nil
}

func isUsernameRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_'
}

// Email validates an address and returns it normalised to lowercase.
//
// Normalising matters for more than tidiness: addresses are matched against
// existing accounts when linking an external sign-in, and "A@example.com"
// finding no match for "a@example.com" would silently create a second account
// for one person.
func Email(raw string) (string, error) {
	address := strings.TrimSpace(raw)

	if address == "" {
		return "", apperr.Invalid("email is required")
	}
	if len(address) > MaxEmailLength {
		return "", apperr.Invalid(fmt.Sprintf("email must be at most %d characters", MaxEmailLength))
	}

	// net/mail parses the addr-spec grammar properly, which a regular
	// expression does not. It also accepts the display-name form
	// `Ali <a@example.com>`, so the parsed address is compared back to the
	// input to reject that.
	parsed, err := mail.ParseAddress(address)
	if err != nil || !strings.EqualFold(parsed.Address, address) {
		return "", apperr.Invalid("that does not look like an email address")
	}

	// Guards against an address whose local part or domain is empty, which the
	// parser tolerates in some forms.
	at := strings.LastIndex(parsed.Address, "@")
	if at <= 0 || at == len(parsed.Address)-1 || !strings.Contains(parsed.Address[at+1:], ".") {
		return "", apperr.Invalid("that does not look like an email address")
	}

	return strings.ToLower(parsed.Address), nil
}

// Password checks length only.
//
// The upper bound is the one that would otherwise bite: without it, a long
// passphrase reaches bcrypt, is rejected there, and becomes an opaque 500.
func Password(password string) error {
	if len(password) < MinPasswordLength {
		return apperr.Invalid(fmt.Sprintf(
			"password must be at least %d characters", MinPasswordLength))
	}
	if len(password) > MaxPasswordBytes {
		return apperr.Invalid(fmt.Sprintf(
			"password must be at most %d bytes; note that accented characters and emoji count as several",
			MaxPasswordBytes))
	}
	return nil
}
