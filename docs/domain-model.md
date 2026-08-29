# Domain Model

## Episode

A complete task execution bound to tenant / agent / user.

| Field | Notes |
| --- | --- |
| Status | `RUNNING`, `SUCCESS`, `PARTIAL`, `FAILED`, `CANCELLED` |
| Goal / Input / TaskType | Task description used for later retrieval |

## Attempt

Ordered tries within an Episode (hypothesis, action, tool I/O, errors).

## Outcome

Terminal result of an Episode: status, payload, verification source, metrics.

Verifier examples: `tool`, `business_system`, `test`, `ci`, `user`, `human`, `llm_judge`.

## Experience

Long-lived reusable knowledge extracted from episodes.

| Dimension | Values (V1) |
| --- | --- |
| Type | `EPISODIC`, `SEMANTIC`, `PROCEDURAL`, `FAILURE`, `CONSTRAINT`, `PREFERENCE` |
| Scope | at least `TENANT`, `USER`, `AGENT`, `TOOL` (also `GLOBAL`, `TEAM`, `TASK_TYPE` reserved) |
| Status | `CANDIDATE`, `ACTIVE`, `DEPRECATED`, `BLOCKED`, `ARCHIVED` |
| Utility | Beta posterior mean `α / (α + β)` |
| Confidence | Extractor / evaluator quality signal |

Lifecycle: Candidate → Evaluator thresholds → ACTIVE / CANDIDATE / discard → optional Supersede → DEPRECATED.

## Feedback

External signal about an Episode outcome. Always store raw rows; never only the aggregated reward.

Sources & default trust: BUSINESS 1.0, USER_EXPLICIT 1.0, HUMAN_REVIEW 0.95, TOOL 0.85, USER_IMPLICIT 0.60, LLM_JUDGE 0.50.

## ExperienceUsage

Links Episode ↔ Experience with retrieval/selection scores — foundation for attribution.

## Learning Math (V1)

```text
quality = confidence × outcomeWeight × reusability × specificity
weightedReward = Σ(w×c×r) / Σ(w×c)
utility = α / (α + β)   # α,β start at 1; update with signed reward × confidence
FinalScore = Similarity × Utility × Confidence × Freshness × ScopeMatch
Freshness = exp(-λ × ageDays)   # λ by type; age from LastUsedAt (else UpdatedAt)
```

## Supersession

`B` supersedes `A`: `A.status = DEPRECATED`, `B.supersedes_id = A.id`. Default retrieval excludes `DEPRECATED` / `BLOCKED` / `ARCHIVED`.

## Security Note

Experience content is **untrusted**. Context Builder must label it as historical reference data, not instructions.
