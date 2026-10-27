"""Every read tool, called through FastMCP, against a store written by the bridge's own MessageStore.

FastMCP validates structured output against each tool's return annotation; a mismatch only shows up when the
tool is called, so call them all.
"""
import asyncio
import importlib
import os
import subprocess
import sys

import pytest

SERVER = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BRIDGE = os.path.join(SERVER, "..", "bridge")


@pytest.fixture(scope="module")
def mcp(tmp_path_factory):
    home = tmp_path_factory.mktemp("wa")
    subprocess.run(["go", "test", "-run", "TestWriteFixture", "-count=1", "."],
                   cwd=BRIDGE, env={**os.environ, "FIXTURE_HOME": str(home)}, check=True, capture_output=True)
    os.environ["WA_HOME"] = str(home)
    sys.path.insert(0, SERVER)
    for name in ("whatsapp", "main"):
        sys.modules.pop(name, None)
    return importlib.import_module("main").mcp


CALLS = [
    ("digest", {"since_hours": 24}),
    ("list_chats", {}),
    ("list_chats", {"query": "Field"}),
    ("get_chat", {"chat_jid": "100@g.us"}),
    ("get_chat", {"chat_jid": "nope@g.us"}),
    ("list_messages", {"chat_jid": "100@g.us", "limit": 5}),
    ("list_messages", {"query": "down", "include_context": False}),
    ("search_contacts", {"query": "91"}),
    ("get_contact_chats", {"jid": "912222"}),
    ("get_direct_chat_by_contact", {"sender_phone_number": "912222"}),
    ("get_last_interaction", {"jid": "912222"}),
    ("get_message_context", {"message_id": "n3"}),
]


@pytest.mark.parametrize("name,args", CALLS, ids=[f"{n}-{i}" for i, (n, _) in enumerate(CALLS)])
def test_read_tool_output_validates(mcp, name, args):
    result = asyncio.run(mcp.call_tool(name, args))
    assert result is not None


def test_every_read_tool_is_covered(mcp):
    tools = {t.name for t in asyncio.run(mcp.list_tools())}
    acting = {"send_message", "send_file", "send_audio_message", "download_media", "add_chat"}
    assert tools - acting == {n for n, _ in CALLS}


class _Resp:
    def __init__(self, status, body):
        self.status_code, self._body, self.text = status, body, str(body)

    def json(self):
        return self._body


@pytest.mark.parametrize("bridge,ok", [
    ((200, {"success": True, "result": "applied"}), True),
    ((200, {"success": True, "result": "ambiguous"}), False),
    ((503, {"message": "Not connected to WhatsApp"}), False),
])
def test_add_chat_posts_one_command(mcp, monkeypatch, bridge, ok):
    wa = sys.modules["whatsapp"]
    with open(os.path.join(wa.WA_HOME, "token"), "w") as f:
        f.write("t0ken\n")
    sent = []
    monkeypatch.setattr(wa.requests, "post", lambda url, json, headers, timeout: sent.append((url, json, headers)) or _Resp(*bridge))
    # a newline must not smuggle a second line into the self-chat command
    _, structured = asyncio.run(mcp.call_tool("add_chat", {"chat": "Field Ops\nNorth"}))
    assert sent == [(f"{wa.WHATSAPP_API_BASE_URL}/command", {"command": "add Field Ops North"},
                     {"Authorization": "Bearer t0ken"})]
    assert structured["result"]["success"] is ok
