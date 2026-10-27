# Choosing which chats are captured

Back to [AGENTS.md](../../AGENTS.md). Decisions: [allowlist](../architecture/decisions/2026-10-23-e302a3-allowlist-gates-content-and-sends.md),
[shared through the self-chat](../architecture/decisions/2026-10-23-f28675-allowlist-carried-in-whatsapp-self-chat.md).

Nothing is captured until you allow it. The bridge keeps every chat's **name** so you can find it, but stores
**messages**, downloads media and **sends** only for allowlisted chats.

## From your phone (applies to every machine)

Open WhatsApp's **Message yourself** chat and send:

| Message | Effect |
|---|---|
| `#wa add Field Ops North` | capture and allow replies to the chat with exactly this name (any letter case); its last 50 messages are fetched |
| `#wa add 1203630000@g.us` | the same, by JID (use when two chats share a name) |
| `#wa add me` | your own "Message yourself" chat (also `myself`, `self`); it has no name to match |
| `#wa remove Field Ops North` | stop capturing and sending |
| `#wa direct on` / `#wa direct off` | allow, or stop allowing, every 1:1 chat |
| `#wa notify Claude pings` | make a group a **notification chat**: Claude's messages to it send without Touch ID ([below](#notification-chats)) |
| `#wa unnotify Claude pings` | stop that; the chat stays allowlisted |

Each machine's bridge applies the command and reacts: ✅ applied · ⏳ pending (no chat with that name seen yet on
that machine; it applies when the chat appears) · ❓ two or more chats have that name, use the JID · ❌ not
understood. Machines that are asleep apply it when they reconnect; a newly paired machine applies past commands
from its history sync. Only messages **you** send in **your own** chat count; nobody else can change your allowlist.

## From the `wa` session

Ask: "start watching Field Ops North". The assistant finds the chat and calls `add_chat`, which always asks
you first and names the chat; it then works like `#wa add`. Not available in `wa -p`.

## From a laptop

```bash
wa-allow list                    # every chat seen; * allowlisted, ⏳ pending names
wa-allow list ops                # filter by name or JID
wa-allow add "Field Ops North"   # posts "#wa add Field Ops North" to your self-chat → every machine
wa-allow remove "Field Ops North"
wa-allow direct on
wa-allow notify "Claude pings"     # asks for Touch ID on this machine
```

`wa-allow add/remove/direct` need the bridge running (`install.sh status`), because they send through it.

## Notification chats

For messages Claude should send while you're away (overnight pings, questions waiting for your answer), keep a
group with only you in it, or you and one account you control. Create the group with someone, remove them, and
name it (e.g. "Claude pings"); then `#wa notify Claude pings`. Sends to it never ask, from any session. Before each
send the bridge checks the group with WhatsApp: more than two members, or you not in it, and it asks for Touch ID
again ([ADR 17d63a](../architecture/decisions/2026-10-27-17d63a-touch-id-send-approval-and-notification-chats.md)).
Replies there are stored like any allowlisted chat.

## Notes

- **History on add:** adding a chat asks your phone for its last 50 messages, so it is useful straight away.
  The phone must be online; if it isn't, capture starts from the next message. A chat silent since the bridge
  was upgraded gets its history when its next message arrives. Not done for `#wa direct on`, or
  for a pending name that resolves later.
- Removing a chat stops capture and sends; already-stored messages stay on each machine until deleted.
- Hand edits of `~/.whatsapp-mcp/config.json` still work but stay on that machine.
- `send_from` in `config.json` lists the folders files can be sent from (default: outbox, `~/Downloads`,
  `~/Desktop`, `~/Documents`, `~/Pictures`). Paths with a hidden folder (`.ssh`, `.aws`, …) are always refused.
