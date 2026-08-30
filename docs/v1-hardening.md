# V1 Hardening Checklist

**Status: V1 Complete** (as of `ac3ee39` + review closure).

Core runtime learning loop is implemented and validated on `main`. Remaining work is tracked under **V1.1** below — do not reopen V1 for those items.

## P0 (must)

| ID | Item | Status |
| --- | --- | --- |
| V1-01 | Incremental LearningEvent (no aggregate replay) | done |
| V1-02 | Feedback idempotency (`idempotency_key`) | done |
| V1-03 | Reward + confidence paired per feedback | done |
| V1-04 | Trace → Extract → Store E2E (no SeedHelpfulOnly) | done (extractor path in jira loop + episodelearn processor) |
| V1-05 | Tool simulator / real Outcome (no keyword success) | done |
| V1-06 | Independent ExperienceEvaluator | done (production feeds Outcome+Evidence) |
| V1-07 | LearningEvent PENDING → APPLIED consistency + FAILED retry | done |

## P1 (must for enterprise readiness)

| ID | Item | Status |
| --- | --- | --- |
| V1-08 | Experience Evidence (`FromAttempts`, AttemptIDs, OutcomeID) | done |
| V1-09–11 | Scope hard authorization filters (fail-closed + search pre-TopK) | done |
| V1-12 | Remove Latin-only lexical gate | done |
| V1-13 | Rename ABSTRACT → COMPRESS (honest) | done |
| V1-14–15 | Trace sanitizer + structured JSON redaction + untrusted prompt boundary | done |
| V1-16 | Action relevance in attribution | suggested |

## P2 (required for full V1 validation)

| ID | Item | Status |
| --- | --- | --- |
| V1-17 | Split UsageRecency vs Validity | done |
| V1-18 | Experience dedup | done |
| V1-19 | Conflict candidate hints | suggested |
| V1-20–22 | Multi-domain / real-agent eval | partial (jira+github simulators) |
| V1-23 | Restart persistence check | done (episode learning jobs persisted in Postgres) |
| V1-24 | Concurrent utility updates | done (optimistic lock + retry) |
| V1-25 | Apache-2.0 LICENSE file | done |
| V1-26 | Episode learning job status (PENDING/APPLIED/FAILED) + retry endpoint | done (Postgres repo + dedup idempotent store) |

## Acceptance spine

```text
unseen task → Episode/Attempts/Outcome
→ Sanitizer → Extractor → Evaluator → Store (+ Evidence)
→ Retrieve (hard scope filter) → Rank → Select → Context
→ Agent + Tool Simulator → real Outcome
→ Feedback (idempotent) → LearningEvent → Utility Δ
→ next task ranking/success improves vs no-memory baseline
```

No `SeedHelpfulOnly`, no keyword-based success, no hand-written Experience mid-test.
Production `CompleteOutcome` passes real Outcome + Evidence via `StoreCandidatesWithOptions`.


## Latest hardening pass (post-review)

Addressed the 6 blockers from the production/E2E consistency review:

1. Production `StoreCandidatesWithOptions` with real Outcome + Evidence (`evaluator.FromAttempts`)
2. Jira learning-loop E2E goes through Extractor + MockLLM (no hand-written candidates)
3. LearningEvent apply is atomic; retries use event-persisted reward/confidence/credit only
4. USER/AGENT/TOOL authorization fail-closed (non-empty scope_key) with SQL pre-TopK filter
5. Structured JSON sanitizer (recursive redact → valid JSON)
6. Episode learning jobs persisted in Postgres; experience store dedup makes retries idempotent


## V1.1 Hardening (next)

| ID | Item | Status |
| --- | --- | --- |
| V1.1-01 | Stale `PROCESSING` recovery (episode learning jobs / in-flight learning events) | planned |
| V1.1-02 | Postgres integration CI (pgvector service + migrate + `go test ./storage/postgres/...`) | planned |

V1.1 is production-ops hardening, not a reopen of the V1 learning-loop contract.
