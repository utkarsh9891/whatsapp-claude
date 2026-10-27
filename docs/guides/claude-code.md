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
| "Reply in Field Ops North that the fix is live" | the assistant drafts the text and names the chat; the send asks for your approval, naming the chat |
| "Start watching Field Ops North" | `add_chat` asks for your approval, naming the chat; then its last 50 messages are fetched |
| "Send that to myself" / "note this in my self-chat" | sent to your own chat without asking |
| "Download the PDF I sent myself" | downloaded without a prompt to `~/.whatsapp-mcp/media/`; Claude can open it there |
| "Send ~/Downloads/report.pdf to Field Ops North" | files from `~/Downloads`, `~/Desktop`, `~/Documents`, `~/Pictures` or the outbox; asks first unless it is to yourself |

Only allowlisted chats have content: [allowlist guide](./allowlist.md). `wa` refuses bypass and config-override
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

It is read-only by design, except for sends to yourself: read tools are pre-allowed, and sends to others and
`add_chat` need an approval a background run can't give, so they are refused.
