# Agent Experience Learning Engine

Independent experience-learning middleware for Agents.

> Trace → Experience → Selection → Usage → Outcome → Feedback → Reward → Utility → Better Future Selection

Not chat Memory. Not generic RAG. Not an Agent framework.

## Status

**Phase 5** — Experience selector + context builder (`POST /api/v1/context`).

See [docs/architecture.md](docs/architecture.md), [docs/roadmap.md](docs/roadmap.md), [docs/domain-model.md](docs/domain-model.md).

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

## License

Apache-2.0 (intended; LICENSE file added when publishing).
