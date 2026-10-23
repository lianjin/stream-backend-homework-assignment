package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/GetStream/stream-backend-homework-assignment/api"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

// Postgres provides storage in PostgreSQL.
type Postgres struct {
	bun *bun.DB
}

// Connect connects to the database and ping the DB to ensure the connection is
// working.
func Connect(ctx context.Context, connStr string) (*Postgres, error) {
	sqlDB := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr)))
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}
	db := bun.NewDB(sqlDB, pgdialect.New())
	return &Postgres{
		bun: db,
	}, nil
}

// ListMessages returns messages ordered by created_at DESC, id DESC.
func (pg *Postgres) ListMessages(ctx context.Context, limit int, cursor api.MessageCursor) ([]api.Message, error) {
	var msgs []message
	q := pg.bun.NewSelect().
		Model(&msgs).
		OrderExpr("created_at DESC, id DESC").
		Limit(limit)

	if !cursor.IsZero() {
		q = q.Where("created_at < ? OR (created_at = ? AND id < ?)",
			cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	out := make([]api.Message, len(msgs))
	for i, m := range msgs {
		out[i] = m.APIMessage()
	}
	return out, nil
}

// InsertMessage inserts a message into the database. The returned message
// holds auto generated fields, such as the message id.
func (pg *Postgres) InsertMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	m := &message{
		MessageText: msg.Text,
		UserID:      msg.UserID,
	}
	if _, err := pg.bun.NewInsert().Model(m).Exec(ctx); err != nil {
		return api.Message{}, fmt.Errorf("insert: %w", err)
	}
	return m.APIMessage(), nil
}

func (pg *Postgres) MessageExists(ctx context.Context, messageID string) (bool, error) {
	exists, err := pg.bun.NewSelect().
		Model((*message)(nil)).
		Where("id = ?", messageID).
		Exists(ctx)
	if err != nil {
		return false, fmt.Errorf("check message: %w", err)
	}
	return exists, nil
}

func (pg *Postgres) InsertReaction(ctx context.Context, in api.Reaction) (api.Reaction, error) {
	m := &reaction{
		MessageID: in.MessageID,
		Type:      in.Type,
		Score:     in.Score,
		UserID:    in.UserID,
	}
	_, err := pg.bun.NewInsert().
		Model(m).
		On("CONFLICT (message_id, user_id, type) DO UPDATE").
		Set("score = EXCLUDED.score").
		Returning("*").
		Exec(ctx)
	if err != nil {
		return api.Reaction{}, fmt.Errorf("upsert reaction: %w", err)
	}
	return m.APIReaction(), nil
}

func (pg *Postgres) ListReactions(ctx context.Context, messageIDs []string) ([]api.Reaction, error) {
	if len(messageIDs) == 0 {
		return nil, nil
	}
	var rows []reaction
	if err := pg.bun.NewSelect().
		Model(&rows).
		Where("message_id IN (?)", bun.In(messageIDs)).
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("scan reactions: %w", err)
	}
	out := make([]api.Reaction, len(rows))
	for i, row := range rows {
		out[i] = row.APIReaction()
	}
	return out, nil
}
