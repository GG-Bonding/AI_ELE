# Evaluation Harness

Offline, deterministic evaluation of the Experience Learning loop.

## Arms

| Arm | Behavior |
| --- | --- |
| `baseline` | No retrieval / no experience context |
| `raw_retrieval` | Similarity-only ranking |
| `utility_retrieval` | Two-phase utility-aware ranking (static utilities) |
| `utility_learning` | Utility ranking + business feedback → beta utility updates |

## Metrics

- Task Success Rate
- Retrieval Precision
- Experience Utilization
- Average Utility
- Negative Transfer Rate
- Token Cost (approx)
- Latency (ms)

## Core proof

`go test ./internal/eval/ -run TestCompareArms -v`

Expected shape on the Jira project-key scenario:

```text
learning success  > baseline success
learning success  > raw success
utility success   > raw success
raw negative-transfer > 0
learning helpful utility rises across tasks
```

Also run the product E2E learning loop:

```bash
go test ./internal/eval/ -run TestJiraExperienceLearningLoop -v
```

## V2-10 Sequential Benchmark (PATH-like)

Compares V1 utility-only vs V2 conflict/supersession intelligence on a probe/train schedule:

```text
probe → positive train → probe → negative pressure → probe
```

```bash
go test ./internal/eval/ -run TestSequentialV2BeatsV1 -v
```

Expected shape:

```text
V2 task success        > V1 task success
V2 negative-transfer   < V1 negative-transfer
V2 post-conflict probe ≈ 1.0 after SUPERSEDES
```

## V2.2-7 Learned PATH Recovery Benchmark

Empty-store sequential benchmark: learn E1 under Env V1 (lenient Jira), then Env V2 shock forces recovery and E2 CONFLICT/SUPERSEDES — **no** seed + pre-run `ResolveConflict`.

```text
cold_start → train_v1 → probe_v1 → shock_v2 → recover_v2 → probe_v2
```

```bash
go test ./internal/eval/ -run TestLearnedPATHRecoveryAfterEnvShift -v
```

Expected shape:

```text
FGT              > 0     (probe_v1 beats cold_start)
NegativeTransfer > 0     (stale E1 fails under Env V2)
RecoveryTime     in (0,6]
E2 ACTIVE / E1 DEPRECATED (or supersession recorded)
probe_v2 success = 1.0
```

## V3-10 Skill vs Pattern Benchmark

Compares naive display-name Pattern tip vs validated Skill runtime under strict Jira sim:

```bash
go test ./internal/eval/ -run TestSkillBenchmarkBeatsPatternOnly -v
```

Expected:

```text
SkillSuccess       > PatternOnlySuccess
SkillSuccess       = 1.0
UnsafeSkillRate    = 0
Activated / ShadowOK = true
```

## V3.1 Learning-chain Benchmark

Proves Experience → Pattern → Skill → Feedback → Evolution (v2) under strict Jira sim:

```bash
go test ./internal/eval/ -run TestLearningChainBeatsPatternAndEvolves -v
```

Expected:

```text
SkillV1Success > PatternOnlySuccess
SkillV2Success > PatternOnlySuccess
UtilityAfterFail < UtilityAfterReward
V2Promoted / V1Superseded = true
UnsafeRate = 0
```

## V3.2 Counterfactual Replay

Offline with-skill vs pattern-tip ACE on strict Jira sim:

```bash
go test ./internal/eval/ -run TestReplayJiraCounterfactualPrefersSkill -v
```

Expected: `PreferSkill=true`, positive `ACEReward` (skill succeeds where tip-only fails).

## V3.3 Provider-path Replay

Same ACE check but routed through `toolprovider.Router` + simulator provider:

```bash
go test ./internal/replay/ -run TestJiraReplayEnvironmentPrefersSkill -v
go test ./internal/skillexec/ -run TestRecoveryContinuesFromStepCursor -v
```

## V3.4 Chaos / Trust Benchmarks

```bash
go test ./internal/skillruntime/chaos/ -v
```

Covers: stable operation key, credential isolation, lease/step fencing, RecoveryPlanner NATIVE retry vs NONE → NEEDS_RECONCILIATION.
