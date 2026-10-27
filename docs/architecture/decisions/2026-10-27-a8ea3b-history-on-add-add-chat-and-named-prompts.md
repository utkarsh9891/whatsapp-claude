# 2026-10-27-a8ea3b — Fetch history when a chat is added, add chats from `wa`, and name the chat in prompts

- **Status:** amended by [2026-10-27-17d63a](./2026-10-27-17d63a-touch-id-send-approval-and-notification-chats.md)
- **Date:** 2026-10-27
- **Deciders:** Utkarsh Upadhyay
- **Scope:** bridge
- **Supersedes / Amends:** amends [2026-10-23-e302a3](./2026-10-23-e302a3-allowlist-gates-content-and-sends.md) and [2026-10-24-f10233](./2026-10-24-f10233-auto-mode-self-sends-and-send-folders.md)

## Context
The allowlist ([e302a3](./2026-10-23-e302a3-allowlist-gates-content-and-sends.md)) keeps the 200+ unchosen
groups off disk, and that still holds. In use it had three rough edges:
- A chat added late had **no history**: the question that prompts adding a group ("what did X say in Y?") is
  usually about a message that arrived before it was added, so the first answer was always "nothing stored".
- Adding a chat meant leaving the `wa` session for the phone or another terminal.
- A send approval showed only a JID (`send to 1203…@g.us?`). With many similarly named groups, the tap could
  not catch a wrong-group send, so the allowlist was the only real check on the recipient.

WhatsApp can return older messages of one chat on request (a peer "history sync on demand" to the primary phone),
but only relative to a message the requester knows: "the N messages before message X".

## Decision
1. **History on add.** When a chat is added by a live command (phone, `wa-allow`, `add_chat`, or another
   machine), the bridge asks the phone for the **50 messages** before that chat's latest known message and
   stores them like any allowlisted message. To have that reference point, the bridge keeps, for **every**
   chat, its latest message's ID, from-me flag and time in `chat_anchors` (no text, no sender, no media).
   If none is known yet (a chat silent since the bridge was upgraded), it asks when the chat's next message
   arrives, using that message as the reference (kept in memory only).
   The answer is applied as content only: it does not change chat activity times and its `#wa` commands are
   ignored (an old `remove` must not undo a newer `add`). Not fetched for `#wa direct on` (could be hundreds of
   chats), for commands replayed from the pairing sync (that sync carries history already), or for a pending
   name that resolves later.
2. **`add_chat` in `wa`.** A new MCP tool posts `#wa add <chat>` through the bridge, so every machine applies
   it. The guard **asks every time**, in every allowed mode, including `auto`; `wa -p` can't answer, so it is
   refused there.
3. **Prompts name the chat.** The guard's approval text for sends and `add_chat` shows the chat's name from the
   bridge's store next to the JID, or says the chat is unknown on this machine. The name is set by chat members,
   so it is shown quoted, stripped of control characters and cut to 80 characters; the JID stays the identifier.

## Assumptions
- 1. The phone answers on-demand history requests while it is online (whatsmeow `BuildHistorySyncRequest`,
     pinned version of 2026-10-17; not yet verified live: the first live add on 2026-10-27 is
     waiting for the chat's next message).
- 2. The last 50 messages cover the typical "catch me up on this chat" question.
- 3. A message ID and time without text are metadata of the same kind as the chat names already stored for
     every chat.

## Reversal triggers
- Revisit if the phone stops answering on-demand requests (Assumption #1): history on add silently does nothing,
  and the docs must say so again.
- Revisit if 50 messages routinely miss what is asked about (Assumption #2): page further back.

## Consequences
- A chat is useful the moment it is added, provided the phone is online.
- Every chat now writes one small anchor row per message; content for unchosen chats is still never stored.
- Allowlist changes can come from the `wa` session itself, always behind a prompt, so a message asking the
  assistant to "add this group" can't do it unseen.
- The send prompt becomes a real recipient check, which a later decision could use to relax the send side of
  the allowlist.

## Alternatives considered
- **Store every chat's content, show only allowlisted chats to the model:** rejected; puts every group's personal
  data on disk ([e302a3](./2026-10-23-e302a3-allowlist-gates-content-and-sends.md) Assumption #2).
- **Only wait for the next message in the chat and fetch before it:** rejected as the main path; a quiet chat
  would stay empty for days. Kept as the fallback when no message is known.
- **Let `add_chat` run without a prompt in auto mode:** rejected; it widens what is stored and who can be sent
  to, which is exactly what an injected message would ask for.
- **Show the name only:** rejected; group names are chosen by members and can imitate another chat.

## What changed since 2026-10-23-e302a3
"History from before a chat was added is not backfilled" no longer holds: the bridge fetches the last 50 messages
on add. Since 2026-10-24-f10233: the guard's `ask` for sends names the chat, and `add_chat` is a new asked action.
