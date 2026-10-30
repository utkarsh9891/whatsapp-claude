# Using WhatsApp from Claude Code

Back to [AGENTS.md](../../AGENTS.md). Why it works this way: [ADR 2026-10-23-8f4095](../architecture/decisions/2026-10-23-8f4095-dedicated-session-and-bypass-guard.md).

Start the WhatsApp session in a terminal with **`wa`**. It is a Claude Code session with only the WhatsApp tools,
in **auto mode** (Claude's safety check reviews each action, so routine steps don't prompt), with bypass mode
disabled. `wa --manual` starts with a prompt for every action instead. Your other sessions can't use WhatsApp; if they try, the
guard hook blocks it.

| You ask | What happens |
|---|---|
| "What needs my attention in the last 12 hours?" | `digest` runs (no prompt); you get a summary by group |
| "Show me the escalation in Field Ops North" | `list_messages` for that chat and window (no prompt) |
| "Reply in Field Ops North that the fix is live" | the assistant drafts the text and names the chat; the bridge shows a Touch ID prompt with the chat and the text |
| "Ping me in Claude pings when the build finishes" | a notification chat: sent without asking, even if you're away |
| "Tell Asha when the build is green, with the PR link" | arm the pattern now (one Touch ID), sent unattended at the step: [sending later](#sending-later-while-youre-away) |
| "Start watching Field Ops North" | `add_chat` asks for your approval, naming the chat; then its last 50 messages are fetched |
| "Send that to myself" / "note this in my self-chat" | sent to your own chat without asking |
| "Download the PDF I sent myself" | downloaded without a prompt to `~/.whatsapp-mcp/media/`; Claude can open it there |
| "Send ~/Downloads/report.pdf to Field Ops North" | files from `~/Downloads`, `~/Desktop`, `~/Documents`, `~/Pictures` or the outbox; Touch ID unless it is to yourself or a notification chat |

Only allowlisted chats have content; sending to a chat (after Touch ID) adds it: [allowlist guide](./allowlist.md). `wa` refuses bypass and config-override
flags; other Claude flags pass through.

## Model

`wa` runs on a cheap model by default: **`haiku`**. Choose another for one session with `--model`, or for every
`wa` in a shell with `WA_MODEL`:

```bash
wa                      # haiku
wa --model sonnet       # this session only
WA_MODEL=opus wa        # any Claude model name or alias
```

## Background (no session)

`wa -p "<question>"` answers once and exits, on the same default model:

```bash
wa -p "what needs my attention in the last 12 hours?"
```

Sends work there too: to yourself and notification chats straight away, to anyone else after your Touch ID.
`add_chat` needs a Claude prompt a background run can't answer, so it is refused.

## Sending from any terminal

`wa-send` sends without a Claude session, through the same bridge checks; from another Claude session use it
as `! wa-send …`:

```bash
wa-send "Claude pings" "build finished"         # notification chat: no prompt
wa-send "Field Ops North" "fix is live"         # Touch ID first
wa-send me --file ~/Downloads/report.pdf        # your own chat: no prompt
```

The Touch ID prompt takes Touch ID only (no password), and waits two minutes. With the lid closed, sends to
others fail; notification chats and your own chat still work.

### Sending later, while you're away

A send to someone else needs your Touch ID when it goes out. For a message that should go out when a step
finishes, **arm** it while you're at the Mac: approve its pattern now, fill in the values later
([ADR 032c66](../architecture/decisions/2026-10-30-032c66-armed-sends-approve-a-pattern-ahead.md)).

```bash
wa-send --arm "Asha" --for 3h "Build {id} is green, PR: {url}"   # Touch ID now, shows the whole pattern
# … at the step, from any session, no prompt:
wa-send "Asha" "Build 4812 is green, PR: https://github.com/acme/app/pull/77"
wa-send --armed                                                # what is armed, uses left, expiry
wa-send --disarm "Asha"                                        # drop it (or an id; nothing = all)
```

| Slot | Holds |
|---|---|
| `{num}` | digits, with `.` or `,` (up to 20) |
| `{id}` | a ticket, PR, branch, SHA or version: letters, digits, `._:/#-`, no spaces (up to 64) |
| `{url}` | one `https://` link (up to 200) |
| `{text}` | any one line (up to 100 characters) |

`{{` and `}}` are literal braces. The whole message must match the pattern; anything else asks for Touch ID as
usual, and says it didn't match. `--times N` allows N messages (default 1, at most 20); `--for` takes `90m`,
`2h`, `1h30m` (default 2 h, at most 24 h). Text only: files always ask. A bridge restart drops armed patterns.
Arming also checks the bridge is connected, so a send that can't work fails now rather than at the step.
Prefer fixed text and narrow slots: while armed, anything that can read the token can send that chat a message
that fits.
