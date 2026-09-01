# Agent Experience Learning Engine

Independent experience-learning middleware for Agents.

> Trace → Experience → Selection → Usage → Outcome → Feedback → Reward → Utility → Better Future Selection

Not chat Memory. Not generic RAG. Not an Agent framework.

## Status

**V1 Complete** — Core runtime learning loop is implemented and validated.

**V2 DoD met** — Experience Intelligence (Attribution → Conflict → Generalization → Evolution) with sequential benchmark proof.

**V2.1-3 landed:** PatternUsage ledger — patterns in context are recorded; episode feedback can credit Patterns directly.
**V2.1-2 landed:** ACTIVE Patterns enter `POST /api/v1/context` (`patterns` + `experiences`); evidence experiences suppressed by default.
**V2.1-1 landed:** ACTION_FIELD attribution uses `affected_fields` provenance (priority feedback only hits priority experience).
**V2-1 landed:** Action Tracking + ExperienceActionLink.
**V2-2 landed:** Feedback Targeting (action / field / experience / tool).
**V2-3 landed:** Attribution uses FeedbackTarget + ExperienceActionLink (not retrieval-score credit).
**V2-0 landed:** Stale `PROCESSING` recovery for episode learning jobs + Postgres/pgvector integration CI.
**V2-10 landed:** Sequential Benchmark — PATH-like V1 vs V2 comparison; V2 raises success and lowers negative transfer via authority supersession.
**V2-9 landed:** Skill Candidate — Pattern → YAML skill description (`auto_execute: false`; no engine execution).
**V2-8 landed:** Pattern Learning — Patterns carry Beta utility; feedback on member experiences updates Pattern utility (and can promote CANDIDATE → ACTIVE).
**V2-7 landed:** Generalization — ≥3 independent episodes → Pattern + `DERIVED_FROM` (heuristic draft; gates on utility/conflict).
**V2-6 landed:** Authority-based supersession — clear winners DEPRECATED losers; close calls keep CONFLICTS.
**V2-5 landed:** Conflict detection — opposing experiences get `CONFLICTS` relations and are blocked from auto-context.
**V2-4 landed:** Semantic dedup — SAME neighbors reinforce Evidence/Confidence (no duplicate ACTIVE insert; Utility unchanged).

```text
Episode → Trace → Sanitize → Extract → Evidence → Evaluate → Dedup → Idempotent Store
→ Authorized Retrieval → Select → Context → Agent/Simulator → Outcome
→ Feedback → Attribution → LearningEvent → Atomic Utility Update → Better Ranking
```

See [docs/v1-hardening.md](docs/v1-hardening.md) for the closed V1 checklist.
See [docs/v2.md](docs/v2.md) for the V2 plan.

**Also tracked (done as V2-0):**
- stale `PROCESSING` recovery for episode learning jobs (retry + startup sweep)
- Postgres integration CI (pgvector service + `go test ./storage/postgres/...`)

See [docs/architecture.md](docs/architecture.md), [docs/roadmap.md](docs/roadmap.md), [docs/domain-model.md](docs/domain-model.md), [docs/evaluation.md](docs/evaluation.md).

## Quick start

```bash
# Start Postgres (pgvector)
docker compose up -d postgres

# Run migrations + server
cp configs/config.example.yaml configs/config.yaml
make run

curl -s localhost:8080/healthz
curl -s localhost:8080/readyz
```

### Episode lifecycle example

```bash
# Create episode
curl -s localhost:8080/api/v1/episodes -H 'Content-Type: application/json' -d '{
  "tenant_id":"tenant_a","agent_id":"agent_01","user_id":"user_01",
  "task_type":"jira.create_issue","goal":"Create Jira issue","input":"project=Payment"
}'

# Add attempt (replace EPISODE_ID)
curl -s localhost:8080/api/v1/episodes/EPISODE_ID/attempts -H 'Content-Type: application/json' -d '{
  "tenant_id":"tenant_a","action":"create_issue","tool_name":"jira.create_issue",
  "status":"FAILED","error_code":"INVALID_PROJECT_KEY"
}'

# Complete with outcome
curl -s localhost:8080/api/v1/episodes/EPISODE_ID/outcome -H 'Content-Type: application/json' -d '{
  "tenant_id":"tenant_a","status":"SUCCESS","verified":true,"verifier":"tool"
}'
```

## Development

```bash
make test
make lint
make migrate
```

### SDKs

```bash
# Go
go test ./sdk/go/...
EXPERIENCE_ENGINE_URL=http://localhost:8080 go run ./sdk/go/example

# Python
cd sdk/python && pip install -e . && python examples/jira_loop.py
```

### Evaluation

```bash
make test-eval
```

## License

Apache-2.0 — see [LICENSE](LICENSE).
