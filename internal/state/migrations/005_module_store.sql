CREATE TABLE module_signing_keys (
    key_id TEXT PRIMARY KEY,
    algorithm TEXT NOT NULL CHECK (algorithm = 'ed25519'),
    public_key_b64 TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE module_repositories (
    id TEXT PRIMARY KEY,
    index_url TEXT NOT NULL,
    signature_url TEXT NOT NULL,
    key_id TEXT NOT NULL REFERENCES module_signing_keys(key_id) ON DELETE RESTRICT,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    last_index_json TEXT,
    last_index_signature_json TEXT,
    last_refreshed_at TEXT,
    last_error TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_module_repositories_enabled ON module_repositories(enabled);
