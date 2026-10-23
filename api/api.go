package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// A DB provides a storage layer that persists messages.
type DB interface {
	ListMessages(ctx context.Context, limit int, cursor MessageCursor) ([]Message, error)
	InsertMessage(ctx context.Context, msg Message) (Message, error)
	MessageExists(ctx context.Context, messageID string) (bool, error)
	InsertReaction(ctx context.Context, reaction Reaction) (Reaction, error)
	ListReactions(ctx context.Context, messageIDs []string) ([]Reaction, error)
}

// A Cache provides a storage layer that caches messages.
type Cache interface {
	ListMessages(ctx context.Context) ([]Message, error)
	InsertMessage(ctx context.Context, msg Message) error
	ReplaceMessages(ctx context.Context, msgs []Message) error
}

// API provides the REST endpoints for the application.
type API struct {
	Logger *slog.Logger
	DB     DB
	Cache  Cache

	once    sync.Once
	mux     *http.ServeMux
	cacheMu sync.Mutex
}

func (a *API) setupRoutes() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /messages", a.listMessages)
	mux.HandleFunc("POST /messages", a.createMessage)
	mux.HandleFunc("POST /messages/{messageID}/reactions", a.createReaction)

	a.mux = mux
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.once.Do(a.setupRoutes)
	a.Logger.Info("Request received", "method", r.Method, "path", r.URL.Path)
	a.mux.ServeHTTP(w, r)
}

func (a *API) respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		a.Logger.Error("Could not encode JSON body", "error", err.Error())
	}
}

func (a *API) respondError(w http.ResponseWriter, status int, err error, msg string) {
	type response struct {
		Error string `json:"error"`
	}
	a.Logger.Error("Error", "error", err.Error())
	a.respond(w, status, response{Error: msg})
}

func (a *API) createMessage(w http.ResponseWriter, r *http.Request) {
	type (
		request struct {
			Text   string `json:"text"`
			UserID string `json:"user_id"`
		}
		response struct {
			ID        string `json:"id"`
			Text      string `json:"text"`
			UserID    string `json:"user_id"`
			CreatedAt string `json:"created_at"`
		}
	)

	var body request
	err := json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		a.respondError(w, http.StatusBadRequest, err, "Could not decode request body")
		return
	}
	r.Body.Close()

	msg, err := a.DB.InsertMessage(r.Context(), Message{
		Text:      body.Text,
		UserID:    body.UserID,
		CreatedAt: time.Now(),
	})
	if err != nil {
		a.respondError(w, http.StatusInternalServerError, err, "Could not insert message")
		return
	}
	a.Logger.Info("Inserted message", "id", msg.ID, "user_id", msg.UserID)

	a.cacheMu.Lock()
	err = a.Cache.InsertMessage(r.Context(), msg)
	a.cacheMu.Unlock()
	if err != nil {
		a.Logger.Error("Could not cache message", "error", err.Error(), "id", msg.ID)
	} else {
		a.Logger.Info("Cached message", "id", msg.ID)
	}

	res := response{
		ID:        msg.ID,
		Text:      msg.Text,
		UserID:    msg.UserID,
		CreatedAt: msg.CreatedAt.Format(time.RFC1123),
	}
	a.respond(w, http.StatusCreated, res)
}

