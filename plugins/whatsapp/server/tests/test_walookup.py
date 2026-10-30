"""bin/walookup.py (chats by name, number or part of a name, ADR 2026-10-30-7e2c9d) against a store the bridge
writes itself: messages.db through MessageStore, whatsapp.db through whatsmeow's store."""
import importlib
import os
import subprocess
import sys

import pytest

SERVER = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BRIDGE = os.path.join(SERVER, "..", "bridge")
BIN = os.path.join(SERVER, "..", "bin")

MEERA_LID = "100000000000001@lid"


@pytest.fixture(scope="module")
def home(tmp_path_factory):
    home = tmp_path_factory.mktemp("wa")
    subprocess.run(["go", "test", "-run", "TestWriteLookupFixture", "-count=1", "."],
                   cwd=BRIDGE, env={**os.environ, "LOOKUP_FIXTURE_HOME": str(home)}, check=True, capture_output=True)
    return home


@pytest.fixture
def lookup(home, monkeypatch):
    monkeypatch.setenv("WA_HOME", str(home))
    monkeypatch.syspath_prepend(BIN)
    sys.modules.pop("walookup", None)
    return importlib.import_module("walookup")


def jids(hits):
    return [p.jid for p in hits]


@pytest.mark.parametrize("query, want", [
    ("Meera", [MEERA_LID]),                       # saved under the number, chatting under the LID: the newer chat
    ("meera", [MEERA_LID]),
    ("1234567890", [MEERA_LID]),                  # number without country code
    ("+91 12345 67890", [MEERA_LID]),
    ("mee s", [MEERA_LID]),                       # prefixes of her self-chosen name
    ("Panda", ["100000000000002@lid"]),           # whole word beats "Pandawa"
    ("po panda", ["100000000000002@lid"]),
    ("Vijay", ["917000000004@s.whatsapp.net"]),   # a contact with no chat: their number
    ("field ops north", ["100@g.us"]),
    ("north", ["100@g.us"]),
    ("claude", ["300@g.us"]),
    ("zzz", []),
])
def test_find(lookup, query, want):
    assert jids(lookup.find(query)) == want


def test_ambiguous_lists(lookup):
    assert sorted(jids(lookup.find("pan po"))) == ["100000000000002@lid", "917000000004@s.whatsapp.net"]
    assert len(lookup.find("Field Ops")) == 2
    assert len(lookup.find("Ravi")) == 2          # two contacts saved with the same name
    assert len(lookup.find("panda", broad=True)) == 2


def test_labels(lookup):
    assert lookup.find("Meera")[0].label == "Meera (+911234567890)"
    assert lookup.find("Guddu")[0].label == "~Guddu Pandawa (+917000000003)"
    assert lookup.find("north")[0].label == "Field Ops North"


def test_resolve(lookup, capsys):
    assert lookup.resolve("Meera") == MEERA_LID
    assert "→ Meera (+911234567890)" in capsys.readouterr().err
    assert lookup.resolve("120363@g.us") == "120363@g.us"
    assert lookup.resolve("me") == "919000000001@s.whatsapp.net"
    assert lookup.resolve("zzz", missing_ok=True) is None
    with pytest.raises(SystemExit) as e:
        lookup.resolve("Ravi")
    assert "2 chats match" in str(e.value) and "nothing done" in str(e.value)
    with pytest.raises(SystemExit) as e:
        lookup.resolve("zzz")
    assert "No chat or contact matches" in str(e.value)


def run(home, *args):
    return subprocess.run([sys.executable, *args], cwd=BIN, env={**os.environ, "WA_HOME": str(home)},
                          capture_output=True, text=True)


def test_wa_allow_list(home):
    out = run(home, "wa-allow", "list", "pand")
    assert out.returncode == 0
    assert "Panda 🐼 Po (+917000000002)" in out.stdout and "~Guddu Pandawa" in out.stdout
    assert "Vijay Pandey - Police" in out.stdout   # contacts without a chat are found too
    full = run(home, "wa-allow", "list").stdout
    assert "Meera (+911234567890)" in full and "Vijay" not in full


def test_wa_send_refuses_ambiguous(home):
    out = run(home, "wa-send", "Ravi", "hello")
    assert out.returncode == 1 and "2 chats match" in out.stderr
