from __future__ import annotations

import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import urlparse

from agent_experience import APIError, ExperienceClient


class _Handler(BaseHTTPRequestHandler):
    def log_message(self, format: str, *args) -> None:  # noqa: A003
        return

    def _read_json(self):
        length = int(self.headers.get("Content-Length", "0"))
        if length == 0:
            return None
        return json.loads(self.rfile.read(length))

    def _write_json(self, status: int, payload):
        raw = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def do_GET(self):  # noqa: N802
        path = urlparse(self.path).path
        if path == "/healthz":
            self._write_json(200, {"status": "ok"})
            return
        self._write_json(404, {"error": "not found"})

    def do_POST(self):  # noqa: N802
        path = urlparse(self.path).path
        body = self._read_json()
        if path == "/api/v1/episodes":
            self._write_json(
                201,
                {
                    "id": "ep1",
                    "tenant_id": body["tenant_id"],
                    "agent_id": body["agent_id"],
                    "user_id": body["user_id"],
                    "goal": body["goal"],
                    "status": "RUNNING",
                },
            )
            return
        if path == "/api/v1/episodes/ep1/attempts":
            self._write_json(201, {"id": "at1", "episode_id": "ep1", "status": body["status"]})
            return
        if path == "/api/v1/episodes/ep1/outcome":
            self._write_json(
                201,
                {
                    "episode": {"id": "ep1", "tenant_id": "tenant_a", "status": "SUCCESS"},
                    "outcome": {"id": "out1", "status": "SUCCESS"},
                },
            )
            return
        if path == "/api/v1/context":
            assert body["task"]
            self._write_json(
                200,
                {
                    "context_id": "ctx1",
                    "disclaimer": "untrusted",
                    "patterns": [
                        {
                            "id": "pat1",
                            "type": "PROCEDURAL",
                            "content": "resolve key first",
                            "utility": 0.8,
                            "confidence": 0.9,
                        }
                    ],
                    "experiences": [
                        {
                            "type": "PROCEDURAL",
                            "content": "Resolve project key first",
                            "source": "x",
                            "confidence": 0.9,
                        }
                    ],
                    "selections": [],
                },
            )
            return
        if path == "/api/v1/episodes/ep1/actions":
            assert body.get("context_id") == "ctx1"
            self._write_json(
                201,
                {
                    "id": "act1",
                    "episode_id": "ep1",
                    "tool_name": body["tool_name"],
                    "status": body["status"],
                    "context_id": "ctx1",
                },
            )
            return
        if path == "/api/v1/feedback":
            self._write_json(
                201,
                {"feedback": {"id": "fb1"}, "episode_reward": {"weighted_reward": 1.0}},
            )
            return
        if path == "/api/v1/patterns/evolve":
            self._write_json(201, {"created": [], "skipped": 0})
            return
        self._write_json(404, {"error": "not found"})


class ClientTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.server = HTTPServer(("127.0.0.1", 0), _Handler)
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()
        host, port = cls.server.server_address
        cls.base = f"http://{host}:{port}"

    @classmethod
    def tearDownClass(cls) -> None:
        cls.server.shutdown()
        cls.server.server_close()
        cls.thread.join(timeout=2)

    def test_loop(self) -> None:
        client = ExperienceClient(
            base_url=self.base,
            tenant_id="tenant_a",
            agent_id="a",
            user_id="u",
        )
        self.assertEqual(client.healthz()["status"], "ok")
        episode = client.start_episode(goal="Create Jira issue")
        self.assertEqual(episode.id, "ep1")
        episode.add_attempt(action="create_issue", status="FAILED", error_code="INVALID_PROJECT_KEY")
        episode.complete(status="SUCCESS", verified=True, verifier="tool")
        ctx = client.get_context(task="Create a Jira issue", episode_id=episode.id, tools=["jira"])
        self.assertEqual(len(ctx["experiences"]), 1)
        fb = client.feedback(episode_id=episode.id, source="business", reward=1.0, confidence=1.0)
        self.assertIn("feedback", fb)

    def test_v2_tracing_and_targets(self) -> None:
        from agent_experience import target_action_field

        client = ExperienceClient(base_url=self.base, tenant_id="tenant_a", agent_id="a", user_id="u")
        episode = client.start_episode(goal="create jira")
        ctx = episode.get_context(task="create PAY jira issue", tools=["jira"])
        self.assertEqual(ctx["context_id"], "ctx1")
        self.assertEqual(len(ctx["patterns"]), 1)
        action = episode.tool_call(
            tool="jira.create_issue",
            input={"project": "PAY"},
            status="SUCCESS",
            context_id=ctx["context_id"],
        )
        self.assertEqual(action["id"], "act1")
        fb = episode.feedback(
            source="human",
            reward=-1.0,
            confidence=1.0,
            target=target_action_field(action["id"], "priority"),
        )
        self.assertIn("feedback", fb)
        evolved = client.evolve_patterns()
        self.assertIn("created", evolved)

    def test_api_error(self) -> None:
        client = ExperienceClient(base_url=self.base, tenant_id="t")
        with self.assertRaises(APIError):
            client.get_episode("missing")


if __name__ == "__main__":
    unittest.main()
