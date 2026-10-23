package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"
)

const (
	maxReactionTypeLen = 32
	maxUserIDLen       = 255
	defaultScore       = 1
)

func (a *API) createReaction(w http.ResponseWriter, r *http.Request) {
	type (
		request struct {
			Type   string `json:"type"`
			Score  *int   `json:"score"`
			UserID string `json:"user_id"`
		}
		response struct {
			ID        string `json:"id"`
			MessageID string `json:"message_id"`
			Type      string `json:"type"`
			Score     int    `json:"score"`
			UserID    string `json:"user_id"`
			CreatedAt string `json:"created_at"`
		}
	)

	messageID := strings.TrimSpace(r.PathValue("messageID"))
	var body request
	err := json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		a.respondError(w, http.StatusBadRequest, err, "Could not decode request body")
		return
	}
	r.Body.Close()

	score := defaultScore
	if body.Score != nil {
		score = *body.Score
	}
	reaction := Reaction{
		MessageID: messageID,
		Type:      strings.TrimSpace(body.Type),
		Score:     score,
		UserID:    strings.TrimSpace(body.UserID),
	}
	if err := validateReaction(reaction); err != nil {
		a.respondError(w, http.StatusBadRequest, err, err.Error())
		return
	}

	exists, err := a.DB.MessageExists(r.Context(), reaction.MessageID)
	if err != nil {
		a.respondError(w, http.StatusInternalServerError, err, "Could not insert reaction")
		return
	}
	if !exists {
		a.respondError(w, http.StatusNotFound, ErrMessageNotFound, "Message not found")
		return
	}

	saved, err := a.DB.InsertReaction(r.Context(), reaction)
	if err != nil {
		if errors.Is(err, ErrMessageNotFound) {
			a.respondError(w, http.StatusNotFound, err, "Message not found")
			return
		}
		a.respondError(w, http.StatusInternalServerError, err, "Could not insert reaction")
		return
	}
	a.Logger.Info("Upserted reaction",
		"id", saved.ID,
		"message_id", saved.MessageID,
		"user_id", saved.UserID,
		"type", saved.Type,
		"score", saved.Score,
	)

	a.respond(w, http.StatusCreated, response{
		ID:        saved.ID,
		MessageID: saved.MessageID,
		Type:      saved.Type,
		Score:     saved.Score,
		UserID:    saved.UserID,
		CreatedAt: saved.CreatedAt.Format(time.RFC1123),
	})
}

func validateReaction(r Reaction) error {
	if r.MessageID == "" {
		return errors.New("message id is required")
	}
	if r.UserID == "" {
		return errors.New("user_id is required")
	}
	if len(r.UserID) > maxUserIDLen {
		return errors.New("user_id is too long")
	}
	if r.Type == "" {
		return errors.New("type is required")
	}
	if len(r.Type) > maxReactionTypeLen {
		return errors.New("type is too long")
	}
	for _, c := range r.Type {
		if unicode.IsSpace(c) {
			return errors.New("type must not contain spaces")
		}
	}
	if r.Score < 1 {
		return errors.New("score must be a positive integer")
	}
	return nil
}
