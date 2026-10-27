# bridge — WhatsApp linked device

Manifest: [`module.yaml`](./module.yaml) · Back to [AGENTS.md](../../../AGENTS.md) ·
Decisions: [fork](../../../docs/architecture/decisions/2026-10-23-de419c-fork-and-harden-whatsapp-mcp.md),
[allowlist](../../../docs/architecture/decisions/2026-10-23-e302a3-allowlist-gates-content-and-sends.md)

| File | Responsibility |
|---|---|
| `main.go` | connection, pairing (QR or `-pair-phone` code), event handling, message store, history sync and history on add ([ADR a8ea3b](../../../docs/architecture/decisions/2026-10-27-a8ea3b-history-on-add-add-chat-and-named-prompts.md)), REST API |
| `config.go` | **guard file:** data dir, allowlist (`config.json`, re-read on change), bearer token, auth middleware, `safeName`, `sendablePath` (`send_from` folders), QR PNG |
| `selfcmd.go` | **guard file:** `#wa` allowlist commands from the owner's self-chat: parse, resolve names, apply once, react ([ADR f28675](../../../docs/architecture/decisions/2026-10-23-f28675-allowlist-carried-in-whatsapp-self-chat.md)) |
| `selfcmd_test.go` | tests for `selfcmd.go` |
| `history.go` | history on add: chat anchors, the on-demand request to the phone, what an on-demand answer may change |
| `history_test.go` | tests for `history.go` |
| `config_test.go` | tests for every function in `config.go` |
| `fixture_test.go` | writes a fixture `messages.db` through the real `MessageStore` when `FIXTURE_HOME` is set (used by the server tests) |

**API.** `POST /api/send`, `POST /api/download` and `POST /api/command` (posts `#wa <command>` to your self-chat and applies it here) on `127.0.0.1:<port>` (default 8080), header
`Authorization: Bearer <~/.whatsapp-mcp/token>`, `Content-Type: application/json`. Sends to a chat not on the
allowlist, and media outside the `send_from` folders (or in a hidden folder), are refused.

**Store.** `messages.db`: `chats(jid, name, last_message_time)` and `chat_anchors(jid, msg_id, from_me, ts)`
(latest message ID and time, no text, used to fetch history on add) for every chat; `messages(...)` for
allowlisted chats only. Timestamps are text with a UTC offset (`2026-10-23 18:37:13+05:30`).

**Logging.** Connection events and errors only. Message text is never logged.
