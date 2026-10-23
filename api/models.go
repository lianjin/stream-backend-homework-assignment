package api

import (
	"errors"
	"time"
)

var ErrMessageNotFound = errors.New("message not found")

// A Message represents a persisted message.
type Message struct {
	ID        string
	Text      string
	UserID    string
	CreatedAt time.Time
	Reactions []Reaction
}

// A Reaction represents a reaction to a message such as a like.
type Reaction struct {
	ID        string
	MessageID string
	Type      string
	Score     int
	UserID    string
	CreatedAt time.Time
}
