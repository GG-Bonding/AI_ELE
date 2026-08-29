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
