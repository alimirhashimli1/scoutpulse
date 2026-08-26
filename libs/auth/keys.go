package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"sync"
)

// Environment variables configuring token keys.
const (
	// PrivateKeyEnvVar holds a PEM-encoded RSA private key. Only the token
	// issuer (identity-svc) should have it.
	PrivateKeyEnvVar = "JWT_PRIVATE_KEY"
	// PublicKeyEnvVar holds a PEM-encoded RSA public key, for services that
	// only need to verify.
	PublicKeyEnvVar = "JWT_PUBLIC_KEY"
	// JWKSURLEnvVar points at an issuer's JWKS document. Preferred over a
	// pinned public key, because key rotation then needs no redeploy.
	JWKSURLEnvVar = "JWKS_URL"
)

// MinRSAKeyBits is the smallest RSA key accepted.
const MinRSAKeyBits = 2048

var (
	keysMu sync.RWMutex

	// signingKey is set only on the issuer. A service with no signing key
	// can verify tokens but cannot mint them -- which is the entire point of
	// moving off a shared symmetric secret: previously any service that could
	// check a token could also forge an admin one.
	signingKey   *rsa.PrivateKey
	signingKeyID string

	// verificationKeys maps a key id to its public key. More than one may be
	// present during a rotation, so tokens signed with the outgoing key stay
	// valid until they expire.
	verificationKeys = map[string]*rsa.PublicKey{}
)

// SetSigningKey installs the private key used to sign tokens, and registers
// the matching public key for verification.
func SetSigningKey(pemBytes []byte) error {
	key, err := parseRSAPrivateKey(pemBytes)
	if err != nil {
		return err
	}
	if err := checkKeySize(&key.PublicKey); err != nil {
		return err
	}

	kid, err := KeyID(&key.PublicKey)
	if err != nil {
		return err
	}

	keysMu.Lock()
	defer keysMu.Unlock()
	signingKey = key
	signingKeyID = kid
	verificationKeys[kid] = &key.PublicKey
	return nil
}

// AddVerificationKey registers a public key for verification. Calling it more
// than once is how a rotation is staged: register the new key everywhere
// before the issuer starts signing with it.
func AddVerificationKey(pemBytes []byte) error {
	key, err := parseRSAPublicKey(pemBytes)
	if err != nil {
		return err
	}

	// The size check matters more here than on the signing key. A key small
	// enough to factor lets an attacker mint tokens this service will accept,
	// so a weak *verification* key is the more dangerous of the two -- and it
	// is the one that arrives from outside, via configuration or a JWKS
	// document.
	if err := checkKeySize(key); err != nil {
		return err
	}

	kid, err := KeyID(key)
	if err != nil {
		return err
	}

	keysMu.Lock()
	defer keysMu.Unlock()
	verificationKeys[kid] = key
	return nil
}

// checkKeySize rejects an RSA key too small to be trusted.
func checkKeySize(pub *rsa.PublicKey) error {
	if pub.N.BitLen() < MinRSAKeyBits {
		return fmt.Errorf("auth: RSA key must be at least %d bits, got %d", MinRSAKeyBits, pub.N.BitLen())
	}
	return nil
}

// LoadSignerFromEnv configures this process to issue tokens, reading the
// private key from JWT_PRIVATE_KEY. Only the identity service should call it.
func LoadSignerFromEnv() error {
	raw, ok := os.LookupEnv(PrivateKeyEnvVar)
	if !ok || raw == "" {
		return fmt.Errorf("auth: %s is not set", PrivateKeyEnvVar)
	}
	return SetSigningKey([]byte(raw))
}

// LoadFromEnv configures token verification from the environment, accepting
// either a pinned public key or a JWKS URL.
//
// A service that only verifies must not be given the private key. That is what
// stops a compromised downstream service from minting an administrator token.
func LoadFromEnv() error {
	if raw := os.Getenv(PublicKeyEnvVar); raw != "" {
		return AddVerificationKey([]byte(raw))
	}
	if url := os.Getenv(JWKSURLEnvVar); url != "" {
		return LoadJWKS(url)
	}
	return fmt.Errorf("auth: set %s or %s to verify tokens", PublicKeyEnvVar, JWKSURLEnvVar)
}

// KeyID derives a stable identifier for a public key: the base64url-encoded
// SHA-256 of its DER encoding. Deriving it from the key means the id never
// disagrees with the key it names.
func KeyID(pub *rsa.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", fmt.Errorf("auth: encoding public key: %w", err)
	}
	sum := sha256.Sum256(der)
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// normalisePEM makes a PEM block survive the trip through an environment
// variable.
//
// A PEM block is multi-line, and the places these keys pass through disagree
// about how to carry that. `cmd/genkeys` writes `KEY="...\n..."` because a
// .env assignment is one line, and Docker Compose un-escapes it on the way in.
// A hosting dashboard generally does not: the value arrives with the quotes
// still attached, or with a literal backslash-n where a newline belongs, and
// pem.Decode then reports "not valid PEM" -- which reads like a bad key rather
// than a transport problem, and cost an evening to tell apart.
//
// So both encodings are accepted. This is only about how the bytes were
// carried; a key that decodes to invalid material still fails below, exactly
// as it did before.
func normalisePEM(raw []byte) []byte {
	s := strings.TrimSpace(string(raw))

	// A quoted .env value pasted verbatim. Only a matched pair is stripped,
	// so a stray quote still fails loudly rather than being silently chewed.
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			s = s[1 : len(s)-1]
		}
	}

	// Escaped newlines, but only when there are no real ones -- a key that
	// already has line structure is left exactly as it is.
	if !strings.Contains(s, "\n") {
		s = strings.ReplaceAll(s, `\n`, "\n")
	}

	return []byte(strings.TrimSpace(s) + "\n")
}

func parseRSAPrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(normalisePEM(pemBytes))
	if block == nil {
		return nil, fmt.Errorf("auth: private key is not valid PEM")
	}

	// Accept both PKCS#1 ("RSA PRIVATE KEY") and PKCS#8 ("PRIVATE KEY"),
	// since key-generation tools differ in which they emit.
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("auth: parsing private key: %w", err)
	}

	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("auth: private key must be RSA, got %T", parsed)
	}
	return key, nil
}

func parseRSAPublicKey(pemBytes []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(normalisePEM(pemBytes))
	if block == nil {
		return nil, fmt.Errorf("auth: public key is not valid PEM")
	}

	if key, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return key, nil
	}

	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("auth: parsing public key: %w", err)
	}

	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("auth: public key must be RSA, got %T", parsed)
	}
	return key, nil
}

// GenerateKeyPair produces a PEM-encoded RSA key pair. It exists for tests and
// for local development; production keys should be generated and stored by
// whatever manages your secrets.
func GenerateKeyPair(bits int) (privatePEM, publicPEM []byte, err error) {
	if bits < MinRSAKeyBits {
		bits = MinRSAKeyBits
	}

	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, nil, fmt.Errorf("auth: generating key: %w", err)
	}

	privDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("auth: encoding private key: %w", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("auth: encoding public key: %w", err)
	}

	privatePEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})
	publicPEM = pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	return privatePEM, publicPEM, nil
}

// resetKeysForTest clears all installed keys.
func resetKeysForTest() {
	keysMu.Lock()
	defer keysMu.Unlock()
	signingKey = nil
	signingKeyID = ""
	verificationKeys = map[string]*rsa.PublicKey{}
}
