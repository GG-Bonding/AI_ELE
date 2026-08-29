# Agent Experience Learning Engine

Independent experience-learning middleware for Agents.

> Trace → Experience → Selection → Usage → Outcome → Feedback → Reward → Utility → Better Future Selection

Not chat Memory. Not generic RAG. Not an Agent framework.

## Status

**Phase 0** — project bootstrap (config, logger, Postgres, health checks, Docker Compose).

See [docs/architecture.md](docs/architecture.md), [docs/roadmap.md](docs/roadmap.md), [docs/domain-model.md](docs/domain-model.md).

## Quick start

```bash
# Start Postgres (pgvector)
docker compose -f deployments/docker-compose.yml up -d postgres

# Run migrations + server (or use compose `server` service)
cp configs/config.example.yaml configs/config.yaml
make run

curl -s localhost:8080/healthz
curl -s localhost:8080/readyz
```

## Development

```bash
make test
make lint
make migrate
```

## License

Apache-2.0 (intended; LICENSE file added when publishing).
