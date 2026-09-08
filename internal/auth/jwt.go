package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/0funct0ry/vessel/internal/store"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("expired token")
)

type Claims struct {
	Name string     `json:"name"`
	Role store.Role `json:"role"`
	jwt.RegisteredClaims
}

type Tokens struct {
	secret []byte
	TTL    time.Duration
	Now    func() time.Time
}

func LoadTokens(ctx context.Context, s store.Store, ttl time.Duration) (*Tokens, bool, error) {
	secret, err := s.GetSetting(ctx, "jwt_secret")
	generated := false
	if errors.Is(err, store.ErrNotFound) {
		bytes := make([]byte, 32)
		if _, err := rand.Read(bytes); err != nil {
			return nil, false, fmt.Errorf("generate jwt secret: %w", err)
		}
		secret = base64.RawStdEncoding.EncodeToString(bytes)
		if err := s.SetSetting(ctx, "jwt_secret", secret); err != nil {
			return nil, false, fmt.Errorf("store jwt secret: %w", err)
		}
		generated = true
	} else if err != nil {
		return nil, false, fmt.Errorf("load jwt secret: %w", err)
	}
	return &Tokens{secret: []byte(secret), TTL: ttl, Now: time.Now}, generated, nil
}

func (t *Tokens) Issue(u store.User) (string, error) {
	now := t.now()
	jti := make([]byte, 16)
	if _, err := rand.Read(jti); err != nil {
		return "", err
	}
	claims := Claims{Name: u.Username, Role: u.Role, RegisteredClaims: jwt.RegisteredClaims{
		Subject: fmt.Sprint(u.ID), IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(t.TTL)), ID: base64.RawURLEncoding.EncodeToString(jti),
	}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
}

func (t *Tokens) Parse(raw string) (Claims, error) {
	var claims Claims
	parsed, err := jwt.ParseWithClaims(raw, &claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return t.secret, nil
	}, jwt.WithTimeFunc(t.now))
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return Claims{}, ErrExpiredToken
		}
		return Claims{}, ErrInvalidToken
	}
	if !parsed.Valid {
		return Claims{}, ErrInvalidToken
	}
	return claims, nil
}

func (t *Tokens) now() time.Time {
	if t.Now != nil {
		return t.Now().UTC()
	}
	return time.Now().UTC()
}
