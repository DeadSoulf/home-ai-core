CREATE TABLE jobs (
    id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL,
    type TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN (
        'queued', 'running', 'cancel_requested', 'cancelled', 'succeeded', 'failed'
    )),
    progress INTEGER NOT NULL DEFAULT 0 CHECK (progress >= 0 AND progress <= 10000),
    message TEXT NOT NULL DEFAULT '',
    actor_type TEXT NOT NULL,
    actor_id TEXT,
    request_id TEXT,
    correlation_id TEXT,
    input_json TEXT NOT NULL DEFAULT '{}',
    result_json TEXT,
    error_code TEXT,
    error_message TEXT,
    created_at TEXT NOT NULL,
    started_at TEXT,
    completed_at TEXT,
    cancel_requested_at TEXT
);

CREATE INDEX idx_jobs_status_created_at ON jobs(status, created_at);
CREATE INDEX idx_jobs_node_created_at ON jobs(node_id, created_at);

CREATE TABLE event_log (
    cursor INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL UNIQUE,
    node_id TEXT NOT NULL,
    component TEXT NOT NULL,
    type TEXT NOT NULL,
    actor_type TEXT,
    actor_id TEXT,
    request_id TEXT,
    correlation_id TEXT,
    job_id TEXT REFERENCES jobs(id) ON DELETE SET NULL,
    occurred_at TEXT NOT NULL,
    data_json TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_event_log_type_cursor ON event_log(type, cursor);
CREATE INDEX idx_event_log_job_cursor ON event_log(job_id, cursor);

INSERT INTO permissions(name, description) VALUES
    ('jobs.read', 'Read persistent jobs and progress'),
    ('jobs.cancel', 'Request cancellation of jobs');

INSERT INTO role_permissions(role_id, permission_name)
SELECT 'role_owner', name
FROM permissions
WHERE name IN ('jobs.read', 'jobs.cancel');

-- A process crash must not leave jobs permanently marked as running.
UPDATE jobs SET status = 'queued', started_at = NULL
WHERE status IN ('running', 'cancel_requested');
