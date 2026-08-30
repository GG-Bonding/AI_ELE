# V1 Hardening Checklist

Do **not** mark V1 Complete until P0 + required P1 items below are done and re-reviewed on `main`.

## P0 (must)

| ID | Item | Status |
| --- | --- | --- |
| V1-01 | Incremental LearningEvent (no aggregate replay) | done |
| V1-02 | Feedback idempotency (`idempotency_key`) | done |
| V1-03 | Reward + confidence paired per feedback | done |
| V1-04 | Trace → Extract → Store E2E (no SeedHelpfulOnly) | done (store-pipeline path; arms still seed for compare) |
| V1-05 | Tool simulator / real Outcome (no keyword success) | done |
| V1-06 | Independent ExperienceEvaluator | done |
| V1-07 | LearningEvent PENDING → APPLIED consistency | done |

## P1 (must for enterprise readiness)

| ID | Item | Status |
| --- | --- | --- |
| V1-08 | Experience Evidence | done |
| V1-09–11 | Scope hard authorization filters | done |
| V1-12 | Remove Latin-only lexical gate | done |
| V1-13 | Rename ABSTRACT → COMPRESS (honest) | done |
| V1-14–15 | Trace sanitizer + untrusted prompt boundary | done |
| V1-16 | Action relevance in attribution | suggested |

## P2 (required for full V1 validation)

| ID | Item | Status |
| --- | --- | --- |
| V1-17 | Split UsageRecency vs Validity | pending |
| V1-18 | Experience dedup | suggested |
| V1-19 | Conflict candidate hints | suggested |
| V1-20–22 | Multi-domain / real-agent eval | pending |
| V1-23 | Restart persistence check | suggested |
| V1-24 | Concurrent utility updates | pending |
| V1-25 | Apache-2.0 LICENSE file | done |

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
