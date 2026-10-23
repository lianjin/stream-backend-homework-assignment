package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/neilotoole/slogt"
)

func TestAPI_listMessages(t *testing.T) {
	msg1 := Message{
		ID:        "1",
		Text:      "Hello",
		UserID:    "testuser",
		CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	msg2 := Message{
		ID:        "2",
		Text:      "World",
		UserID:    "testuser",
		CreatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
	}

	tests := []struct {
		name       string
		url        string
		db         *testdb
		cache      *testcache
		wantStatus int
		wantBody   string
	}{
		{
			name: "InvalidCursor",
			url:  "/messages?cursor=not-a-cursor",
			cache: &testcache{
				listMessages: func(t *testing.T) ([]Message, error) {
					t.Fatal("cache should not be called")
					return nil, nil
				},
			},
			wantStatus: 400,
			wantBody: `{
				"error": "Invalid cursor"
			}`,
		},
		{
			name: "DBError",
			cache: &testcache{
				listMessages: func(t *testing.T) ([]Message, error) {
					return nil, nil
				},
			},
			db: &testdb{
				listMessages: func(t *testing.T, limit int, cursor MessageCursor) ([]Message, error) {
					return nil, errors.New("something went wrong")
				},
			},
			wantStatus: 500,
			wantBody: `{
				"error": "Could not list messages"
			}`,
		},
		{
			name: "CacheErrorFallbackDB",
			cache: &testcache{
				listMessages: func(t *testing.T) ([]Message, error) {
					return nil, errors.New("something went wrong")
				},
				replaceMessages: func(t *testing.T, msgs []Message) error {
					if len(msgs) != 1 || msgs[0].ID != "1" {
						t.Fatalf("unexpected replace %v", msgs)
					}
					return nil
				},
			},
			db: &testdb{
				listMessages: func(t *testing.T, limit int, cursor MessageCursor) ([]Message, error) {
					if !cursor.IsZero() {
						t.Fatal("expected first-page cursor")
					}
					return []Message{msg1}, nil
				},
			},
			wantStatus: 200,
			wantBody: `{
				"messages": [
					{
						"id": "1",
						"text": "Hello",
						"user_id": "testuser",
						"created_at": "Mon, 01 Jan 2024 00:00:00 UTC",
						"reaction_count": 0,
						"reactions": []
					}
				],
				"cursor": "` + MessageCursor{CreatedAt: msg1.CreatedAt, ID: msg1.ID}.String() + `"
			}`,
		},
		{
			name: "Empty",
			cache: &testcache{
				listMessages: func(t *testing.T) ([]Message, error) {
					return nil, nil
				},
				replaceMessages: func(t *testing.T, msgs []Message) error {
					if len(msgs) != 0 {
						t.Fatalf("unexpected replace %v", msgs)
					}
					return nil
				},
			},
			db: &testdb{
				listMessages: func(t *testing.T, limit int, cursor MessageCursor) ([]Message, error) {
					return nil, nil
				},
			},
			wantStatus: 200,
			wantBody: `{
				"messages": [],
				"cursor": ""
			}`,
		},
		{
			name: "CacheIncompleteRefreshDB",
			cache: &testcache{
				listMessages: func(t *testing.T) ([]Message, error) {
					return []Message{msg1}, nil
				},
				replaceMessages: func(t *testing.T, msgs []Message) error {
					if len(msgs) != 2 {
						t.Fatalf("got %d messages, want 2", len(msgs))
					}
					return nil
				},
			},
			db: &testdb{
				listMessages: func(t *testing.T, limit int, cursor MessageCursor) ([]Message, error) {
					if limit != messagePageSize {
						t.Errorf("limit=%d, want %d", limit, messagePageSize)
					}
					if !cursor.IsZero() {
						t.Fatal("expected first-page cursor")
					}
					return []Message{msg2, msg1}, nil
				},
			},
			wantStatus: 200,
			wantBody: `{
				"messages": [
					{
						"id": "2",
						"text": "World",
						"user_id": "testuser",
						"created_at": "Tue, 02 Jan 2024 00:00:00 UTC",
						"reaction_count": 0,
						"reactions": []
					},
					{
						"id": "1",
						"text": "Hello",
						"user_id": "testuser",
						"created_at": "Mon, 01 Jan 2024 00:00:00 UTC",
						"reaction_count": 0,
						"reactions": []
					}
				],
				"cursor": "` + MessageCursor{CreatedAt: msg1.CreatedAt, ID: msg1.ID}.String() + `"
			}`,
		},
		{
			name: "CacheFull",
			cache: &testcache{
				listMessages: func(t *testing.T) ([]Message, error) {
					return fullCachePage(), nil
				},
			},
			db: &testdb{
				listMessages: func(t *testing.T, limit int, cursor MessageCursor) ([]Message, error) {
					t.Fatal("list messages should not be called when cache is full")
					return nil, nil
				},
			},
			wantStatus: 200,
			wantBody: fullCachePageBody(),
		},
		{
			name: "WithCursor",
			url:  "/messages?cursor=" + url.QueryEscape(MessageCursor{CreatedAt: msg2.CreatedAt, ID: msg2.ID}.String()),
			cache: &testcache{
				listMessages: func(t *testing.T) ([]Message, error) {
					t.Fatal("cache should not be called when cursor is set")
					return nil, nil
				},
			},
			db: &testdb{
				listMessages: func(t *testing.T, limit int, cursor MessageCursor) ([]Message, error) {
					if cursor.ID != msg2.ID {
						t.Errorf("cursor ID=%q, want %q", cursor.ID, msg2.ID)
					}
					return []Message{msg1}, nil
				},
			},
			wantStatus: 200,
			wantBody: `{
				"messages": [
					{
						"id": "1",
						"text": "Hello",
						"user_id": "testuser",
						"created_at": "Mon, 01 Jan 2024 00:00:00 UTC",
						"reaction_count": 0,
						"reactions": []
					}
				],
				"cursor": "` + MessageCursor{CreatedAt: msg1.CreatedAt, ID: msg1.ID}.String() + `"
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.db != nil {
				tt.db.T = t
			}
			if tt.cache != nil {
				tt.cache.T = t
			}
			api := &API{
				DB:     tt.db,
				Cache:  tt.cache,
				Logger: slogt.New(t),
			}

			srv := httptest.NewServer(api)
			defer srv.Close()

			path := tt.url
			if path == "" {
				path = "/messages"
			}
			req, _ := http.NewRequest("GET", srv.URL+path, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			checkStatus(t, resp.StatusCode, tt.wantStatus)
			checkBody(t, resp, tt.wantBody)
		})
	}
}

func fullCachePage() []Message {
	out := make([]Message, messagePageSize)
	for i := 0; i < messagePageSize; i++ {
		out[i] = Message{
			ID:        string(rune('a' + i)),
			Text:      "Hello",
			UserID:    "testuser",
			CreatedAt: time.Date(2024, 1, 10-i, 0, 0, 0, 0, time.UTC),
		}
	}
	return out
}

func fullCachePageBody() string {
	msgs := fullCachePage()
	var b strings.Builder
	b.WriteString(`{"messages":[`)
	for i, msg := range msgs {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"id":"` + msg.ID + `","text":"Hello","user_id":"testuser","created_at":"` + msg.CreatedAt.Format(time.RFC1123) + `","reaction_count":0,"reactions":[]}`)
	}
	b.WriteString(`],"cursor":"` + cursorFromMessages(msgs) + `"}`)
	return b.String()
}

func TestAPI_createMessage(t *testing.T) {
	tests := []struct {
		name        string
		cache       *testcache
		db          *testdb
		req         string
		wantStatus  int
		wantBody    string
		containsLog string
	}{
		{
			name:       "InvalidJSON",
			req:        `not json`,
			wantStatus: 400,
			wantBody: `{
				"error": "Could not decode request body"
			}`,
		},
		{
			name: "DBError",
			req: `{
				"text": "hello",
				"user_id": "test"
			}`,
			db: &testdb{
				insertMessage: func(t *testing.T, msg Message) (Message, error) {
					return Message{}, errors.New("something went wrong")
				},
			},
			wantStatus: 500,
			wantBody: `{
				"error": "Could not insert message"
			}`,
		},
		{
			name: "CacheError",
			req: `{
				"text": "hello",
				"user_id": "test"
			}`,
			cache: &testcache{
				insertMessage: func(t *testing.T, msg Message) error {
					return errors.New("something went wrong")
				},
			},
			db: &testdb{
				insertMessage: func(t *testing.T, msg Message) (Message, error) {
					return Message{
						ID:        "1",
						Text:      msg.Text,
						UserID:    msg.UserID,
						CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					}, nil
				},
			},
			wantStatus: 201,
			wantBody: `{
				"id": "1",
				"text": "hello",
				"user_id": "test",
				"created_at": "Mon, 01 Jan 2024 00:00:00 UTC"
			}`,
			containsLog: "Could not cache message",
		},
		{
			name: "OK",
			req: `{
				"text": "hello",
				"user_id": "test"
			}`,
			db: &testdb{
				insertMessage: func(t *testing.T, msg Message) (Message, error) {
					if msg.UserID != "test" {
						t.Errorf("Got UserID %q, want test", msg.UserID)
					}
					if msg.Text != "hello" {
						t.Errorf("Got Text %q, want test", msg.Text)
					}
					return Message{
						ID:        "1",
						Text:      msg.Text,
						UserID:    msg.UserID,
						CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					}, nil
				},
			},
			cache: &testcache{
				insertMessage: func(t *testing.T, msg Message) error {
					if msg.UserID != "test" {
						t.Errorf("Got UserID %q, want test", msg.UserID)
					}
					if msg.Text != "hello" {
						t.Errorf("Got Text %q, want test", msg.Text)
					}
					return nil
				},
			},
			wantStatus: 201,
			wantBody: `{
				"id": "1",
				"text": "hello",
				"user_id": "test",
				"created_at": "Mon, 01 Jan 2024 00:00:00 UTC"
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			if tt.db != nil {
				tt.db.T = t
			}
			if tt.cache != nil {
				tt.cache.T = t
			}
			api := &API{
				DB:     tt.db,
				Cache:  tt.cache,
				Logger: slog.New(slog.NewTextHandler(buf, nil)),
			}

			srv := httptest.NewServer(api)
			defer srv.Close()

			req, _ := http.NewRequest("POST", srv.URL+"/messages", strings.NewReader(tt.req))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			checkStatus(t, resp.StatusCode, tt.wantStatus)
			checkBody(t, resp, tt.wantBody)
			checkLog(t, buf, tt.containsLog)
		})
	}
}

func TestAPI_createReaction(t *testing.T) {
	tests := []struct {
		name       string
		db         *testdb
		messageID  string
		req        string
		wantStatus int
		wantBody   string
	}{
		{
			name: "OK",
			req: `{
				"type": "thumbs_up",
				"user_id": "test"
			}`,
			messageID: "12345",
			db: &testdb{
				insertReaction: func(t *testing.T, reaction Reaction) (Reaction, error) {
					if reaction.UserID != "test" {
						t.Errorf("Got UserID %q, want test", reaction.UserID)
					}
					if reaction.Type != "thumbs_up" {
						t.Errorf("Got Type %q, want thumbs_up", reaction.Type)
					}
					if reaction.Score != 1 {
						t.Errorf("Got Score %d, want 1", reaction.Score)
					}
					return Reaction{
						ID:        "1",
						MessageID: "12345",
						Score:     1,
						Type:      reaction.Type,
						UserID:    reaction.UserID,
						CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					}, nil
				},
			},
			wantStatus: 201,
			wantBody: `{
				"id": "1",
				"message_id": "12345",
				"type": "thumbs_up",
				"score": 1,
				"user_id": "test",
				"created_at": "Mon, 01 Jan 2024 00:00:00 UTC"
			}`,
		},
		{
			name:      "MissingUserID",
			messageID: "12345",
			req:       `{"type":"like"}`,
			wantStatus: 400,
			wantBody: `{
				"error": "user_id is required"
			}`,
		},
		{
			name:      "MessageNotFound",
			messageID: "12345",
			req:       `{"type":"like","user_id":"test"}`,
			db: &testdb{
				messageExists: func(t *testing.T, messageID string) (bool, error) {
					return false, nil
				},
			},
			wantStatus: 404,
			wantBody: `{
				"error": "Message not found"
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.db == nil {
				tt.db = &testdb{}
			}
			tt.db.T = t
			api := &API{
				DB:     tt.db,
				Logger: slogt.New(t),
			}

			srv := httptest.NewServer(api)
			defer srv.Close()

			req, _ := http.NewRequest("POST", srv.URL+"/messages/"+tt.messageID+"/reactions", strings.NewReader(tt.req))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			checkStatus(t, resp.StatusCode, tt.wantStatus)
			checkBody(t, resp, tt.wantBody)
		})
	}
}

type testdb struct {
	T              *testing.T
	listMessages   func(t *testing.T, limit int, cursor MessageCursor) ([]Message, error)
	insertMessage  func(t *testing.T, msg Message) (Message, error)
	insertReaction func(t *testing.T, reaction Reaction) (Reaction, error)
	listReactions  func(t *testing.T, messageIDs []string) ([]Reaction, error)
	messageExists  func(t *testing.T, messageID string) (bool, error)
}

func (db *testdb) ListMessages(_ context.Context, limit int, cursor MessageCursor) ([]Message, error) {
	return db.listMessages(db.T, limit, cursor)
}

func (db *testdb) InsertMessage(_ context.Context, msg Message) (Message, error) {
	return db.insertMessage(db.T, msg)
}

func (db *testdb) InsertReaction(_ context.Context, reaction Reaction) (Reaction, error) {
	return db.insertReaction(db.T, reaction)
}

func (db *testdb) MessageExists(_ context.Context, messageID string) (bool, error) {
	if db.messageExists == nil {
		return true, nil
	}
	return db.messageExists(db.T, messageID)
}

func (db *testdb) ListReactions(_ context.Context, messageIDs []string) ([]Reaction, error) {
	if db.listReactions == nil {
		return nil, nil
	}
	return db.listReactions(db.T, messageIDs)
}

type testcache struct {
	T               *testing.T
	listMessages    func(t *testing.T) ([]Message, error)
	insertMessage   func(t *testing.T, msg Message) error
	replaceMessages func(t *testing.T, msgs []Message) error
}

func (c *testcache) ListMessages(_ context.Context) ([]Message, error) {
	return c.listMessages(c.T)
}

func (c *testcache) InsertMessage(_ context.Context, msg Message) error {
	return c.insertMessage(c.T, msg)
}

func (c *testcache) ReplaceMessages(_ context.Context, msgs []Message) error {
	if c.replaceMessages == nil {
		return nil
	}
	return c.replaceMessages(c.T, msgs)
}

func checkStatus(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("Got HTTP status %d, want %d", got, want)
	}
}

func checkBody(t *testing.T, resp *http.Response, want string) {
	t.Helper()
	gotBody := normalizeJSON(t, resp.Body)
	wantBody := normalizeJSON(t, bytes.NewReader([]byte(want)))
	if gotBody != wantBody {
		t.Errorf("Body does not match\nGot\n  %s\n\nWant\n  %s", gotBody, wantBody)
	}
}

func checkLog(t *testing.T, buffer *bytes.Buffer, want string) {
	t.Helper()

	if s := buffer.String(); want != "" && !strings.Contains(s, want) {
		t.Errorf("Log does not contain  %s\n", want)
	}
}

func normalizeJSON(t *testing.T, r io.Reader) string {
	t.Helper()
	var buf bytes.Buffer
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("Could not read JSON: %v", err)
	}
	if err := json.Indent(&buf, b, "  ", "  "); err != nil {
		t.Fatalf("Could not indent JSON: %v", err)
	}
	return strings.TrimSpace(buf.String())
}
