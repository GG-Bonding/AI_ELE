# Domain Model

## Episode

A complete task execution bound to tenant / agent / user.

| Field | Notes |
| --- | --- |
| Status | `RUNNING`, `SUCCESS`, `PARTIAL`, `FAILED`, `CANCELLED` |
| Goal / Input / TaskType | Task description used for later retrieval |

## Attempt

Ordered tries within an Episode (hypothesis, action, tool I/O, errors).

## AgentAction (V2)

Concrete steps the agent took — attribution graph nodes (distinct from Attempt trial logs).

| Type | Meaning |
| --- | --- |
| `TOOL_CALL` | Tool invocation |
| `PLAN` | Planning step |
| `DECISION` | Decision point |
| `ANSWER` | Final/user-facing answer |
| `WORKFLOW_STEP` | Multi-step workflow unit |

## ExperienceActionLink (V2)

`Experience → Action` influence edge (`influence ∈ [0,1]`, optional evidence). Foundation for V2 attribution (not retrieval-score credit sharing).

APIs: `POST/GET .../episodes/{id}/actions`, `POST .../actions/{action_id}/links`, `GET .../action-links`.

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
| Utility | Beta posterior mean `α / (α + β)` — practice quality |
| Confidence | Evidence strength / quality (raised on semantic reinforce; not Utility) |
| Evidence | Trace metadata + `support_episode_ids` (V2-4 multi-episode corroboration) |

Lifecycle: Candidate → Evaluator thresholds → ACTIVE / CANDIDATE / discard → optional Supersede → DEPRECATED.

### ExperienceRelation (V2-5)

Directed edge between experiences: `DUPLICATE` / `SUPPORTS` / `CONFLICTS` / `SUPERSEDES` / `DERIVED_FROM`.

Unresolved `CONFLICTS` → selector fail-closes (BLOCK both sides). Clear Authority gap → `SUPERSEDES` + loser `DEPRECATED` (V2-6).

### Semantic Dedup (V2-4)

Store path: exact fingerprint (within episode) → semantic neighbors (same type/scope) → `DedupJudge`.

`SAME` merges into the existing experience (evidence + confidence↑). Opposing polarity near-neighbors are `CONFLICT` and are not merged (V2-5 deepens this).

## Feedback

External signal about an Episode outcome. Always store raw rows.

### FeedbackTarget (V2)

Optional locator so feedback can say *what* was wrong, not only *that* the episode scored poorly.

| Type | Required fields |
| --- | --- |
| `EPISODE` | (none) |
| `ACTION` | `action_id` |
| `ACTION_FIELD` | `action_id`, `field` |
| `TOOL` | `tool_name` |
| `EXPERIENCE` | `experience_id` |

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


## Attribution (V2)

When Feedback carries a Target:

| Target | Credit rule |
| --- | --- |
| `EPISODE` / nil | V1 fallback: split by retrieval/final score among used experiences |
| `ACTION` / `ACTION_FIELD` | Only experiences linked to that action (by `ExperienceActionLink.influence`) |
| `TOOL` | Only experiences linked to actions with that tool name |
| `EXPERIENCE` | 100% credit to that experience if it was used |

If a precise target has no matching links, learning **fails closed** (no utility update) instead of blaming high-ranked experiences.
