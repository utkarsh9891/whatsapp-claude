# 2026-10-27-17d63a — The bridge asks for Touch ID before sending to others; notification chats send unattended

- **Status:** amended by [2026-10-27-16abb6](./2026-10-27-16abb6-touch-id-send-adds-the-chat.md), [2026-10-30-032c66](./2026-10-30-032c66-armed-sends-approve-a-pattern-ahead.md)
- **Date:** 2026-10-27
- **Deciders:** Utkarsh Upadhyay
- **Scope:** bridge
- **Supersedes / Amends:** amends [2026-10-24-f10233](./2026-10-24-f10233-auto-mode-self-sends-and-send-folders.md) and [2026-10-27-a8ea3b](./2026-10-27-a8ea3b-history-on-add-add-chat-and-named-prompts.md)

## Context
A send to anyone but the owner was approved through a Claude Code prompt that the guard hook raised. That had
two problems:
- **It only covered the MCP tools.** Any process running as the owner can read `~/.whatsapp-mcp/token` and
  `POST /api/send` directly, including a bypass-mode Claude session steered by a web page or file. Nothing asked.
- **It needed an interactive Claude session.** `wa -p`, a `!` command and other sessions could not send to anyone
  but the owner, because nothing could answer the prompt. Sending from wherever the owner is working wasn't possible.

The owner also wants unattended messages, such as overnight pings or questions while they are away, in a chat
reserved for that, without approving each one.

macOS can ask the device owner to authenticate (Touch ID, Apple Watch or the account password) from any process
in their login session, including a launchd agent (verified 2026-10-28 with a `launchctl submit` job). A process
cannot answer that prompt for the owner.

## Decision
1. **The bridge asks for owner authentication before every send to anyone but the owner**, whoever calls it: the
   `wa` session, `wa -p`, the new `wa-send` CLI, or a raw API call. It runs a small Swift helper installed next to
   the bridge as **WhatsApp for Claude** (source `approve/wa-approve.swift`), whose prompt reads as a sentence:
   *WhatsApp for Claude is trying to send "…" to the group "Field Ops North".* It names the chat, says whether it
   is a group, and shows the start of the text (or the file name); the JID only when no name is known. No
   approval within 120 s, a cancel, or a missing helper means no send. One prompt at a time.
2. **The guard lets sends through when the helper is installed**, because the bridge now decides; without the
   helper (an older bridge), sends to others still ask as before. Sends to the owner's own chat need nothing.
3. **Notification chats send unattended.** `notify_chats` in `config.json` lists groups the owner reserves for
   messages from Claude. Sends to them need no approval, but the bridge checks the group's members with WhatsApp
   on every send: at most two members, one of them the owner. If anyone else joins, sends ask again. A chat is made
   a notification chat with `#wa notify <chat>` in the self-chat (the phone is the proof) or `wa-allow notify`
   (which asks for Touch ID), and stops with `#wa unnotify <chat>`. A notification chat is also allowlisted, so
   replies are stored.
4. **`wa-send <chat> <text>`** sends from any terminal or `!` command, through the same bridge checks, without a
   Claude session.

## Assumptions
- 1. A process can't approve a LocalAuthentication prompt for the owner (macOS design; holds unless the owner
     has disabled the login password and biometrics).
- 2. The launchd agent can show the prompt (verified 2026-10-28, macOS 26.6).
- 3. A group with at most two members, one being the owner, reaches only the owner and one account they chose.

## Reversal triggers
- Revisit if the prompt stops appearing for launchd agents (Assumption #2): sends to others would all be refused;
  fall back to the guard's `ask` by removing the helper.
- Revisit if a notification chat is found reaching someone the owner didn't choose (Assumption #3).

## Consequences
- Sends to others can come from any session and always reach the owner's fingerprint first; the token is no
  longer enough to send as the owner.
- Unattended messages work, but only into chats that can't reach other people.
- A prompt shows at most ~150 characters of the text; longer messages are approved on their start.
- Sends to others wait while the owner is away from the Mac, and fail after 120 s.
- Residual, accepted by the owner on 2026-10-28: code running as the owner (a steered bypass-mode session) can
  swap the helper, add a group it shares with the owner to `notify_chats`, or build a bridge without the check
  and send with the session keys in `whatsapp.db`. Touch ID stops the tools and the API, not code set on getting
  around it ([security model](../security-model.md#residual-risks-accepted-stated-plainly)).

## Alternatives considered
- **A global switch for unattended sends:** rejected; while on, a message in any group could steer a send to any
  allowlisted group with nobody watching.
- **Approve in the WhatsApp self-chat:** rejected by the owner; it fills their personal chat with approvals.
- **A plain macOS dialog:** rejected; a bypass-mode process with accessibility access can click it.
- **Root-owned bridge, helper and notify list:** rejected; the owner isn't an admin on their Mac, and it leaves
  the session keys readable, so a rebuilt bridge still sends.
- **Session keys encrypted with a Keychain key bound to the bridge's code identity:** deferred; it would close all
  three routes but is days of work (whatsmeow's store has no encryption) for a risk the owner accepts while
  bypass sessions reach other channels anyway. Revisit if sending from bypass sessions becomes routine.
- **Show the JID next to the name** (as the guard's prompt in a8ea3b does): rejected after the owner found the
  first prompt unreadable; "the group" vs a person's name keeps a group from passing as a person, and the JID
  appears whenever the name is unknown.
- **Keep the guard's `ask` as well:** rejected; two approvals per send, and it still blocks every non-interactive caller.

## What changed since 2026-10-24-f10233
Sends to others are approved in the bridge with owner authentication instead of by the guard's `ask`, so they
work from non-interactive sessions and from the API. Since 2026-10-27-a8ea3b: the send prompt that names the chat
is now the Touch ID prompt.
