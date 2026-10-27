# plugin — Claude Code packaging

Manifest: [`module.yaml`](./module.yaml) · Back to [AGENTS.md](../../AGENTS.md) ·
Decision: [ADR 2026-10-23-8f4095](../../docs/architecture/decisions/2026-10-23-8f4095-dedicated-session-and-bypass-guard.md)

| Path | What it is |
|---|---|
| [`hooks/bypass-guard.sh`](./hooks/bypass-guard.sh) | `PreToolUse` hook on `mcp__.*whatsapp__.*`: allows only `default`, `acceptEdits`, `plan`, `auto`; sends to yourself and downloads go through, sends to others and `add_chat` ask, naming the chat; fails closed. Test: [`hooks/test-bypass-guard.sh`](./hooks/test-bypass-guard.sh) |
| [`wa/settings.json`](./wa/settings.json) | permissions of the `wa` session: starts in auto mode, bypass disabled, reads allowed |
| [`bin/wa`](./bin/wa) | launches the WhatsApp-only Claude session ([guide](../../docs/guides/claude-code.md)) |
| [`bin/wa-allow`](./bin/wa-allow) | allowlist CLI ([guide](../../docs/guides/allowlist.md)) |
| [`bin/install.sh`](./bin/install.sh) | `install`, `pair [phone]`, `service`, `status`, `uninstall` ([runbook](../../docs/operations/runbook.md)) |
| [`commands/wa-setup.md`](./commands/wa-setup.md) | `/wa-setup [phone]`: install, pair and start the service from Claude Code |
| [`skills/whatsapp-triage/SKILL.md`](./skills/whatsapp-triage/SKILL.md) | how the assistant reads (digest first), and replies (draft, confirm target, approve) |
| [`bridge/`](./bridge/README.md), [`server/`](./server/README.md) | the two runtime modules |

Changing any guard file: read the [security model](../../docs/architecture/security-model.md) first.
