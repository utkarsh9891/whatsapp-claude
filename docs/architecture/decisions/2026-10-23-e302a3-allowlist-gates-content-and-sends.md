# 2026-10-23-e302a3 — The bridge stores content and allows sends only for allowlisted chats

- **Status:** amended by [2026-10-23-f28675](./2026-10-23-f28675-allowlist-carried-in-whatsapp-self-chat.md), [2026-10-27-a8ea3b](./2026-10-27-a8ea3b-history-on-add-add-chat-and-named-prompts.md), [2026-10-27-16abb6](./2026-10-27-16abb6-touch-id-send-adds-the-chat.md)
- **Date:** 2026-10-23
- **Deciders:** Utkarsh Upadhyay
- **Scope:** bridge
- **Supersedes / Amends:** —

## Context
The account is in 200+ groups of field agents, many of which carry merchant and customer personal data. Storing
every group would mean hundreds of MB of mostly irrelevant, sensitive text on disk, and any "catch me up" would
cost tens of thousands of tokens. Sending is the most dangerous action: with hundreds of similarly named
groups, a mistaken or injected send can reach the wrong audience.

## Decision
The bridge enforces a per-machine allowlist in `~/.whatsapp-mcp/config.json`:
- `allow_chats`: chat JIDs whose **message content** is stored, whose media can be downloaded, and which can be
  **sent to**;
- `allow_direct_chats`: treat every 1:1 chat as allowlisted;
- **names and last-activity times of all chats are always stored**, so chats can be discovered and added;
- the bridge, not the AI client, makes the decision: a send to an unlisted chat is refused at the local API;
- the file is re-read when it changes, so no restart is needed; history from before a chat was added is not
  backfilled.

## Assumptions
- 1. Only a small subset of the user's chats need capture or replies (true for the 200+ group account).
- 2. The model provider receiving message text is acceptable only for chats the user deliberately chose.
- 3. An empty allowlist is the right default for a new install (nothing captured until opted in).

## Reversal triggers
- Revisit if most chats need capture, making an allowlist pure friction; a denylist would then fit (Assumption #1).
- Revisit if the allowlist needs to be shared between machines without per-machine edits (planned as its own ADR).

## Consequences
- New installs capture nothing until the user runs `wa-allow add`; the [allowlist guide](../../guides/allowlist.md)
  explains this.
- `digest` reports active chats that are *not* allowlisted by name only, so the user can see what they're missing.
- A chat added late has no earlier history on that machine.

## Alternatives considered
- **Store everything, filter at read time:** rejected; sensitive data at rest and token cost (Assumptions #1, #2).
- **Allowlist enforced only in the MCP server or prompts:** rejected; the bridge is the one component every
  client must go through, so it is the only place a send restriction can't be bypassed.
- **Denylist:** rejected under Assumption #1.
