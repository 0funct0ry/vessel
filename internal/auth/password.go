// Package auth provides authentication primitives used by Vessel's HTTP API.
package auth

import "golang.org/x/crypto/bcrypt"

const (
	BcryptCost      = 12
	MinimumPassword = 8
)

func HashPassword(password string) (string, error) {
	encoded, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	return string(encoded), err
}

func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
