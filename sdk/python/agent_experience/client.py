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
    ) -> dict[str, Any]:
        return self._request(
            "POST",
            "/api/v1/feedback",
            body={
                "tenant_id": self._resolve_tenant(tenant_id),
                "episode_id": episode_id,
                "source": source,
                "signal": signal,
                "reward": reward,
                "confidence": confidence,
                "evidence": evidence,
            },
        )

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
