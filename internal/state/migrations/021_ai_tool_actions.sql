CREATE TABLE ai_tool_actions (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tool_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    sensitivity TEXT NOT NULL CHECK (sensitivity IN ('change', 'sensitive')),
    input_json TEXT NOT NULL DEFAULT '{}',
    status TEXT NOT NULL CHECK (status IN ('pending', 'executing', 'executed', 'rejected', 'failed')),
    result_json TEXT,
    error_code TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_ai_tool_actions_conversation_created
    ON ai_tool_actions(conversation_id, created_at DESC);

CREATE INDEX idx_ai_tool_actions_user_status
    ON ai_tool_actions(user_id, status, updated_at DESC);
