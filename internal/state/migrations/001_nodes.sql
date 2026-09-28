CREATE TABLE nodes (
    id TEXT PRIMARY KEY,
    hostname TEXT NOT NULL,
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL
);

CREATE INDEX idx_nodes_last_seen_at ON nodes(last_seen_at);
