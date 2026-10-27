# 2026-10-23-f28675 — The allowlist travels through WhatsApp's "Message yourself" chat

- **Status:** amended by [2026-10-27-fc6f0e](./2026-10-27-fc6f0e-commands-through-notification-chats.md)
- **Date:** 2026-10-23
- **Deciders:** Utkarsh Upadhyay
- **Scope:** bridge
- **Supersedes / Amends:** amends [2026-10-23-e302a3](./2026-10-23-e302a3-allowlist-gates-content-and-sends.md)

## Context
The owner uses several machines, each a separate linked device with its own `config.json`. Editing the allowlist
on each machine is error-prone. File-sync services (iCloud Drive, Dropbox) can't be relied on, because not every
machine can be set up with one. The allowlist should also be manageable from the phone, without a laptop.

Every linked device of an account already receives the messages in the account's own **"Message yourself"**
chat, and nobody but the account owner can post there.

## Decision
- **Commands in the self-chat drive the allowlist.** A message *from me*, in *my own chat*, whose text starts
  with `#wa ` is a command:
  - `#wa add <chat name | JID>`, `#wa remove <chat name | JID>`, `#wa direct on|off`.
  - Every bridge applies it to its own `config.json`. A bridge that was offline gets the command when it
    reconnects; a newly paired bridge applies the commands found in its history sync, oldest first.
- **Names resolve per machine.** A name is matched exactly (case-insensitive) against the chats this bridge
  knows. One match: its JID is added. No match yet: the name is kept as **pending** (`allow_names`) and resolves
  the first time a chat with that exact name is seen. Several matches: nothing is added (pass the JID instead).
- **Each command is applied once per machine**, tracked by message ID (`applied_commands`), so replays and
  reconnects are idempotent.
- **Feedback by reaction.** The bridge reacts to a live command: ✅ applied, ⏳ pending (name not seen yet),
  ❓ ambiguous, ❌ not understood. Reactions from the same account replace each other, so several machines
  produce one reaction, not a pile of replies.
- **`wa-allow add/remove/direct` posts the command** through the local bridge (new `POST /api/command`), so
  changes made on a laptop reach every machine too. `wa-allow list` stays a local read.
- **Only the owner's self-chat is trusted.** Commands in any other chat, or from anyone else, are ignored.

## Assumptions
- 1. WhatsApp delivers self-chat messages to every linked device, including ones offline when they were sent
     (multi-device behaviour, 2026-10).
- 2. A new linked device's history sync includes recent self-chat messages.
- 3. Only the account owner can post in the account's own chat.
- 4. Chat names are stable enough to use as allowlist keys; a JID is accepted where they aren't.

## Reversal triggers
- Revisit if new machines regularly miss older commands (Assumption #2): add a periodic full-state snapshot
  message, or a `wa-allow import` from another machine.
- Revisit if WhatsApp lets other people post into a user's self-chat (Assumption #3): the command channel
  would become an attack path and must be closed immediately.
- Revisit if group renames cause wrong matches in practice (Assumption #4): require JIDs.

## Consequences
- The allowlist is the same on every machine, with nothing set up per machine and no sync service.
- Allowlist changes leave a visible trail in the self-chat.
- A pending name could be claimed by a group created later with exactly that name and with the owner as a
  member. This is accepted for usability; `wa-allow list` shows pending names, and `#wa remove` clears them.
- Local hand edits of `config.json` still work, but stay on that machine.

## Alternatives considered
- **File sync (iCloud Drive / Dropbox symlink):** rejected; can't be set up on every machine.
- **Allowlist in the git repo:** rejected; the repo is public and group IDs are private.
- **WhatsApp pins, mutes or archives as the signal:** rejected; pins are capped at 3, and mute or archive already
  mean something else to the user.
- **Bridges post a full snapshot after each change:** rejected for now; every online machine would post, giving
  duplicates. Kept as the fallback under Assumption #2.
