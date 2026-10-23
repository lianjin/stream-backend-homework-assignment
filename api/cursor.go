package api

import (
	"fmt"
	"strings"
	"time"
)

const cursorSep = "|"

// A MessageCursor is a keyset position for listing messages.
type MessageCursor struct {
	CreatedAt time.Time
	ID        string
}

func (c MessageCursor) IsZero() bool {
	return c.ID == "" && c.CreatedAt.IsZero()
}

func (c MessageCursor) String() string {
	if c.IsZero() {
		return ""
	}
	return c.CreatedAt.UTC().Format(time.RFC3339Nano) + cursorSep + c.ID
}

func ParseMessageCursor(raw string) (MessageCursor, error) {
	if raw == "" {
		return MessageCursor{}, nil
	}
	at, id, ok := strings.Cut(raw, cursorSep)
	if !ok || at == "" || id == "" {
		return MessageCursor{}, fmt.Errorf("invalid cursor %q", raw)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return MessageCursor{}, fmt.Errorf("invalid cursor time: %w", err)
	}
	return MessageCursor{CreatedAt: createdAt, ID: id}, nil
}

func cursorFromMessages(msgs []Message) string {
	if len(msgs) == 0 {
		return ""
	}
	last := msgs[len(msgs)-1]
	return MessageCursor{CreatedAt: last.CreatedAt, ID: last.ID}.String()
}
