"""Thin Python client for the Agent Experience Learning Engine."""

from __future__ import annotations

import json
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass, field
from typing import Any, Optional


class APIError(Exception):
    def __init__(self, status_code: int, body: str) -> None:
        self.status_code = status_code
        self.body = body
        super().__init__(f"experience api: status {status_code}: {body}")


@dataclass
class ExperienceClient:
    """HTTP client for /api/v1."""

    base_url: str
    tenant_id: str = ""
    agent_id: str = ""
    user_id: str = ""
    timeout: float = 30.0
    _opener: urllib.request.OpenerDirector = field(default_factory=urllib.request.build_opener, repr=False)

    def __post_init__(self) -> None:
        self.base_url = self.base_url.rstrip("/")

    def _resolve_tenant(self, tenant_id: str = "") -> str:
        return tenant_id or self.tenant_id

    def _request(
        self,
        method: str,
        path: str,
        *,
        query: Optional[dict[str, str]] = None,
        body: Any = None,
    ) -> Any:
        url = self.base_url + path
        if query:
            url += "?" + urllib.parse.urlencode(query)
        data = None
        headers = {}
        if body is not None:
            data = json.dumps(body).encode("utf-8")
            headers["Content-Type"] = "application/json"
        req = urllib.request.Request(url, data=data, headers=headers, method=method)
        try:
            with self._opener.open(req, timeout=self.timeout) as resp:
                raw = resp.read().decode("utf-8")
                if not raw:
                    return None
                return json.loads(raw)
        except urllib.error.HTTPError as exc:
            try:
                err_body = exc.read().decode("utf-8", errors="replace")
            finally:
                exc.close()
            raise APIError(exc.code, err_body.strip()) from exc

    def healthz(self) -> dict[str, str]:
        return self._request("GET", "/healthz")

    def start_episode(
        self,
        *,
        goal: str,
        tenant_id: str = "",
        agent_id: str = "",
        user_id: str = "",
        task_type: str = "",
        input: str = "",
    ) -> "Episode":
        payload = self._request(
            "POST",
            "/api/v1/episodes",
            body={
                "tenant_id": self._resolve_tenant(tenant_id),
                "agent_id": agent_id or self.agent_id,
                "user_id": user_id or self.user_id,
                "task_type": task_type,
                "goal": goal,
                "input": input,
            },
        )
        return Episode(client=self, data=payload)

    def get_episode(self, episode_id: str, *, tenant_id: str = "") -> dict[str, Any]:
        return self._request(
            "GET",
            f"/api/v1/episodes/{urllib.parse.quote(episode_id)}",
            query={"tenant_id": self._resolve_tenant(tenant_id)},
        )

    def get_context(
        self,
        *,
        task: str,
        tenant_id: str = "",
        agent_id: str = "",
        user_id: str = "",
        episode_id: str = "",
        tools: Optional[list[str]] = None,
        max_experiences: int = 0,
        max_patterns: int = 0,
        max_tokens: int = 0,
        top_k: int = 0,
    ) -> dict[str, Any]:
        return self._request(
            "POST",
            "/api/v1/context",
            body={
                "tenant_id": self._resolve_tenant(tenant_id),
                "agent_id": agent_id or self.agent_id,
                "user_id": user_id or self.user_id,
                "episode_id": episode_id,
                "task": task,
                "tools": tools or [],
                "max_experiences": max_experiences,
                "max_patterns": max_patterns,
                "max_tokens": max_tokens,
                "top_k": top_k,
            },
        )

    def feedback(
        self,
        *,
        episode_id: str,
        source: str,
        tenant_id: str = "",
        signal: str = "",
        reward: Optional[float] = None,
        confidence: float = 0.0,
        evidence: str = "",
        target: Optional[dict[str, Any]] = None,
    ) -> dict[str, Any]:
        body: dict[str, Any] = {
            "tenant_id": self._resolve_tenant(tenant_id),
            "episode_id": episode_id,
            "source": source,
            "signal": signal,
            "reward": reward,
            "confidence": confidence,
            "evidence": evidence,
        }
        if target is not None:
            body["target"] = target
        return self._request("POST", "/api/v1/feedback", body=body)

    def search_experiences(
        self,
        *,
        task: str,
        tenant_id: str = "",
        tools: Optional[list[str]] = None,
        top_k: int = 0,
    ) -> list[dict[str, Any]]:
        out = self._request(
            "POST",
            "/api/v1/experiences/search",
            body={
                "tenant_id": self._resolve_tenant(tenant_id),
                "task": task,
                "tools": tools or [],
                "top_k": top_k,
            },
        )
        return list(out.get("experiences") or [])

    def supersede(self, old_id: str, replacement_id: str, *, tenant_id: str = "") -> dict[str, Any]:
        return self._request(
            "POST",
            f"/api/v1/experiences/{urllib.parse.quote(old_id)}/supersede",
            body={
                "tenant_id": self._resolve_tenant(tenant_id),
                "replacement_id": replacement_id,
            },
        )

    def get_pattern(self, pattern_id: str, *, tenant_id: str = "") -> dict[str, Any]:
        return self._request(
            "GET",
            f"/api/v1/patterns/{urllib.parse.quote(pattern_id)}",
            query={"tenant_id": self._resolve_tenant(tenant_id)},
        )

    def generalize_patterns(self, experience_ids: list[str], *, tenant_id: str = "") -> dict[str, Any]:
        return self._request(
            "POST",
            "/api/v1/patterns/generalize",
            body={
                "tenant_id": self._resolve_tenant(tenant_id),
                "experience_ids": experience_ids,
            },
        )

    def evolve_patterns(self, *, tenant_id: str = "", min_utility: float = 0.0) -> dict[str, Any]:
        body: dict[str, Any] = {"tenant_id": self._resolve_tenant(tenant_id)}
        if min_utility:
            body["min_utility"] = min_utility
        return self._request("POST", "/api/v1/patterns/evolve", body=body)

    def apply_pattern_reward(
        self,
        pattern_id: str,
        *,
        reward: float,
        confidence: float = 1.0,
        idempotency_key: str = "",
        tenant_id: str = "",
    ) -> dict[str, Any]:
        return self._request(
            "POST",
            f"/api/v1/patterns/{urllib.parse.quote(pattern_id)}/reward",
            body={
                "tenant_id": self._resolve_tenant(tenant_id),
                "reward": reward,
                "confidence": confidence,
                "idempotency_key": idempotency_key,
            },
        )

    def propose_skill(self, pattern_id: str, *, tenant_id: str = "") -> dict[str, Any]:
        return self._request(
            "POST",
            f"/api/v1/patterns/{urllib.parse.quote(pattern_id)}/skill",
            body={"tenant_id": self._resolve_tenant(tenant_id)},
        )

    def get_skill(self, skill_id: str, *, tenant_id: str = "") -> dict[str, Any]:
        return self._request(
            "GET",
            f"/api/v1/skills/{urllib.parse.quote(skill_id)}",
            query={"tenant_id": self._resolve_tenant(tenant_id)},
        )


