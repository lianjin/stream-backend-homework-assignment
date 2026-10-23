package api

import (
	"testing"
	"time"
)

func TestParseMessageCursor(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		got, err := ParseMessageCursor("")
		if err != nil {
			t.Fatal(err)
		}
		if !got.IsZero() {
			t.Fatalf("got %+v, want zero", got)
		}
	})

	t.Run("RoundTrip", func(t *testing.T) {
		want := MessageCursor{
			CreatedAt: time.Date(2024, 1, 2, 3, 4, 5, 6, time.UTC),
			ID:        "388d74ea-cc39-4566-860f-0df6068f3330",
		}
		got, err := ParseMessageCursor(want.String())
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != want.ID {
			t.Errorf("ID=%q, want %q", got.ID, want.ID)
		}
		if !got.CreatedAt.Equal(want.CreatedAt) {
			t.Errorf("CreatedAt=%v, want %v", got.CreatedAt, want.CreatedAt)
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		if _, err := ParseMessageCursor("not-a-cursor"); err == nil {
			t.Fatal("expected error")
		}
	})
}
