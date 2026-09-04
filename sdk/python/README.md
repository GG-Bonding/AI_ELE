# Python SDK

Thin client for the Agent Experience Learning Engine HTTP API (V1 + V2 surfaces).

```bash
cd sdk/python
pip install -e .
```

```python
from agent_experience import ExperienceClient, target_action_field

client = ExperienceClient(
    base_url="http://localhost:8080",
    tenant_id="tenant_a",
    agent_id="agent_01",
    user_id="user_01",
)

episode = client.start_episode(goal="Create Jira issue")
ctx = episode.get_context(task="Create a Jira issue", tools=["jira"])
action = episode.tool_call(
    tool="jira.create_issue",
    input={"project": "PAY", "priority": "High"},
    status="SUCCESS",
    context_id=ctx.get("context_id", ""),
)
episode.feedback(
    source="human",
    reward=-1.0,
    confidence=1.0,
    target=target_action_field(action["id"], "priority"),
)
client.evolve_patterns()
```

See `examples/jira_loop.py`.
