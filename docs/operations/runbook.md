# Runbook

Back to [AGENTS.md](../../AGENTS.md). Paths: [on-disk layout](../architecture/overview.md#on-disk-layout-whatsapp-mcp-mode-0700).

## Install

Prereqs (macOS): `brew install go uv` (`jq` ships with macOS 15+), and the Command Line Tools for `swiftc`
(`xcode-select --install`), which builds the Touch ID helper.

```text
/plugin marketplace add utkarsh9891/whatsapp-claude
/plugin install whatsapp@whatsapp-claude
/wa-setup 91XXXXXXXXXX
```

Without Claude: `plugins/whatsapp/bin/install.sh` then `plugins/whatsapp/bin/install.sh pair 91XXXXXXXXXX`.
If `wa` isn't found, add `~/.local/bin` to `PATH`.

## Removing an old Cursor install

Versions 0.3.x could add a Cursor guard. If `~/.cursor/hooks.json` still lists `cursor-guard.sh`, Cursor blocks every
MCP and shell call (the hook is fail-closed and its script is gone). Remove the entries:

```bash
jq '.hooks |= with_entries(.value |= map(select((.command // "") | test("cursor-guard") | not)))' \
  ~/.cursor/hooks.json > /tmp/h && mv /tmp/h ~/.cursor/hooks.json
rm -rf ~/.whatsapp-mcp/cursor-workspace ~/.whatsapp-mcp/cursor-taint ~/.local/bin/wa-cursor
```

## Pair (link this machine)

`install.sh pair <number>` prints an 8-character code: on the phone, **WhatsApp → Settings → Linked devices →
Link a device → Link with phone number instead**, enter it within ~2 minutes. Without a number it opens a QR
image to scan. On success the background service starts.

## Service

| Task | Command |
|---|---|
| status | `install.sh status` |
| restart | `install.sh service` |
| logs | `tail -f ~/.whatsapp-mcp/bridge.log` |
| stop | `launchctl bootout gui/$(id -u)/com.whatsapp-claude.bridge` |
| test Touch ID approval | `install.sh approve-test` (one prompt, nothing sent) |

## Update

`/plugin marketplace update whatsapp-claude`, then `/wa-setup` (rebuilds `~/.whatsapp-mcp/app`; data, pairing
and allowlist are kept).

## Several machines

Each machine is a separate linked device (WhatsApp allows four): install and pair on each. Replies sent from any
device reach all of them. Each machine stores from its own pairing onwards. The allowlist is shared through your
self-chat ([allowlist guide](../guides/allowlist.md)); nothing needs syncing.

## Uninstall

`install.sh uninstall` stops the service and removes the launchers; `~/.whatsapp-mcp` is kept. To unlink fully,
delete `~/.whatsapp-mcp` and remove the device under **Linked devices** on the phone.

## Troubleshooting

| Symptom | Check | Fix |
|---|---|---|
| `paired: no` after it worked | phone unlinked the device, or it expired | `install.sh pair <number>` |
| no new messages | `install.sh status`, `tail ~/.whatsapp-mcp/bridge.log` | `install.sh service`; if logged out, pair again |
| chat missing in `digest` | `wa-allow list <name>`: no `*`, or ⏳ pending | `wa-allow add` it; for ⏳, check the exact name or use the JID |
| `#wa` command got no reaction | that machine's bridge is down | `install.sh status`; it applies on reconnect |
| send refused "media must be inside" | file location | use a file under a `send_from` folder (`~/Downloads`, `~/Desktop`, `~/Documents`, `~/Pictures`, outbox), not in a hidden folder |
| "WhatsApp tools are blocked in permission mode" | you're in a normal session | use `wa` |
| `/plugin marketplace add` fails on clone | network blocks SSH | `gh auth login && gh auth setup-git`, or use the HTTPS URL |
| bridge won't build after an update | `go version` vs `go.mod` | `brew upgrade go` |
| build fails `unknown architecture arm64e.x1` | a newer SDK than the linker (`ls /Library/Developer/CommandLineTools/SDKs`) | `SDKROOT=…/MacOSX<your macOS>.sdk install.sh`, or reinstall the Command Line Tools |
| send fails "not approved" | the Touch ID prompt was declined or timed out (2 min) | send again and touch the sensor |
| send fails "Touch ID isn't available" | lid closed, or locked out after failed attempts | open the lid, or unlock the Mac with your password once |
| notification chat asks for Touch ID | `tail ~/.whatsapp-mcp/bridge.log`: more than two members, or you're not in it | remove the extra member |
