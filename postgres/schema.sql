-- Add your db schema creation SQL for reactions here

-- Messages
CREATE TABLE IF NOT EXISTS messages (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  message_text TEXT NOT NULL,
  user_id VARCHAR(255) NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages (created_at DESC);

CREATE TABLE IF NOT EXISTS reactions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  message_id uuid NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
  user_id VARCHAR(255) NOT NULL,
  type VARCHAR(32) NOT NULL,
  score INTEGER NOT NULL DEFAULT 1 CHECK (score > 0),
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (message_id, user_id, type)
);
