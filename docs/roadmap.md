# Roadmap

Development is strictly phased. Do not skip ahead. One phase → one suggested commit.

## Phase 0 — Bootstrap ✅

- Go module, config, structured logger
- PostgreSQL + migration runner
- `GET /healthz`, `GET /readyz`
- Docker Compose
- `make test` / `make lint` / basic unit tests

**Out of scope:** Episode, Experience, LLM providers

## Phase 1 — Episode Lifecycle

- Episode / Attempt / Outcome models + repositories
- REST: create episode, add attempt, complete with outcome
- Tenant isolation tests

## Phase 2 — Experience Extraction

- `ExperienceExtractor` with LLM provider interface
- JSON schema validation + one retry
- Mock LLM unit tests → `[]ExperienceCandidate`

## Phase 3 — Store + Retrieval

- Experience repository (pgvector embeddings)
- Metadata filter + TopK vector similarity
- OpenAI-compatible embedding provider

## Phase 4 — Two-Phase Retrieval

- Phase 1: semantic candidate set
- Phase 2: utility-aware ranking  
  `FinalScore = Similarity × Utility × Confidence × Freshness × ScopeMatch`
- Deterministic ranking tests

## Phase 5 — Selector + Context

- KEEP / ABSTRACT / IGNORE / BLOCK
- Context Builder (max experiences / tokens)
- Untrusted-data framing in context payload

## Phase 6 — Feedback Pipeline

- Feedback model, normalizer (reward ∈ [-1,1]), source weights
- Persist raw feedback; compute weighted reward

## Phase 7 — Utility Learning

- ExperienceUsage tracking
- Attribution strategy interface (V1: score-proportional)
- Beta-style utility update
- E2E: success ↑ utility, failure ↓ utility, ranking changes

## Phase 8 — Supersession + Decay

- Supersede API; deprecated excluded from retrieval
- Freshness decay (type-specific λ)
- Evolution interface: Supersede + Decay only

## Phase 9 — SDKs

- Go SDK + Python SDK
- Examples for episode → context → feedback loop

## Phase 10 — Evaluation

Four arms: Baseline / Raw Retrieval / Utility Retrieval / Utility + Learning  
Metrics: success rate, precision, utilization, avg utility, negative transfer, tokens, latency

## V1 Definition of Done

See product prompt §三十八. Core proof is the Jira project-key learning loop (three tasks).
