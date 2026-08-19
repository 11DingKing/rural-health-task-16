# 乡村医生健康守护与基层医疗协同平台

A production-grade Go backend service for managing exoskeleton rehabilitation robot certification submissions, dispatch to testing agencies, rule-based standard evaluation, and result consistency enforcement.

## System Architecture

```
┌─────────────────────────────────────────────────────┐
│                   Enterprise (HTTP)                  │
└──────────────┬──────────────────────────┬───────────┘
               │                          │
    ┌──────────▼──────────┐   ┌─────────▼─────────┐
    │  certgateway (HTTP)  │   │  certupstream      │
    │  Port 52661          │   │  (simulator)       │
    │  - REST API          │   │  Port 52662/52663   │
    │  - Gateway manager   │◄─►│  - Test endpoints   │
    │  - Circuit breaker   │   └───────────────────┘
    │  - Scheduler         │
    └──────────┬───────────┘
               │
    ┌──────────▼──────────┐    ┌────────────────────┐
    │  KV Store (WAL)     │    │  SQLite (audit)    │
    │  - Submissions      │    │  - Audit logs      │
    │  - Rules/Versions   │    │  - Schema version   │
    │  - Dispatch tasks   │    └────────────────────┘
    │  - Results          │
    │  - Summaries        │
    │  - Callbacks        │
    │  - Idempotency      │
    │  - Dead letters     │
    └─────────────────────┘
```

## Project Structure

```
cmd/
  certgateway/     Main HTTP gateway service
  certupstream/    Upstream testing agency simulator
  certadmin/       Admin CLI tool
internal/
  domain/          Domain models and state machines
  persistence/     Self-implemented file-based KV store (WAL, indexes, recovery)
  repository/      Repository layer (uses KV store)
  ruleengine/      Rule expression DSL engine (lexer, parser, AST, evaluator)
  rulemgmt/        Rule management service (trials, versions, approval)
  submission/      Submission service (idempotency, charge, state machine)
  dispatch/        Dispatch orchestration (agency selection, failover)
  consistency/     Derived view consistency (summary recompute, freeze, repair)
  callback/        Callback notification (HMAC signing, retry, dedup)
  gateway/         Upstream gateway manager (circuit breaker, tracing)
  audit/           Audit service (SQLite-backed)
  scheduler/       Background task scheduler (ticker-driven, graceful stop)
  config/          Configuration parsing and validation
  httpapi/         HTTP handlers, router, middleware
  middleware/       HTTP middleware (logging, recovery, CORS, request ID)
  logging/         Structured logging (zerolog)
  clock/           Injectable clock abstraction
  errorsx/         Error chain utilities
  models/          HTTP DTOs
configs/           YAML configuration
migrations/        Schema reference
web/               React 18 + TypeScript + Ant Design frontend
```

## Data Model

### KV Store Records (6+ interrelated types)

| Bucket | Key | Secondary Indexes |
|--------|-----|-------------------|
| submissions | submission ID | model_batch, enterprise, status |
| standards | standard code | category |
| rules | rule ID | standard, status |
| rule_versions | version ID | rule, standard, status |
| rule_trials | trial ID | rule, status |
| agencies | agency ID | code, standard |
| dispatch_tasks | task ID | submission, agency, status, model_batch |
| test_results | result ID | dispatch, submission, model, standard |
| summaries | summary ID | submission, model, status |
| callbacks | callback ID | submission, status |
| idempotency | idempotency key | business |
| dead_letters | letter ID | entity, resolved |

### SQLite Tables
- `audit_logs` — append-only audit trail with entity, actor, action indexes
- `schema_version` — data version marker

## Key Features

- **Idempotent submissions**: Same model+batch returns the original result, charges zero
- **Rule DSL engine**: Lexer → parser → type checker → evaluator with trial sandbox
- **Rule versioning**: Rules versioned by effective date, historical conclusions preserved
- **Multi-upstream gateway**: Circuit breaker, timeout backoff, automatic failover
- **Derived consistency**: Summary recomputed on detail change, frozen if divergent
- **Callback retry**: HMAC-signed notifications with exponential backoff
- **Background scheduler**: Dispatch retry, callback delivery, consistency check
- **Crash recovery**: WAL replay on restart, incomplete transactions discarded

