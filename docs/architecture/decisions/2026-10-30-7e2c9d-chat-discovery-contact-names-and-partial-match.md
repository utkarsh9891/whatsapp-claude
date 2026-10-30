# 2026-10-30-7e2c9d — Find chats by contact name, number or part of a name

- **Status:** accepted
- **Date:** 2026-10-30
- **Deciders:** Utkarsh Upadhyay
- **Scope:** bridge
- **Supersedes / Amends:** —

## Context
`wa-send` and `wa-allow` found a chat only by its exact stored name, and the bridge named a 1:1 chat once, when
it first saw it. WhatsApp now addresses most 1:1 chats by LID (`…@lid`), while saved contacts are keyed by phone
number, so the bridge found no contact for them and stored the digits as the name, for good. In use, the owner
asked to message a saved contact and neither the name nor the number found the chat; the Touch ID prompt then
named the person by an ID nobody recognises, which weakens T5 (wrong recipient).

## Decision
1. **The bridge names 1:1 chats from contacts on both addresses.** For a LID chat it also looks up the phone
   number's contact (WhatsApp's LID↔number map), and the reverse. The owner's saved name wins (full name, then
   first name), then a business name, then the name the person set for themselves, shown as `~Name` (WhatsApp's
   own mark for a name the other person chose). Chats still named by digits or a `~` name are renamed when a
   better name is known, at connect and on each message.
2. **The CLIs resolve a chat by name, number or part of a name**, in one shared lookup (`bin/walookup.py`):
   - a JID or `me` as before;
   - a number with at least 7 digits, with or without country code, finds that person's chats;
   - a name matches chat names and contact names (saved, business, self-chosen), ignoring case, in tiers, the
     first tier with a match deciding: an **exact** name; names containing every word typed as a **whole
     word** ("Panda" finds "Panda Po", not "Pandawa"); names where every word typed **starts** a word
     ("pan po" finds "Panda Po").
   - A person's phone and LID chats count as one; the one with the latest message is used.
   - **One match is used** and the CLI prints which chat it chose; **several are listed** with numbers and
     nothing is done; none is an error.
3. `wa-allow list <search>` uses the same matching and shows contact names; `wa-allow add/remove/notify/unnotify
   <name>` post the resolved JID, and fall back to posting the name when nothing on this machine matches (so a
   pending name still works).

## Assumptions
- 1. The owner reads the chat named in the Touch ID prompt before approving, so a prefix that picks the wrong
     person is caught there (16abb6 #1). Sends without a prompt go to the owner's own or notification chats, or
     an armed chat ([032c66](./2026-10-30-032c66-armed-sends-approve-a-pattern-ahead.md)).
- 2. WhatsApp keeps the LID↔number map on the linked device for chats the owner talks in.

## Reversal triggers
- Revisit if a prefix match leads to an approved send to the wrong person (Assumption #1): require exact names
  for sends.
- Revisit if LID chats again show digits for saved contacts (Assumption #2).

## Consequences
- Chats show the names the owner knows, in `wa`, the CLIs and the prompts.
- A chat can be named by someone else's self-chosen name, as groups already are; the `~` says so.
- The CLIs read the contact and LID tables in `whatsapp.db` (read-only), as `wa-send` already read the device row.

## Alternatives considered
- **One prefix tier, no whole-word tier:** rejected; "Panda" would tie "Panda Po" with "Pandawa" and do nothing.
- **Substring matching:** rejected; "an" would match most chats, and prefix matching covers how people shorten
  names.
- **Use the first of several matches:** rejected; the wrong person may get the message (T5).
- **Resolve names in the bridge for the CLIs:** rejected for now; the lookup is read-only and local, and the
  bridge already resolves `#wa` names for other machines.
- **Leave push names unmarked:** rejected; anyone can set their name to look like one of the owner's contacts.
