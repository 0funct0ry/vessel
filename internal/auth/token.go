package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/oklog/ulid/v2"
)

// TokenPrefix identifies a Vessel personal API token so the auth middleware
// can distinguish it from a session JWT without ambiguity.
const TokenPrefix = "vessel_pat_"

const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// GenerateToken returns a new raw personal API token and its stored hash.
// The raw value is returned to the caller exactly once; only the hash is
// ever persisted.
func GenerateToken() (raw string, hash string, err error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", "", err
	}
	raw = TokenPrefix + ulid.Make().String() + "_" + base62(secret)
	return raw, HashToken(raw), nil
}

// HashToken returns the stable, non-reversible representation of a raw
// token stored at rest. A personal API token is a high-entropy random
// secret, not a low-entropy human password, so a fast hash is used here
// instead of the slow bcrypt cost password.go pays for login credentials.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// IsToken reports whether a bearer value looks like a personal API token
// rather than a session JWT.
func IsToken(raw string) bool {
	return strings.HasPrefix(raw, TokenPrefix)
}

func base62(b []byte) string {
	var sb strings.Builder
	for _, v := range b {
		sb.WriteByte(base62Alphabet[int(v)%len(base62Alphabet)])
	}
	return sb.String()
}
