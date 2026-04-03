CREATE TABLE jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    public_id VARCHAR(64) NOT NULL UNIQUE,
    token VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    progress INTEGER NOT NULL DEFAULT 0,
    spec_json TEXT NOT NULL,
    warnings_json TEXT NOT NULL,
    error_json TEXT NULL,
    result_json TEXT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

-- public_id: external job identifier exposed to API clients.
-- token: reserved for weak isolation / access control before user auth exists.
-- status: job lifecycle state aggregated from task execution.
-- progress: aggregate 0-100 progress for the whole job.
-- spec_json: normalized job request payload serialized as JSON.
-- warnings_json: non-fatal warnings collected during execution serialized as JSON.
-- error_json: terminal job-level error serialized as JSON.
-- result_json: final output metadata serialized as JSON.

CREATE INDEX idx_jobs_status ON jobs(status);

CREATE TABLE tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id INTEGER NOT NULL,
    task_key VARCHAR(64) NOT NULL,
    type VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    resource_key VARCHAR(64) NOT NULL,
    depends_on_json TEXT NOT NULL,
    attempt INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 1,
    payload_json TEXT NOT NULL,
    output_ref_json TEXT NOT NULL,
    error_json TEXT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

-- job_id: parent job primary key.
-- task_key: stable task identifier inside a job DAG, used by dependencies and review.
-- type: business type that determines executor routing.
-- status: task execution state.
-- resource_key: shared resource pool key used for scheduler-level concurrency limits.
-- depends_on_json: upstream task keys serialized as JSON.
-- attempt: current execution attempt count.
-- max_attempts: maximum allowed attempts for this task.
-- payload_json: task input snapshot serialized as JSON.
-- output_ref_json: references to task outputs serialized as JSON.
-- error_json: latest terminal task error serialized as JSON.

CREATE INDEX idx_tasks_job_id ON tasks(job_id);
CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_job_status ON tasks(job_id, status);
CREATE UNIQUE INDEX idx_tasks_job_key ON tasks(job_id, task_key);
