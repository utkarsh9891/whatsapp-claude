# 2026-10-27-fc6f0e — `#wa` commands also travel through notification chats, keeping the self-chat clean

- **Status:** accepted
- **Date:** 2026-10-27
- **Deciders:** Utkarsh Upadhyay
- **Scope:** bridge
- **Supersedes / Amends:** amends [2026-10-23-f28675](./2026-10-23-f28675-allowlist-carried-in-whatsapp-self-chat.md)

## Context
Allowlist changes reach every machine as `#wa` commands posted in the owner's "Message yourself" chat
([f28675](./2026-10-23-f28675-allowlist-carried-in-whatsapp-self-chat.md)). The owner keeps that chat for
personal notes and doesn't want the bridge posting there. They now have notification chats
([17d63a](./2026-10-27-17d63a-touch-id-send-approval-and-notification-chats.md)), groups reserved for Claude's
messages to them, and they want a chat added by a send ([16abb6](./2026-10-27-16abb6-touch-id-send-adds-the-chat.md))
to be captured on every machine.

The self-chat was safe as a command channel for one reason: only the owner's account can post as the owner.
That holds in any chat.

## Decision
1. **Control chats** are the owner's self-chat and every notification chat. A `#wa` command is obeyed in a control
   chat only if the owner's own account sent it; the other member of a notification group can't issue commands.
2. **Where the bridge posts commands:** `wa-allow notify <group>` posts into that group. Every other command from
   this machine (`wa-allow add/remove/direct/unnotify`) and the sync of a chat added by a send (`#wa add <JID>`)
   go to the first notification chat. The self-chat is used only when there is no notification chat, and a
   send-added chat is then not synced (it stays on the sending machine).
3. Unchanged: commands typed by the owner in the self-chat still work; commands are replayed from the pairing
   sync only for the self-chat.

## Assumptions
- 1. Only the owner's account can send messages that arrive as from the owner (WhatsApp end-to-end design).
- 2. Each machine knows its notification chats before it needs commands from them: a machine joins by making the
     group a notification chat itself (`wa-allow notify`, with Touch ID), or from a self-chat command.

## Reversal triggers
- Revisit if a notification group could be used by someone else to post as the owner (Assumption #1).
- Revisit if machines often miss commands because they lack the notification chat (Assumption #2).

## Consequences
- The bridge never writes to the self-chat while a notification chat exists.
- Notification chats now carry `#wa add <JID>` lines alongside Claude's messages.
- A machine paired fresh doesn't replay commands from notification chats; it needs `wa-allow notify` once.

## Alternatives considered
- **Keep the self-chat as the only channel:** rejected by the owner; it fills their personal chat.
- **Sync nothing (per-machine allowlists):** rejected by the owner; both Macs should capture the same chats.
- **A dedicated command group separate from notifications:** rejected; one more group to set up on every
  machine, for no gain in trust.

## What changed since 2026-10-23-f28675
The self-chat was the only chat whose `#wa` messages counted; notification chats, which only the owner (and at
most one account they chose) are in, now count too when the owner sends the command.
