-- Audit log schema (SQLite)
-- The main business data is stored in the self-implemented KV store.
-- This SQLite database stores only audit logs and upstream traces,
-- which benefit from SQL querying capabilities.

CREATE TABLE IF NOT EXISTS audit_logs (
    id TEXT PRIMARY KEY,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    action TEXT NOT NULL,
    actor TEXT NOT NULL,
    detail TEXT,
    occurred_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_entity ON audit_logs(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_logs(actor);
CREATE INDEX IF NOT EXISTS idx_audit_time ON audit_logs(occurred_at);

CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY
);

INSERT OR IGNORE INTO schema_version (version) VALUES (1);

-- KV Store buckets (managed by the application, documented here for reference):
-- submissions       - Certification submissions (key: submission ID)
-- standards         - Certification standards (key: standard code)
-- rules             - Rule definitions (key: rule ID)
-- rule_versions     - Versioned rule snapshots (key: version ID)
-- rule_trials       - Rule trial calculations (key: trial ID)
-- agencies          - Testing agencies (key: agency ID)
-- dispatch_tasks    - Tasks dispatched to agencies (key: task ID)
-- test_results      - Detail test results (key: result ID)
-- summaries         - Model summary conclusions (key: summary ID)
-- callbacks         - Callback notifications (key: callback ID)
-- idempotency       - Idempotency records (key: idempotency key)
-- dead_letters      - Permanent failure records (key: letter ID)
--
-- Secondary indexes:
-- submissions: model_batch (model_no:batch_no), enterprise, status
-- rules: standard, status
-- rule_versions: rule, standard, status
-- rule_trials: rule, status
-- agencies: code, standard
-- dispatch_tasks: submission, agency, status, model_batch
-- test_results: dispatch, submission, model, standard
-- summaries: submission, model, status
-- callbacks: submission, status
-- idempotency: business
-- dead_letters: entity, resolved
