# 2026-10-24-f10233 — Auto mode, free sends to yourself, and sending from everyday folders

- **Status:** amended by [2026-10-27-a8ea3b](./2026-10-27-a8ea3b-history-on-add-add-chat-and-named-prompts.md), [2026-10-27-17d63a](./2026-10-27-17d63a-touch-id-send-approval-and-notification-chats.md)
- **Date:** 2026-10-24
- **Deciders:** Utkarsh Upadhyay
- **Scope:** plugin
- **Supersedes / Amends:** amends [2026-10-23-8f4095](./2026-10-23-8f4095-dedicated-session-and-bypass-guard.md)

## Context
In daily use the guard made the tool hard to use: every action in `wa` prompted (manual mode), Claude Code's
newer **auto** mode was blocked outright (the guard only knew `default`, `acceptEdits` and `plan`), downloads of
PDFs the owner sent to themselves were held behind prompts, and files could only be sent from
`~/.whatsapp-mcp/outbox`, which nothing fills.

Auto mode is not "prompts off": a safety classifier reviews each action before it runs, and `ask` rules and hook
`ask` decisions still stop for the user (verified 2026-10-24 on Claude Code 2.1.309). The risk the guard exists
for, a message steering an action nobody sees, is what auto mode's classifier and an explicit send approval
address.

## Decision
1. **The guard allows `auto`** alongside `default`/`manual`, `acceptEdits` and `plan`. `bypassPermissions`,
   `dontAsk` and anything unknown stay blocked.
2. **`wa` starts in auto mode**; `wa --manual` starts with prompts on everything.
3. **The guard decides sends itself**, by recipient: to the owner's own chat (phone-number or LID JID, read from
   the bridge's device store) it **allows** without a prompt; to anyone else it **asks**; if the owner's IDs
   can't be read it asks. `download_media` is allowed without a prompt; downloads land in `~/.whatsapp-mcp/media/`,
   which the `wa` session may read.
4. **Files can be sent from everyday folders:** the bridge accepts media from a configurable `send_from` list in
   `config.json` (default `~/.whatsapp-mcp/outbox`, `~/Downloads`, `~/Desktop`, `~/Documents`, `~/Pictures`),
   resolved through symlinks, and refuses any path with a hidden component (`.ssh`, `.aws`, `.config`, …).

## Assumptions
- 1. Claude Code's auto mode runs a classifier on every tool call and still honours `ask` rules and hook `ask`
     decisions (verified 2026-10-24, 2.1.309).
- 2. Hooks receive `permission_mode` `auto` in auto mode and `default` for manual (verified 2026-10-24).
- 3. Only the account owner can post in the owner's own chat, so a send there reaches nobody else.
- 4. Secrets live in hidden folders or outside the `send_from` folders.

## Reversal triggers
- Revisit if auto mode stops honouring `ask` (Assumption #1): block `auto` again, as for `dontAsk`.
- Revisit if credentials or keys turn up inside a `send_from` folder in practice (Assumption #4): narrow the list.

## Consequences
- Routine reading, downloading and opening files run without prompts in `wa`; sends to other people still take
  one tap.
- Sending a file Claude didn't create no longer needs a copy into the outbox.
- An injected message could make Claude post into the owner's own chat without asking; nobody else sees it.

## Alternatives considered
- **Keep manual mode and the outbox:** rejected; unusable in practice (the reason for this ADR).
- **Let auto mode decide all sends:** rejected by the owner; a send to another person keeps a human tap.
- **Any path except a denylist:** rejected; an allowlist of folders fails closed for new secret locations.
