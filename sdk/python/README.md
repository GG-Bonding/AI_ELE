# Python SDK

Thin client for the Agent Experience Learning Engine HTTP API.

```bash
cd sdk/python
pip install -e .
```

```python
from agent_experience import ExperienceClient

client = ExperienceClient(
    base_url="http://localhost:8080",
    tenant_id="tenant_a",
    agent_id="agent_01",
    user_id="user_01",
)

episode = client.start_episode(goal="Create Jira issue")
episode.add_attempt(action="create_issue", tool_name="jira.create_issue", status="FAILED")
episode.complete(status="SUCCESS", verified=True, verifier="tool")

context = client.get_context(task="Create a Jira issue", episode_id=episode.id, tools=["jira"])
client.feedback(episode_id=episode.id, source="business", reward=1.0, confidence=1.0)
```

See `examples/jira_loop.py`.
