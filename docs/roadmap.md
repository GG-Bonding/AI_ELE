# Roadmap

**Current:** V2 complete on `main` (V2-0…V2-10). Ops hardening V2-0 landed with the intelligence track.

V1 Complete. V1.1 (stale PROCESSING recovery, Postgres integration CI) remains tracked as **V2-0** ops hardening and may run in parallel; it does not reopen the V1 learning-loop contract.

Development is strictly phased within V2. Preferred order: V2-1 → V2-2 → V2-3 → V2-4. See [docs/v2.md](v2.md).

## Phase 0 — Bootstrap ✅

- Go module, config, structured logger
- PostgreSQL + migration runner
- `GET /healthz`, `GET /readyz`
- Docker Compose
- `make test` / `make lint` / basic unit tests

**Out of scope:** Episode, Experience, LLM providers

## Phase 1 — Episode Lifecycle ✅

- Episode / Attempt / Outcome models + repositories
- REST: create episode, add attempt, complete with outcome
- Tenant isolation tests

## Phase 2 — Experience Extraction ✅

- `ExperienceExtractor` with LLM provider interface
- JSON schema validation + one retry
- Mock LLM unit tests → `[]ExperienceCandidate`

## Phase 3 — Store + Retrieval ✅

- Experience repository (pgvector embeddings)
- Metadata filter + TopK vector similarity
- OpenAI-compatible embedding provider

## Phase 4 — Two-Phase Retrieval ✅

- Phase 1: semantic candidate set
- Phase 2: utility-aware ranking  
  `FinalScore = Similarity × Utility × Confidence × Freshness × ScopeMatch`
- Deterministic ranking tests

## Phase 5 — Selector + Context ✅

- KEEP / ABSTRACT / IGNORE / BLOCK
- Context Builder (max experiences / tokens)
- Untrusted-data framing in context payload

## Phase 6 — Feedback Pipeline ✅

- Feedback model, normalizer (reward ∈ [-1,1]), source weights
- Persist raw feedback; compute weighted reward

## Phase 7 — Utility Learning ✅

- ExperienceUsage tracking
- Attribution strategy interface (V1: score-proportional)
- Beta-style utility update
- E2E: success ↑ utility, failure ↓ utility, ranking changes

## Phase 8 — Supersession + Decay ✅

- Supersede API; deprecated excluded from retrieval
- Freshness decay (type-specific λ; age from LastUsedAt)
- Evolution interface: Supersede + Decay only

## Phase 9 — SDKs ✅

- Go SDK + Python SDK
- Examples for episode → context → feedback loop

## Phase 10 — Evaluation ✅

Four arms: Baseline / Raw Retrieval / Utility Retrieval / Utility + Learning  
Metrics: success rate, precision, utilization, avg utility, negative transfer, tokens, latency  
Core E2E: Jira project-key learning loop (three tasks)

## V1 Definition of Done ✅

**Met on `main`.** Ops follow-ups tracked as V2-0 / V1.1.

See [docs/evaluation.md](evaluation.md). Core proof remains the Jira project-key learning loop.

## V2 — Experience Intelligence

| Phase | Status |
| --- | --- |
| V2-0 V1.1 hardening (stale PROCESSING + Postgres CI) | ✅ landed |
| V2-1 Action Tracking + ExperienceActionLink | ✅ in progress / landed |
| V2-2 Feedback Targeting | ✅ landed |
| V2-3 Attribution v2 | ✅ landed |
| V2-4 Semantic Dedup | ✅ landed |
| V2-5 Conflict Detection | ✅ landed |
| V2-6 Supersession (intelligence) | ✅ landed |
| V2-7 Generalization → Pattern | ✅ landed |
| V2-8 Pattern Learning | ✅ landed |
| V2-9 Skill Candidate | ✅ landed |
| V2-10 Sequential benchmark | ✅ landed |

## V2.1 — Close the Intelligence Loop

| Phase | Status |
| --- | --- |
| V2.1-1 ACTION_FIELD field attribution (`affected_fields`) | ✅ landed |
| V2.1-2 Pattern into retrieval/context | ✅ landed |
| V2.1-3 Pattern usage / suppress | ✅ landed |
| V2.1-4 PatternLearningEvent exactly-once | ✅ landed |
| V2.1-5 Automatic generalization | ✅ landed |

## V2.2 — Production Intelligence

| Phase | Status |
| --- | --- |
| V2.2-1 Pattern embedding semantic retrieval | ✅ landed |
| V2.2-2 Context provenance → Action | ✅ landed |
| V2.2-3 Persistent Evolution jobs | ✅ landed |
| V2.2-4 Structured semantic Judge | ✅ landed |
| V2.2-5 LLM Pattern Generalizer | ✅ landed |
| V2.2-6 V2 SDK | ✅ landed |
| V2.2-7 Learned recovery benchmark | ✅ landed |

Full DoD and architecture: [docs/v2.md](v2.md).
