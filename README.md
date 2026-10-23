# Stream Message API

Homework for Stream's backend interview. The starter was a message REST API with an unimplemented reaction endpoint. This repo completes pagination, reactions, validation, and schema for high read/write load.

## Prerequisites

- Go 1.22 or newer ([install](https://go.dev/doc/install))
- Docker (PostgreSQL + Redis)

## Setup

```bash
docker compose up -d
go run ./cmd/api
```

The server listens on `localhost:8080`. Flags (`go run ./cmd/api -h`) default to the compose services: Postgres `localhost:5432`, Redis `localhost:6379`.

`postgres/schema.sql` is applied only when the Postgres volume is first created. After changing the schema:

```bash
docker compose down -v && docker compose up -d
```

If a local Postgres already binds `5432`, stop it or the app will connect to the wrong instance.

## What was implemented

| Requirement | Approach |
|---|---|
| `reactions` table | UUID PK, FK to `messages` with `ON DELETE CASCADE`, `CHECK (score > 0)` |
| Indexes / uniqueness | `messages.id` PK; `messages(created_at DESC)` for keyset pagination; `UNIQUE (message_id, user_id, type)` (also supports `IN (message_id)` lookups) |
| `POST .../reactions` | Upsert on that unique key; omitted `score` defaults to 1; missing message → 404 |
| Pagination | Pages of 10, newest first; cursor is `(created_at, id)` so equal timestamps do not skip/duplicate |
| List reactions | Each message includes `reaction_count` and `reactions`; loaded from DB in one `IN` query (no reaction cache) |
| Validation | Cursor format; reaction `user_id` / `type` / positive `score` |

### Pagination and cache

- `GET /messages` with no `cursor`: latest page. Redis holds the latest 10 **message shells**. If Redis has fewer than 10 (cold or partial), the API loads the latest 10 from Postgres and **rebuilds** the cache under a mutex so create and replace cannot interleave. Cache hits do not take that lock.
- `GET /messages?cursor=...`: Postgres only, `created_at < t OR (created_at = t AND id < id)`, `ORDER BY created_at DESC, id DESC LIMIT 10`.
- Response `cursor` is the last item encoded as `{RFC3339Nano}|{uuid}`. Empty list → empty cursor.

Create still writes Postgres then Redis (Redis failure is logged; the HTTP create still succeeds).

### Why reactions are not cached

Redis speeds up **which 10 messages are latest**. Reactions are always read from Postgres after that. A 10-id `IN` query on the unique index is cheap; caching reactions would require invalidation on every score upsert, with no perfect consistency. If reaction payloads later dominate latency, a per-message snapshot plus delete-on-write would be the next step—not patching scores inside Redis.

## API

### `GET /messages`

Query: optional `cursor`.

```bash
curl -s 'localhost:8080/messages'
curl -s 'localhost:8080/messages?cursor=2024-01-02T00:00:00Z|7c6d956b-58d6-4ac3-9984-f341346edc37'
```

```json
{
  "messages": [
    {
      "id": "...",
      "text": "world!",
      "user_id": "testuser",
      "created_at": "Fri, 23 Oct 2026 13:00:00 UTC",
      "reaction_count": 1,
      "reactions": [
        {
          "id": "...",
          "message_id": "...",
          "type": "like",
          "score": 1,
          "user_id": "testuser",
          "created_at": "Fri, 23 Oct 2026 13:00:01 UTC"
        }
      ]
    }
  ],
  "cursor": "2026-10-23T13:00:00Z|..."
}
```

Invalid cursor → `400`.

### `POST /messages`

```bash
curl -s -X POST localhost:8080/messages \
  -H 'Content-Type: application/json' \
  -d '{"text":"hello","user_id":"testuser"}'
```

`201` with `id`, `text`, `user_id`, `created_at`.

### `POST /messages/{messageID}/reactions`

Same user + message + type updates `score` and keeps the original reaction id / `created_at`. HTTP status is `201` for both insert and update.

```bash
curl -s -X POST localhost:8080/messages/$MESSAGE_ID/reactions \
  -H 'Content-Type: application/json' \
  -d '{"type":"like","user_id":"testuser"}'

curl -s -X POST localhost:8080/messages/$MESSAGE_ID/reactions \
  -H 'Content-Type: application/json' \
  -d '{"type":"like","score":10,"user_id":"testuser"}'
```

| Case | Status |
|---|---|
| Missing `user_id` / `type`, `score < 1` | 400 |
| Unknown `messageID` | 404 |

## Tests

```bash
go test ./...
go test -tags=integration ./...   # wipes compose DB data
hurl --test e2e.hurl              # API must be running; empty DB
```

Postman: import `postman_collection.json` (base URL `http://localhost:8080`). Run **Happy path** in order so `message_id` / `cursor` are captured.

## Layout

- `cmd/api` — process, Postgres/Redis connect
- `api` — HTTP, pagination, validation, cache orchestration
- `postgres` — SQL via bun
- `redis` — latest-10 message cache (ZSET index + per-message hashes)
