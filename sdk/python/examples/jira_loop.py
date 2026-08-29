#!/usr/bin/env python3
"""Episode → context → feedback loop example."""

from __future__ import annotations

import os
import sys

# Allow running without install: python examples/jira_loop.py
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from agent_experience import ExperienceClient


def main() -> None:
    base = os.environ.get("EXPERIENCE_ENGINE_URL", "http://localhost:8080")
    client = ExperienceClient(
        base_url=base,
        tenant_id="tenant_a",
        agent_id="agent_01",
        user_id="user_01",
    )
    print("health:", client.healthz())

    episode = client.start_episode(
        task_type="jira.create_issue",
        goal="Create Jira issue",
        input="project=Payment",
    )
    print("episode:", episode.id)

    context = client.get_context(
        task="Create a Jira issue for payment timeout",
        episode_id=episode.id,
        tools=["jira.search_projects", "jira.create_issue"],
        max_experiences=5,
    )
    print("experiences:", len(context.get("experiences") or []))

    episode.add_attempt(
        action="create_issue",
        tool_name="jira.create_issue",
        status="FAILED",
        error_code="INVALID_PROJECT_KEY",
    )
    episode.complete(status="SUCCESS", verified=True, verifier="tool")
    fb = client.feedback(episode_id=episode.id, source="business", reward=1.0, confidence=1.0)
    print("feedback utility_updates:", len(fb.get("utility_updates") or []))


if __name__ == "__main__":
    main()
