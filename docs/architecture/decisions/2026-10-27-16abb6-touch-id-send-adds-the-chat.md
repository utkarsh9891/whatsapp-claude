# 2026-10-27-16abb6 — A send you approve with Touch ID may go to any chat, and adds it to the allowlist

- **Status:** accepted
- **Date:** 2026-10-27
- **Deciders:** Utkarsh Upadhyay
- **Scope:** bridge
- **Supersedes / Amends:** amends [2026-10-23-e302a3](./2026-10-23-e302a3-allowlist-gates-content-and-sends.md) and [2026-10-27-17d63a](./2026-10-27-17d63a-touch-id-send-approval-and-notification-chats.md)

## Context
The bridge refused any recipient not on the allowlist ([e302a3](./2026-10-23-e302a3-allowlist-gates-content-and-sends.md)),
because approvals then showed only a JID and couldn't catch a wrong-group send. Since
[17d63a](./2026-10-27-17d63a-touch-id-send-approval-and-notification-chats.md), every send to anyone but the owner
waits for the owner's Touch ID on a prompt that names the chat and shows the text. In use, "send this to group X"
from the owner failed on the allowlist, the owner had to ask for the chat to be added, and then approve the send:
three steps for one decision.

No signal separates "the owner typed this" from "a session decided this" that a session running as the owner
can't also produce; the owner's fingerprint is the one a session can't.

## Decision
1. **The bridge sends to a chat that isn't allowlisted** once the owner approves it with Touch ID. The prompt says
   what else happens: *WhatsApp for Claude is trying to send "…" to the group "X" and start saving its messages.*
2. **After that send succeeds, the chat is added to the allowlist** on this machine, and the bridge posts
   `#wa add <JID>` to the first notification chat so every machine adds it ([2026-10-27-fc6f0e](./2026-10-27-fc6f0e-commands-through-notification-chats.md)); with no notification
   chat it stays on this machine, because the owner keeps the self-chat free of such messages. Its last 50
   messages are fetched ([a8ea3b](./2026-10-27-a8ea3b-history-on-add-add-chat-and-named-prompts.md)).
3. Unchanged: sends to the owner's own chat need nothing and add nothing; notification chats must be allowlisted
   and send unattended; nothing is ever sent to an unlisted chat without Touch ID.

## Assumptions
- 1. The owner reads the prompt (chat name and text) before touching the sensor.
- 2. A chat the owner sends to is one they want to follow.

## Reversal triggers
- Revisit if the owner finds themselves approving prompts without reading them (Assumption #1): require the
  allowlist for sends again.
- Revisit if chats get captured that the owner didn't want (Assumption #2): send without adding.

## Consequences
- One request, one Touch ID, done, for any chat.
- T5 (wrong group) now rests on the named Touch ID prompt alone, not on a second, independent list: a steered
  session can reach any of the owner's chats if the owner approves without reading.
- The allowlist now decides what is stored, and grows with the chats the owner messages, on every machine.

## Alternatives considered
- **Keep refusing unlisted chats:** rejected by the owner; three steps for one decision.
- **Send without adding:** rejected by the owner; replies in a chat they wrote to should be visible.
- **Sync through the self-chat:** rejected; the owner keeps that chat for personal notes.
- **Keep it on the sending machine only:** rejected by the owner; both Macs should capture the same chats.
- **Detect owner-typed requests (a `UserPromptSubmit` hook recording the prompt):** rejected; a steered or
  mistaken session can name a chat the owner mentioned, and code running as the owner can forge the record.

## What changed since 2026-10-23-e302a3
"A send to an unlisted chat is refused at the local API" no longer holds: approvals now name the chat
(17d63a), so the owner's Touch ID replaces the list as the recipient check.
