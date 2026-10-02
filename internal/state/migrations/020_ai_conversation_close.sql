ALTER TABLE ai_conversations ADD COLUMN closed_at TEXT;

CREATE INDEX idx_ai_conversations_user_closed_updated
    ON ai_conversations(user_id, closed_at, updated_at DESC);