## Getting Started

### Prerequisites
- Go 1.26
- Node.js 20+ (for frontend)

### Build and Run

```bash
# Build all Go binaries
go build ./...

# Run the gateway service (port 52661)
go run ./cmd/certgateway -config configs/config.yaml

# In another terminal, run an upstream simulator
go run ./cmd/certupstream -addr :52662 -name "Primary Testing Center"

# Run a second upstream for failover
go run ./cmd/certupstream -addr :52663 -name "Backup Testing Center"

# Use the admin CLI
go run ./cmd/certadmin submissions
```

### Frontend

```bash
cd web
npm install
npm run type-check   # TypeScript check
npm run build       # Build to web/dist/
npm run dev         # Dev server with hot reload
```

The Go server serves the built frontend from `web/dist/`.

### Testing

```bash
go test -timeout=300s -count=1 ./...
go test -race -timeout=420s -count=1 ./...
```

### Configuration

Configuration is loaded from `configs/config.yaml` and can be overridden by environment variables:

| Config | Env Var | Default |
|--------|---------|---------|
| http_addr | CERT_HTTP_ADDR | :52661 |
| data_dir | CERT_DATA_DIR | ./data |
| signing_key | CERT_SIGNING_KEY | change-this-in-production |
| log level | LOG_LEVEL | info |
| log format | LOG_FORMAT | json |

### Example API Requests

```bash
# Create a submission
curl -X POST http://localhost:52661/api/v1/submissions \
  -H "Content-Type: application/json" \
  -d '{
    "enterprise_id": "ent_001",
    "enterprise_name": "Rehab Co",
    "model_no": "EXO-R1",
    "model_name": "Rehab Exoskeleton V1",
    "category": "exoskeleton",
    "risk_level": 2,
    "standard_codes": ["GB-9706"],
    "batch_no": "BATCH-001"
  }'

# List submissions
curl "http://localhost:52661/api/v1/submissions?page=1&page_size=10"

# Dispatch to agencies
curl -X POST http://localhost:52661/api/v1/submissions/sub_xxx/dispatch

# Create a rule
curl -X POST http://localhost:52661/api/v1/rules \
  -H "Content-Type: application/json" \
  -d '{
    "standard_code": "GB-9706",
    "name": "High Risk Rule",
    "expression": "submission.risk_level >= 2",
    "priority": 1
  }'

# Run rule trial
curl -X POST http://localhost:52661/api/v1/rules/rule_xxx/trial \
  -H "Content-Type: application/json" \
  -d '{"context_json": "{\"submission\": {\"risk_level\": 3}}"}'

# Health checks
curl http://localhost:52661/healthz
curl http://localhost:52661/readyz
```

## Main API Routes

| Method | Path | Description |
|--------|------|-------------|
| POST | /api/v1/submissions | Create submission |
| GET | /api/v1/submissions | List submissions (paginated) |
| GET | /api/v1/submissions/:id | Get submission |
| PUT | /api/v1/submissions/:id | Update submission |
| DELETE | /api/v1/submissions/:id/cancel | Cancel submission |
| POST | /api/v1/submissions/:id/dispatch | Dispatch to agencies |
| POST | /api/v1/submissions/batch-dispatch | Batch dispatch |
| GET | /api/v1/submissions/export | Export reconciliation |
| GET | /api/v1/dispatches | List dispatch tasks |
| POST | /api/v1/dispatches/:id/retry | Retry failed dispatch |
| POST | /api/v1/results | Submit test result |
| GET | /api/v1/summaries/:submissionID | Get summary |
| POST | /api/v1/summaries/:submissionID/check | Check consistency |
| POST | /api/v1/rules | Create rule |
| GET | /api/v1/rules | List rules |
| POST | /api/v1/rules/:id/trial | Run trial calculation |
| POST | /api/v1/rules/:id/trials/:trialID/approve | Approve trial |
| GET | /api/v1/audit | List audit logs |
| GET | /api/v1/gateway/status | Gateway status |
| GET | /api/v1/backlog | Backlog stats |
| GET | /healthz | Liveness |
| GET | /readyz | Readiness |
