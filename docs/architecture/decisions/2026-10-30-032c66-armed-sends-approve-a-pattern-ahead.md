# 2026-10-30-032c66 — Armed sends: approve a message pattern with Touch ID ahead of time

- **Status:** accepted
- **Date:** 2026-10-30
- **Deciders:** Utkarsh Upadhyay
- **Scope:** bridge
- **Supersedes / Amends:** amends [2026-10-27-17d63a](./2026-10-27-17d63a-touch-id-send-approval-and-notification-chats.md)

## Context
Since [17d63a](./2026-10-27-17d63a-touch-id-send-approval-and-notification-chats.md) every send to anyone but the
owner waits for the owner's Touch ID at the moment it is sent, and fails after 120 s. The owner often wants a
message sent when a step finishes ("tell the reviewer once the build is green, with the PR link"), and steps finish
while they are away from the Mac, so the prompt times out and the message is lost. Notification chats don't help:
the message is for someone else.

The owner is at the Mac when the task starts, and knows the shape of the message then, but not its values (the PR
link, a count, a build number). Exact-text approval ahead of time doesn't cover that; an open approval for "any
text to this chat" would let any process holding the token send anything as the owner for the whole window.

## Decision
1. **Arming.** `POST /api/arm {recipient, pattern, for, times}` (and `wa-send --arm <chat> [--for 2h] [--times N]
   <pattern>`) asks for the owner's Touch ID with the whole pattern in the prompt: *WhatsApp for Claude is trying
   to send messages like "Build {id} is green: {url}" to "Asha" without asking, once in the next 2 h ({id} is a
   short ID, {url} is an https link).* Arming first checks the bridge is connected, so a send that can't work fails
   now, while the owner is there, not at the step.
2. **Patterns have typed slots, no free choice of type.** Fixed text plus `{num}` (digits with `.` or `,`, up to
   20), `{id}` (a ticket, PR, branch, SHA or version: letters, digits and `._:/#-`, up to 64, no spaces), `{url}`
   (one `https://` link, up to 200, no spaces) and `{text}` (one line, up to 100 characters). `{{` and `}}` are
   literal braces. A pattern is refused if it names another slot, has no fixed letter or digit (it would be an open
   approval), or can't be shown whole in the prompt (over 150 characters, or control or text-direction
   characters).
3. **A send uses an armed pattern** only if it goes to the same chat, is text (files always ask), and its **whole**
   text matches the pattern, each slot filled with a value of its type. Each use counts against `times` (default
   1, at most 20); the pattern expires after `for` (default 2 h, at most 24 h). A use is given back if WhatsApp
   then refuses the send. A send that matches nothing asks for Touch ID as before, saying when it didn't match what
   was armed for that chat.
4. **Armed patterns live in the bridge's memory only**, never on disk, so writing a file can't create one; a bridge
   restart drops them (and the send then asks). `wa-send --armed` lists them; `wa-send --disarm [<chat>|<id>]`
   drops them without a prompt, since that only takes rights away.
5. Unchanged: your own chat and notification chats need nothing and can't be armed; an approved send to a chat not
   on the allowlist adds it ([16abb6](./2026-10-27-16abb6-touch-id-send-adds-the-chat.md)), and the arming prompt
   says so.

## Assumptions
- 1. The owner reads the pattern, chat and window in the prompt before touching the sensor (as in 16abb6 #1).
- 2. Typed slots carry what the owner's step-X messages need (links, IDs, counts, a short summary).
- 3. Between arming and use, the risk the owner accepts is: any process with the token can send to that chat any
     message that fits the pattern, until the count or window runs out.

## Reversal triggers
- Revisit if armed patterns are mostly bare `{text}` with a word around it (Assumption #2 fails and they are open
  approvals in disguise): narrow `{text}` or drop it.
- Revisit if an armed pattern is used for a message the owner didn't intend (Assumption #3): bind a use to a secret
  returned at arming, or to the arming process.

## Consequences
- A message to someone else can go out unattended at the step, after one Touch ID at the start of the task.
- T14 ("the token alone can't send to anyone but the owner") now has a bounded exception: while a pattern is
  armed, the token can send messages that fit it to that chat. `{url}` admits any https link and `{text}` any 100
  characters, so a steered process could put a link or a line of its choosing in front of that person; the prompt
  shows which slots a pattern has.
- Arming is lost on restart (upgrade, crash, reboot); re-arm and check with `wa-send --armed`.
- Not covered: files, notification chats, the MCP tools (they send through the bridge, so they use armed patterns,
  but can't arm).

## Alternatives considered
- **Exact text only:** rejected by the owner; values aren't known when the task starts (Assumption #2).
- **Open approval (any text, one chat, a window):** rejected; the token alone could send anything as the owner for
  the window.
- **Free regex patterns:** rejected; a regex is hard to read in a prompt, and `.*` is an open approval.
- **Queue a timed-out send and ask again when the owner is back:** deferred; keeps the guard whole but delivers
  late, which defeats "tell them when it's done". Can be added beside arming.
- **Bind a use to a secret returned at arming:** rejected for now; the secret would sit in the arming session's
  transcript or environment, readable by the same processes that can read the token.
- **Store armed patterns in `config.json`:** rejected; any process running as the owner could write one.

## What changed since 2026-10-27-17d63a
"Sends to others wait while the owner is away, and fail after 120 s" no longer holds for a send that fits a
pattern the owner armed with Touch ID beforehand.
