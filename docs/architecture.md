# Architecture — Agent Experience Learning Engine

## Positioning

This system is an **experience learning middleware**, independent of any specific Agent / Claw framework.

It is **not**:

- a chat Memory store
- a generic RAG / vector search service
- an Agent framework (no planning, tool execution, or workflow loop)

It **is** a runtime learning loop:

```text
Trace → Experience → Selection → Usage → Outcome → Feedback → Reward → Utility → Better Future Selection
```

## Responsibility Boundary

| Owner | Responsibilities |
| --- | --- |
| Agent | Planning, reasoning, tool calling, execution |
| Experience Engine | Observe, extract, evaluate, store, retrieve, select, feedback, reward, learn, evolve |

## Learning Loop

```text
Agent
  │ Task / Tool Call / Tool Result
  ▼
Observer → Episode Builder → Experience Extractor → Experience Evaluator → Experience Store
  ▲                                                                                    │
  │                                                                                    ▼
Context Builder ← Experience Selector ← Retriever (Two-Phase) ← Experience Store
  │
  ▼
Agent → Feedback Collector → Normalizer → Attribution → Reward → Utility Updater → Store
```

## Core Ideas (inspired, not copied)

1. **Episode → Attempt → Outcome → Pattern** — durable task traces and verified outcomes
2. **Experience Utility** — retrieve by similarity, rank by utility; update utility from environment feedback without training LLM weights
3. **Independent Memory Service** — HTTP API + SDKs + provider/storage adapters

## Runtime Topology (V1)

```text
┌─────────────────┐     REST /api/v1      ┌──────────────────────────┐
│ Agent / SDK     │ ───────────────────▶  │ Experience Engine (Go)   │
└─────────────────┘                       │  HTTP + domain packages  │
                                          └────────────┬─────────────┘
                                                       │
                                          ┌────────────▼─────────────┐
                                          │ PostgreSQL 17 + pgvector │
                                          └──────────────────────────┘
                                                       │
                                          ┌────────────▼─────────────┐
                                          │ LLM / Embedding providers│
                                          │ (OpenAI-compatible)      │
                                          └──────────────────────────┘
```

## Package Layout

```text
cmd/server          — process entrypoint
api/http            — REST handlers, middleware
internal/<domain>   — episode, experience, retrieval, feedback, …
storage/postgres    — repositories (no business logic)
sdk/{go,python}     — thin clients
migrations          — SQL schema
configs             — default config
deployments         — docker-compose
```

Rules:

- One package, one responsibility
- Repositories contain persistence only
- Providers are interfaces; V1 ships OpenAI-compatible implementations
- Fail fast; wrap errors with context
- Experience content is **untrusted reference data**, never system instructions

## Multi-Tenancy

Every Episode, Experience, Feedback, and Retrieval is scoped by `tenant_id`. Cross-tenant retrieval is forbidden and covered by tests.

## Observability (V1)

- Prometheus metrics: request count/latency, episode/experience/feedback counters, retrieval latency, extract failures, utility updates
- Structured JSON logs with `request_id`, `tenant_id`, `episode_id` (never secrets)

## Explicit Non-Goals (V1)

Neo4j, complex KG, RL model training, LLM fine-tuning, multi-agent orchestration, skill execution, workflow engine, UI dashboard, Kafka, Redis, K8s operator.
