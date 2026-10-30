# bridge — WhatsApp linked device

Manifest: [`module.yaml`](./module.yaml) · Back to [AGENTS.md](../../../AGENTS.md) ·
Decisions: [fork](../../../docs/architecture/decisions/2026-10-23-de419c-fork-and-harden-whatsapp-mcp.md),
[allowlist](../../../docs/architecture/decisions/2026-10-23-e302a3-allowlist-gates-content-and-sends.md)

| File | Responsibility |
|---|---|
| `main.go` | connection, pairing (QR or `-pair-phone` code), event handling, message store, history sync and history on add ([ADR a8ea3b](../../../docs/architecture/decisions/2026-10-27-a8ea3b-history-on-add-add-chat-and-named-prompts.md)), REST API |
| `config.go` | **guard file:** data dir, allowlist (`config.json`, re-read on change), bearer token, auth middleware, `safeName`, `sendablePath` (`send_from` folders), QR PNG |
| `selfcmd.go` | **guard file:** `#wa` allowlist commands from the owner in the self-chat or a notification chat ([ADR fc6f0e](../../../docs/architecture/decisions/2026-10-27-fc6f0e-commands-through-notification-chats.md)): parse, resolve names, apply once, react ([ADR f28675](../../../docs/architecture/decisions/2026-10-23-f28675-allowlist-carried-in-whatsapp-self-chat.md)) |
| `approve.go` | **guard file:** Touch ID approval before sends to others, notification-chat membership check, prompt text ([ADR 17d63a](../../../docs/architecture/decisions/2026-10-27-17d63a-touch-id-send-approval-and-notification-chats.md)) |
| `names.go` | names 1:1 chats from contacts on both the phone and LID address, self-chosen names marked `~`, and renames digit-named chats ([ADR 7e2c9d](../../../docs/architecture/decisions/2026-10-30-7e2c9d-chat-discovery-contact-names-and-partial-match.md)) |
| `names_test.go` | tests for `names.go` |
| `armed.go` | **guard file:** armed sends: pattern slots, the arming prompt, in-memory patterns and their use ([ADR 032c66](../../../docs/architecture/decisions/2026-10-30-032c66-armed-sends-approve-a-pattern-ahead.md)) |
| `armed_test.go` | tests for `armed.go` |
| `approve_test.go` | tests for `approve.go` and the `notify` commands |
| `selfcmd_test.go` | tests for `selfcmd.go` |
| `history.go` | history on add: chat anchors, the on-demand request to the phone, what an on-demand answer may change |
| `history_test.go` | tests for `history.go` |
| `config_test.go` | tests for every function in `config.go` |
| `fixture_test.go` | writes a fixture `messages.db` through the real `MessageStore` when `FIXTURE_HOME` is set (used by the server tests) |

**API.** `POST /api/send` (Touch ID first unless to you or a notification chat), `POST /api/download`, `POST /api/command` (posts `#wa <command>` to your notification chat, or the self-chat if you have none, and applies it here; `notify` asks for Touch ID) `POST /api/approve-test` (one prompt, nothing sent), and `POST /api/arm` / `/api/armed` / `/api/disarm` (approve a message pattern for a chat ahead of time with Touch ID, list, drop; [ADR 032c66](../../../docs/architecture/decisions/2026-10-30-032c66-armed-sends-approve-a-pattern-ahead.md)) on `127.0.0.1:<port>` (default 8080), header
`Authorization: Bearer <~/.whatsapp-mcp/token>`, `Content-Type: application/json`. A send to a chat not on the
allowlist asks for Touch ID and, once sent, adds the chat here; media outside the `send_from` folders (or in a
hidden folder) is refused.

**Store.** `messages.db`: `chats(jid, name, last_message_time)` and `chat_anchors(jid, msg_id, from_me, ts)`
(latest message ID and time, no text, used to fetch history on add) for every chat; `messages(...)` for
allowlisted chats only. Timestamps are text with a UTC offset (`2026-10-23 18:37:13+05:30`).

**Logging.** Connection events and errors only. Message text is never logged.
