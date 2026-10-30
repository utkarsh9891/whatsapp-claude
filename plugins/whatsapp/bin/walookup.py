"""Find a WhatsApp chat by name, number or part of a name, for wa-send and wa-allow.

Reads messages.db (chats) and whatsapp.db (contacts, the LID-to-number map, this device) read-only.
A person's phone-number and LID chats are one match. An exact name wins, then names containing every
word typed as a whole word, then names where every word typed starts a word; a lookup acts only on a
single match. ADR 2026-10-30-7e2c9d.
"""
import os
import re
import sqlite3
import sys

HOME = os.environ.get("WA_HOME") or os.path.expanduser("~/.whatsapp-mcp")
SHOW = 15  # matches listed when a lookup is ambiguous


def ro(db):
    path = os.path.join(HOME, db)
    if not os.path.exists(path):
        sys.exit("No WhatsApp store: is the bridge paired and running? (install.sh status)")
    return sqlite3.connect(f"file:{path}?mode=ro", uri=True)


def norm(s):
    return " ".join(s.lower().split())


def words(s):
    return re.findall(r"\w+", s.lower())


def placeholder(name):
    """A chat name that is only a number (the bridge's fallback)."""
    return name.lstrip("+0123456789") == ""


class Person:
    """A group, or one person with all their chats and contact names."""

    def __init__(self, key):
        self.key = key
        self.chats = []  # (last message time, jid)
        self.names = set()  # every name to match against
        self.saved = ""  # owner's saved name, or a business name
        self.chosen = ""  # the name the person chose (push name)
        self.chat_name = ""
        self.number = ""  # phone number digits, for people

    @property
    def last(self):
        return max(self.chats)[0] if self.chats else ""

    @property
    def jid(self):
        """The address to use: the chat with the latest message, else the phone number."""
        if self.chats:
            return max(self.chats)[1]
        return f"{self.number}@s.whatsapp.net" if self.number else self.key

    @property
    def label(self):
        name = self.saved or self.chat_name or (f"~{self.chosen}" if self.chosen else "")
        number = f"+{self.number}" if self.number else ""
        return " ".join(x for x in (name, f"({number})" if name and number else number) if x) or self.jid


def people():
    """Every chat and contact this machine knows, grouped by person."""
    w = ro("whatsapp.db")
    lid_pn = {}
    try:
        lid_pn = {lid: pn for lid, pn in w.execute("SELECT lid, pn FROM whatsmeow_lid_map")}
    except sqlite3.Error:
        pass

    def key(jid):
        user, _, server = jid.partition("@")
        user = user.split(":")[0]
        if server == "lid":
            return lid_pn.get(user, jid)
        if server == "s.whatsapp.net":
            return user
        return jid

    found = {}

    def person(jid):
        k = key(jid)
        if k not in found:
            found[k] = Person(k)
            if k.isdigit():
                found[k].number = k
        return found[k]

    for jid, name, last in ro("messages.db").execute(
            "SELECT jid, COALESCE(name, ''), COALESCE(last_message_time, '') FROM chats"):
        p = person(jid)
        p.chats.append((last, jid))
        name = name.strip()
        if name and not placeholder(name):
            p.names.add(name.lstrip("~"))
            if not p.chat_name or not name.startswith("~"):
                p.chat_name = name
    try:
        contacts = w.execute("SELECT their_jid, COALESCE(full_name, ''), COALESCE(first_name, ''), "
                             "COALESCE(business_name, ''), COALESCE(push_name, '') FROM whatsmeow_contacts")
    except sqlite3.Error:
        contacts = []
    for jid, full, first, business, push in contacts:
        p = person(jid)
        for name in (full, first, business, push):
            if name.strip():
                p.names.add(name.strip())
        p.saved = p.saved or full.strip() or first.strip() or business.strip()
        p.chosen = p.chosen or push.strip()
    return list(found.values())


def find(query, everyone=None, broad=False):
    """People matching query, latest first: by number (7+ digits), else the first tier with a match of
    exact name, whole words, word prefixes. broad (for listing) returns every word-prefix match."""
    everyone = people() if everyone is None else everyone
    digits = re.sub(r"[\s()+-]", "", query)
    if digits.isdigit() and len(digits) >= 7:
        hits = [p for p in everyone if p.number and p.number.endswith(digits)]
    else:
        q = norm(query)
        hits = [p for p in everyone if any(norm(n) == q for n in p.names)]
        typed = words(query)
        if not hits and typed and not broad:
            hits = [p for p in everyone if any(set(typed) <= set(words(n)) for n in p.names)]
        if (broad or not hits) and typed:
            hits = [p for p in everyone
                    if any(all(any(w.startswith(t) for w in words(n)) for t in typed) for n in p.names)]
    return sorted(hits, key=lambda p: p.last, reverse=True)


def own_jid():
    row = ro("whatsapp.db").execute("SELECT jid FROM whatsmeow_device LIMIT 1").fetchone()
    if not row:
        sys.exit("Not paired: install.sh pair")
    return row[0].split("@")[0].split(":")[0] + "@s.whatsapp.net"


def listing(hits):
    lines = [f"  {p.label[:48]:48}  {p.jid}" for p in hits[:SHOW]]
    if len(hits) > SHOW:
        lines.append(f"  … and {len(hits) - SHOW} more")
    return "\n".join(lines)


def resolve(query, missing_ok=False):
    """The JID for query, naming the match on stderr. Exits when several match, or none (unless missing_ok)."""
    if "@" in query and " " not in query:
        return query
    if query.lower() in ("me", "myself", "self"):
        return own_jid()
    hits = find(query)
    if len(hits) == 1:
        print(f"→ {hits[0].label}  {hits[0].jid}", file=sys.stderr)
        return hits[0].jid
    if not hits:
        if missing_ok:
            return None
        sys.exit(f"No chat or contact matches {query!r} (wa-allow list <part of a name or number>)")
    sys.exit(f"{len(hits)} chats match {query!r}; nothing done. Be more specific, or pass the number or JID:\n"
             + listing(hits))
