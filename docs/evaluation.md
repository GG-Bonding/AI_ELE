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
