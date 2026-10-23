package api

import (
	"context"
	"net/http"
	"time"
)

const messagePageSize = 10

func (a *API) listMessages(w http.ResponseWriter, r *http.Request) {
	cursor, err := ParseMessageCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		a.respondError(w, http.StatusBadRequest, err, "Invalid cursor")
		return
	}

	msgs, err := a.listMessagesPage(r.Context(), cursor)
	if err != nil {
		a.respondError(w, http.StatusInternalServerError, err, "Could not list messages")
		return
	}

	if err := a.attachReactions(r.Context(), msgs); err != nil {
		a.respondError(w, http.StatusInternalServerError, err, "Could not list messages")
		return
	}

	a.respond(w, http.StatusOK, listMessagesResponse{
		Messages: toListMessages(msgs),
		Cursor:   cursorFromMessages(msgs),
	})
}

type listMessage struct {
	ID            string         `json:"id"`
	Text          string         `json:"text"`
	UserID        string         `json:"user_id"`
	CreatedAt     string         `json:"created_at"`
	ReactionCount int            `json:"reaction_count"`
	Reactions     []listReaction `json:"reactions"`
}

type listReaction struct {
	ID        string `json:"id"`
	MessageID string `json:"message_id"`
	Type      string `json:"type"`
	Score     int    `json:"score"`
	UserID    string `json:"user_id"`
	CreatedAt string `json:"created_at"`
}

type listMessagesResponse struct {
	Messages []listMessage `json:"messages"`
	Cursor   string        `json:"cursor"`
}

func toListMessages(msgs []Message) []listMessage {
	out := make([]listMessage, len(msgs))
	for i, msg := range msgs {
		reactions := toListReactions(msg.Reactions)
		out[i] = listMessage{
			ID:            msg.ID,
			Text:          msg.Text,
			UserID:        msg.UserID,
			CreatedAt:     msg.CreatedAt.Format(time.RFC1123),
			ReactionCount: len(reactions),
			Reactions:     reactions,
		}
	}
	return out
}

func toListReactions(reactions []Reaction) []listReaction {
	if reactions == nil {
		return []listReaction{}
	}
	out := make([]listReaction, len(reactions))
	for i, r := range reactions {
		out[i] = listReaction{
			ID:        r.ID,
			MessageID: r.MessageID,
			Type:      r.Type,
			Score:     r.Score,
			UserID:    r.UserID,
			CreatedAt: r.CreatedAt.Format(time.RFC1123),
		}
	}
	return out
}

func (a *API) attachReactions(ctx context.Context, msgs []Message) error {
	if len(msgs) == 0 {
		return nil
	}
	ids := make([]string, len(msgs))
	for i, msg := range msgs {
		ids[i] = msg.ID
	}
	reactions, err := a.DB.ListReactions(ctx, ids)
	if err != nil {
		return err
	}
	byMessage := make(map[string][]Reaction, len(msgs))
	for _, r := range reactions {
		byMessage[r.MessageID] = append(byMessage[r.MessageID], r)
	}
	for i := range msgs {
		msgs[i].Reactions = byMessage[msgs[i].ID]
	}
	a.Logger.Info("Attached reactions", "messages", len(msgs), "reactions", len(reactions))
	return nil
}

func (a *API) listMessagesPage(ctx context.Context, cursor MessageCursor) ([]Message, error) {
	if cursor.IsZero() {
		return a.loadOrRefreshLatest(ctx)
	}
	return a.listOlderMessages(ctx, cursor)
}

func (a *API) loadOrRefreshLatest(ctx context.Context) ([]Message, error) {
	cached, err := a.Cache.ListMessages(ctx)
	if err != nil {
		a.Logger.Error("Could not list messages from cache", "error", err.Error())
		return a.refreshLatestFromDB(ctx)
	}
	a.Logger.Info("Got messages from cache", "count", len(cached))
	if len(cached) >= messagePageSize {
		return cached[:messagePageSize], nil
	}
	a.Logger.Info("Cache incomplete, refreshing latest page from DB", "count", len(cached))
	return a.refreshLatestFromDB(ctx)
}

func (a *API) refreshLatestFromDB(ctx context.Context) ([]Message, error) {
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()

	cached, err := a.Cache.ListMessages(ctx)
	if err == nil && len(cached) >= messagePageSize {
		a.Logger.Info("Cache filled while waiting to refresh", "count", len(cached))
		return cached[:messagePageSize], nil
	}

	msgs, err := a.DB.ListMessages(ctx, messagePageSize, MessageCursor{})
	if err != nil {
		return nil, err
	}
	a.Logger.Info("Got latest messages from DB", "count", len(msgs))
	a.replaceLatestCache(ctx, msgs)
	return msgs, nil
}

func (a *API) replaceLatestCache(ctx context.Context, msgs []Message) {
	if err := a.Cache.ReplaceMessages(ctx, msgs); err != nil {
		a.Logger.Error("Could not replace message cache", "error", err.Error(), "count", len(msgs))
		return
	}
	a.Logger.Info("Replaced message cache", "count", len(msgs))
}

func (a *API) listOlderMessages(ctx context.Context, cursor MessageCursor) ([]Message, error) {
	msgs, err := a.DB.ListMessages(ctx, messagePageSize, cursor)
	if err != nil {
		return nil, err
	}
	a.Logger.Info("Got messages from DB", "count", len(msgs), "cursor", cursor.String())
	return msgs, nil
}
