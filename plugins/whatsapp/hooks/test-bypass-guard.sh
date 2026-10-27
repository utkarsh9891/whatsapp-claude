#!/bin/bash
# Cases for bypass-guard.sh; run by CI (job plugin).
set -u
guard="$(cd "$(dirname "$0")" && pwd)/bypass-guard.sh"
WA_HOME=$(mktemp -d); export WA_HOME
trap 'rm -rf "$WA_HOME"' EXIT
fail=0

# check label, expected ("block" = exit 2, "none" = exit 0 with no decision, or a decision), payload
check() {
  local out code got
  out=$(printf '%s' "$3" | "$guard" 2>/dev/null); code=$?
  if [ "$code" = 2 ]; then got=block
  elif [ "$code" = 0 ] && [ -z "$out" ]; then got=none
  else got=$(jq -r '.hookSpecificOutput.permissionDecision // "bad"' <<<"$out" 2>/dev/null || echo bad); fi
  if [ "$got" = "$2" ]; then echo "ok   $1"; else echo "FAIL $1: got $got, want $2"; fail=1; fi
}
# says label, payload, expected substring of the decision's reason
says() {
  local why
  why=$(printf '%s' "$2" | "$guard" 2>/dev/null | jq -r '.hookSpecificOutput.permissionDecisionReason // ""' 2>/dev/null)
  if [[ "$why" == *"$3"* ]]; then echo "ok   $1"; else echo "FAIL $1: reason \"$why\" lacks \"$3\""; fail=1; fi
}
p() { # mode, tool, recipient
  jq -nc --arg m "$1" --arg t "mcp__whatsapp__$2" --arg r "${3:-}" \
    '{hook_event_name: "PreToolUse", permission_mode: $m, tool_name: $t, tool_input: {recipient: $r}}'
}

check "manual (default) read"            none  "$(p default digest)"
check "acceptEdits read"                 none  "$(p acceptEdits list_chats)"
check "plan read"                        none  "$(p plan digest)"
check "auto read"                        none  "$(p auto digest)"
check "bypassPermissions blocked"        block "$(p bypassPermissions digest)"
check "dontAsk blocked"                  block "$(p dontAsk digest)"
check "unknown mode blocked"             block "$(p whatever digest)"
check "missing mode blocked"             block '{"tool_name":"mcp__whatsapp__digest"}'
check "malformed input blocked"          block 'not json'
check "empty input blocked"              block ''
check "Cursor input blocked"             block '{"cursor_version":"3.27.6","hook_event_name":"beforeMCPExecution","tool_name":"digest"}'
check "bypass send still blocked"        block "$(p bypassPermissions send_message 919999)"

check "send before pairing: asks"        ask   "$(p auto send_message 919999@s.whatsapp.net)"

sqlite3 "$WA_HOME/whatsapp.db" "CREATE TABLE whatsmeow_device (jid TEXT, lid TEXT); INSERT INTO whatsmeow_device VALUES ('919999:54@s.whatsapp.net', '12345:54@lid');"
check "send to own number: allowed"      allow "$(p auto send_message 919999)"
check "send to own PN JID: allowed"      allow "$(p default send_message 919999@s.whatsapp.net)"
check "send to own LID: allowed"         allow "$(p auto send_file 12345@lid)"
check "send to own device JID: allowed"  allow "$(p auto send_message 919999:54@s.whatsapp.net)"
check "send to someone else: asks"       ask   "$(p auto send_message 911111)"
check "send to a group: asks"            ask   "$(p auto send_file 120363000@g.us)"
check "own digits on group server: asks" ask   "$(p auto send_message 919999@g.us)"
check "LID digits as phone number: asks" ask   "$(p auto send_message 12345)"
check "empty recipient: asks"            ask   "$(p auto send_audio_message "")"
check "download: allowed"                allow "$(p auto download_media)"
check "download in bypass: blocked"      block "$(p bypassPermissions download_media)"

# Prompts name the chat from the bridge's store (ADR 2026-10-27-a8ea3b)
a() { # mode, chat
  jq -nc --arg m "$1" --arg c "$2" \
    '{hook_event_name: "PreToolUse", permission_mode: $m, tool_name: "mcp__whatsapp__add_chat", tool_input: {chat: $c}}'
}
rtl=$(printf '\342\200\256') # U+202E right-to-left override
sqlite3 "$WA_HOME/messages.db" "CREATE TABLE chats (jid TEXT PRIMARY KEY, name TEXT, last_message_time TIMESTAMP);
  INSERT INTO chats VALUES ('120363000@g.us', 'Field Ops North', NULL), ('911111@s.whatsapp.net', 'Asha', NULL),
  ('120363999@g.us', 'Evil$rtl'||char(10)||'Message yourself', NULL);"
says "send prompt names the group"         "$(p auto send_message 120363000@g.us)" '"Field Ops North" (120363000@g.us)'
says "send prompt names a phone number"    "$(p auto send_file 911111)" '"Asha" (911111@s.whatsapp.net)'
says "send prompt drops device part"       "$(p auto send_message 911111:3@s.whatsapp.net)" '"Asha" (911111@s.whatsapp.net)'
says "send prompt: unknown chat"           "$(p auto send_message 120363555@g.us)" "120363555@g.us (a chat this machine hasn't seen)"
says "send prompt: name cleaned"           "$(p auto send_message 120363999@g.us)" '"EvilMessage yourself" (120363999@g.us)'
check "quote in recipient: asks"           ask   "$(p auto send_message "1' OR '1'='1@g.us")"
says "quote in recipient: never looked up" "$(p auto send_message "1' OR '1'='1@g.us")" "an unreadable recipient"
check "add_chat: asks"                     ask   "$(a auto 120363000@g.us)"
check "add_chat in manual: asks"           ask   "$(a default "Field Ops North")"
check "add_chat in bypass: blocked"        block "$(p bypassPermissions add_chat)"
check "add_chat in dontAsk: blocked"       block "$(a dontAsk 120363000@g.us)"
says "add_chat prompt names the chat"      "$(a auto 120363000@g.us)" 'start capturing "Field Ops North" (120363000@g.us)?'
says "add_chat by name"                    "$(a auto "Field$rtl Ops")" 'the chat named "Field Ops"'
says "add_chat me"                         "$(a auto me)" 'your own "Message yourself" chat'

exit $fail
