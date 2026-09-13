// Package store defines Vessel's persistence boundary.
package store

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("store: not found")
	ErrConflict = errors.New("store: conflict")
)

type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

type DeliveryStatus string

const (
	DeliveryPending DeliveryStatus = "pending"
	DeliverySuccess DeliveryStatus = "success"
	DeliveryFailed  DeliveryStatus = "failed"
	DeliveryDead    DeliveryStatus = "dead"
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         Role
	CreatedAt    time.Time
	LastLoginAt  *time.Time
}

type Token struct {
	ID         string
	UserID     int64
	Name       string
	Hash       string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
}

type Webhook struct {
	ID          string
	Name        string
	URL         string
	Secret      *string
	Enabled     bool
	EventTypes  []string
	Filters     []byte
	Headers     []byte
	MaxAttempts int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Stack is a stored Compose stack definition. Vessel keeps only the source of
// truth (the compose file and its .env); everything else about a stack —
// status, containers, services — is derived live from Docker.
type Stack struct {
	ID          string
	Name        string
	ComposeYAML string
	EnvContent  string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Delivery struct {
	ID           string
	WebhookID    string
	EventID      string
	Payload      []byte
	Attempt      int
	Status       DeliveryStatus
	StatusCode   *int
	ResponseMS   *int
	ResponseBody *string
	Error        *string
	CreatedAt    time.Time
	NextRetryAt  *time.Time
}

type DeliveryQuery struct {
	Limit  int
	Cursor string
}

type Event struct {
	ID        string
	Type      string
	Action    string
	SubjectID string
	Name      string
	Attrs     []byte
	CreatedAt time.Time
}

type EventQuery struct {
	Limit int
	Types []string
}

// Store is deliberately expressed in domain values so API, auth, and webhook
// code do not need to know which persistence backend is active.
type Store interface {
	GetSetting(context.Context, string) (string, error)
	SetSetting(context.Context, string, string) error

	CreateUser(context.Context, User) (User, error)
	GetUser(context.Context, int64) (User, error)
	GetUserByUsername(context.Context, string) (User, error)
	ListUsers(context.Context) ([]User, error)
	UpdateUser(context.Context, User) (User, error)
	DeleteUser(context.Context, int64) error

	CreateToken(context.Context, Token) (Token, error)
	GetTokenByHash(context.Context, string) (Token, error)
	ListTokensByUser(context.Context, int64) ([]Token, error)
	DeleteToken(context.Context, string, int64) error
	TouchTokenLastUsed(context.Context, string, time.Time) error

	CreateWebhook(context.Context, Webhook) (Webhook, error)
	GetWebhook(context.Context, string) (Webhook, error)
	ListWebhooks(context.Context) ([]Webhook, error)
	UpdateWebhook(context.Context, Webhook) (Webhook, error)
	DeleteWebhook(context.Context, string) error

	CreateStack(context.Context, Stack) (Stack, error)
	GetStack(context.Context, string) (Stack, error)
	GetStackByName(context.Context, string) (Stack, error)
	ListStacks(context.Context) ([]Stack, error)
	UpdateStack(context.Context, Stack) (Stack, error)
	DeleteStack(context.Context, string) error

	CreateDelivery(context.Context, Delivery) (Delivery, error)
	GetDelivery(context.Context, string) (Delivery, error)
	ListDeliveries(context.Context, string, DeliveryQuery) ([]Delivery, error)
	UpdateDelivery(context.Context, Delivery) (Delivery, error)

	CreateEvent(context.Context, Event) (Event, error)
	ListEvents(context.Context, EventQuery) ([]Event, error)
	DeleteEvent(context.Context, string) error
	ClearEvents(context.Context) (int64, error)

	Close() error
}
