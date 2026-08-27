package auth

import (
	"strings"
	"testing"
)

// A PEM key is multi-line, and every hop between `genkeys` and a running
// container encodes that differently. These cover the encodings actually seen
// in deployment, because telling "the key is wrong" apart from "the newlines
// were lost" from the error alone is otherwise guesswork.
func TestParseAcceptsTransportEncodings(t *testing.T) {
	privPEM, pubPEM, err := GenerateKeyPair(MinRSAKeyBits)
	if err != nil {
		t.Fatalf("generating key pair: %v", err)
	}

	encodings := map[string]func(string) string{
		"pristine": func(s string) string { return s },
		"escaped newlines": func(s string) string {
			return strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", `\n`)
		},
		"quoted and escaped": func(s string) string {
			return `"` + strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", `\n`) + `"`
		},
		"quoted with real newlines": func(s string) string { return `"` + s + `"` },
		"CRLF":                      func(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") },
		"surrounding whitespace":    func(s string) string { return "\n  " + s + "  \n" },
		"no trailing newline":       func(s string) string { return strings.TrimRight(s, "\n") },
	}

	for name, encode := range encodings {
		t.Run("private/"+name, func(t *testing.T) {
			key, err := parseRSAPrivateKey([]byte(encode(string(privPEM))))
			if err != nil {
				t.Fatalf("parsing private key: %v", err)
			}
			if got := key.N.BitLen(); got != MinRSAKeyBits {
				t.Errorf("key size = %d, want %d", got, MinRSAKeyBits)
			}
		})

		t.Run("public/"+name, func(t *testing.T) {
			if _, err := parseRSAPublicKey([]byte(encode(string(pubPEM)))); err != nil {
				t.Fatalf("parsing public key: %v", err)
			}
		})
	}
}

// Tolerating transport damage must not extend to tolerating a bad key. Each of
// these produced a real crash loop in deployment, and each must still fail.
func TestParseRejectsDamagedKeys(t *testing.T) {
	privPEM, _, err := GenerateKeyPair(MinRSAKeyBits)
	if err != nil {
		t.Fatalf("generating key pair: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(privPEM), "\n"), "\n")

	// A single altered base64 character in the CRT region -- the exact
	// signature of a value that lost characters in transit.
	damaged := append([]string(nil), lines...)
	idx := len(damaged) * 8 / 10
	body := []byte(damaged[idx])
	if body[5] == 'A' {
		body[5] = 'B'
	} else {
		body[5] = 'A'
	}
	damaged[idx] = string(body)

	cases := map[string]string{
		"corrupted body":  strings.Join(damaged, "\n"),
		"truncated":       strings.Join(lines[:len(lines)-2], "\n") + "\n" + lines[len(lines)-1],
		"empty":           "",
		"not PEM at all":  "definitely-not-a-key",
		"unmatched quote": `"` + strings.Join(lines, "\n"),
	}

	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseRSAPrivateKey([]byte(input)); err == nil {
				t.Fatal("expected an error, got none")
			}
		})
	}
}