def target_episode() -> dict[str, str]:
    return {"type": "EPISODE"}


def target_action(action_id: str) -> dict[str, str]:
    return {"type": "ACTION", "action_id": action_id}


def target_action_field(action_id: str, field: str) -> dict[str, str]:
    return {"type": "ACTION_FIELD", "action_id": action_id, "field": field}


def target_tool(tool_name: str) -> dict[str, str]:
    return {"type": "TOOL", "tool_name": tool_name}


def target_experience(experience_id: str) -> dict[str, str]:
    return {"type": "EXPERIENCE", "experience_id": experience_id}


@dataclass
class Episode:
    client: ExperienceClient
    data: dict[str, Any]

    @property
    def id(self) -> str:
        return str(self.data["id"])

    @property
    def tenant_id(self) -> str:
        return str(self.data["tenant_id"])

    def add_attempt(
        self,
        *,
        action: str = "",
        tool_name: str = "",
        status: str = "",
        hypothesis: str = "",
        error_code: str = "",
        error_message: str = "",
        sequence: int = 0,
        input: Any = None,
        output: Any = None,
    ) -> dict[str, Any]:
        return self.client._request(
            "POST",
            f"/api/v1/episodes/{urllib.parse.quote(self.id)}/attempts",
            body={
                "tenant_id": self.tenant_id,
                "hypothesis": hypothesis,
                "action": action,
                "tool_name": tool_name,
                "input": input,
                "output": output,
                "status": status,
                "error_code": error_code,
                "error_message": error_message,
                "sequence": sequence,
            },
        )

    def complete(
        self,
        *,
        status: str,
        verified: bool = False,
        verifier: str = "",
        result: Any = None,
        metrics: Optional[dict[str, float]] = None,
    ) -> dict[str, Any]:
        return self.client._request(
            "POST",
            f"/api/v1/episodes/{urllib.parse.quote(self.id)}/outcome",
            body={
                "tenant_id": self.tenant_id,
                "status": status,
                "result": result,
                "verified": verified,
                "verifier": verifier,
                "metrics": metrics or {},
            },
        )

    def get_context(self, *, task: str, tools: Optional[list[str]] = None, **kwargs: Any) -> dict[str, Any]:
        return self.client.get_context(
            task=task,
            tenant_id=self.tenant_id,
            episode_id=self.id,
            tools=tools,
            **kwargs,
        )

    def record_action(
        self,
        *,
        tool_name: str = "",
        status: str = "",
        type: str = "TOOL_CALL",
        input: Any = None,
        output: Any = None,
        context_id: str = "",
        attempt_id: str = "",
        sequence: int = 0,
    ) -> dict[str, Any]:
        return self.client._request(
            "POST",
            f"/api/v1/episodes/{urllib.parse.quote(self.id)}/actions",
            body={
                "tenant_id": self.tenant_id,
                "type": type,
                "tool_name": tool_name,
                "input": input,
                "output": output,
                "status": status,
                "attempt_id": attempt_id,
                "sequence": sequence,
                "context_id": context_id,
            },
        )

    def tool_call(
        self,
        *,
        tool: str,
        input: Any = None,
        status: str = "SUCCESS",
        context_id: str = "",
    ) -> dict[str, Any]:
        return self.record_action(tool_name=tool, input=input, status=status, context_id=context_id)

    def link_experience(
        self,
        action_id: str,
        experience_id: str,
        *,
        influence: Optional[float] = None,
        affected_fields: Optional[list[str]] = None,
        evidence: str = "",
    ) -> dict[str, Any]:
        return self.client._request(
            "POST",
            f"/api/v1/episodes/{urllib.parse.quote(self.id)}/actions/{urllib.parse.quote(action_id)}/links",
            body={
                "tenant_id": self.tenant_id,
                "experience_id": experience_id,
                "influence": influence,
                "affected_fields": affected_fields or [],
                "evidence": evidence,
            },
        )

    def feedback(self, *, source: str, reward: Optional[float] = None, **kwargs: Any) -> dict[str, Any]:
        return self.client.feedback(
            episode_id=self.id,
            tenant_id=self.tenant_id,
            source=source,
            reward=reward,
            **kwargs,
        )
