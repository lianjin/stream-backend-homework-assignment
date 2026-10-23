package postgres

import (
	"time"

	"github.com/GetStream/stream-backend-homework-assignment/api"
)

// A message represents a message in the database.
type message struct {
	ID          string    `bun:",pk,type:uuid,default:uuid_generate_v4()"`
	MessageText string    `bun:"message_text,notnull"`
	UserID      string    `bun:",notnull"`
	CreatedAt   time.Time `bun:",nullzero,default:now()"`
}

func (m message) APIMessage() api.Message {
	return api.Message{
		ID:        m.ID,
		Text:      m.MessageText,
		UserID:    m.UserID,
		CreatedAt: m.CreatedAt,
	}
}

// A reaction represents a reaction to a message in the database.
type reaction struct {
	ID        string    `bun:",pk,type:uuid,nullzero,default:gen_random_uuid()"`
	MessageID string    `bun:"message_id,notnull,type:uuid"`
	Type      string    `bun:",notnull"`
	Score     int       `bun:",notnull,default:1"`
	UserID    string    `bun:",notnull"`
	CreatedAt time.Time `bun:",nullzero,default:now()"`
}

func (r reaction) APIReaction() api.Reaction {
	return api.Reaction{
		ID:        r.ID,
		MessageID: r.MessageID,
		Type:      r.Type,
		Score:     r.Score,
		UserID:    r.UserID,
		CreatedAt: r.CreatedAt,
	}
}
