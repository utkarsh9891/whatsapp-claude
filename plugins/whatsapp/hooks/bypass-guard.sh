#!/bin/bash
# Guard for WhatsApp MCP tools (ADR 2026-10-23-8f4095, amended by 2026-10-24-f10233, 2026-10-27-a8ea3b
# and 2026-10-27-17d63a).
# Incoming messages are untrusted text, so the tools only run in permission modes
# where Claude still checks actions: manual (default), acceptEdits, plan, auto.
# Bypass, dontAsk and unknown modes are blocked; malformed input fails closed.
# Sends to the owner's own chat are allowed. Sends to anyone else are allowed when the
# bridge's Touch ID helper is installed, because the bridge then asks the owner itself;
# with an older bridge the user is asked here. Downloads are allowed. Adding a chat always asks.
# Prompts name the chat from the bridge's store next to its JID.
WA_HOME="${WA_HOME:-$HOME/.whatsapp-mcp}"
input=$(cat)

# Cursor also runs Claude Code hooks, without a permission_mode: blocked as "unknown".
mode=$(jq -r '.permission_mode // "unknown"' 2>/dev/null <<<"$input") || mode="unknown"
case "$mode" in
  default|acceptEdits|plan|auto) ;;
  *)
    echo "WhatsApp tools are blocked in permission mode '$mode'. Start a WhatsApp session with: wa" >&2
    exit 2
    ;;
esac

decide() { # decision, reason
  jq -nc --arg d "$1" --arg r "$2" \
    '{hookSpecificOutput: {hookEventName: "PreToolUse", permissionDecision: $d, permissionDecisionReason: $r}}'
  exit 0
}

# is_self reports whether a recipient (phone number or JID) is the owner's own chat,
# using the linked device's phone-number JID and LID from the bridge's store.
is_self() {
  local r="$1" user server own pn lid
  own=$(sqlite3 "file:$WA_HOME/whatsapp.db?mode=ro" "SELECT jid || ' ' || COALESCE(lid, '') FROM whatsmeow_device LIMIT 1" 2>/dev/null) || return 1
  read -r pn lid <<<"$own"
  pn=${pn%%@*}; pn=${pn%%:*}; pn=${pn%%.*}
  lid=${lid%%@*}; lid=${lid%%:*}; lid=${lid%%.*}
  if [[ "$r" == *@* ]]; then user=${r%%@*}; server=${r#*@}; else user=$r; server=s.whatsapp.net; fi
  user=${user%%:*}
  [ -n "$user" ] || return 1
  { [ -n "$pn" ] && [ "$server" = s.whatsapp.net ] && [ "$user" = "$pn" ]; } && return 0
  { [ -n "$lid" ] && [ "$server" = lid ] && [ "$user" = "$lid" ]; } && return 0
  return 1
}

# clean makes untrusted text safe to show in a prompt: no control or text-direction
# characters, at most 80 characters. Chat names are chosen by chat members.
clean='def clean: explode | map(select(. >= 32 and (. < 127 or . > 159)
  and ((. >= 8206 and . <= 8207) or (. >= 8234 and . <= 8238) or (. >= 8294 and . <= 8297) | not)))
  | implode | .[:80];'

# chat_label names a recipient (phone number or JID): "Name" (jid) from the bridge's
# store, or the JID alone. Anything outside JID characters is never looked up.
chat_label() {
  local r="$1" user server name
  [[ "$r" =~ ^[0-9A-Za-z@.:_-]+$ ]] || { echo "an unreadable recipient"; return; }
  if [[ "$r" == *@* ]]; then user=${r%%@*}; server=${r#*@}; else user=$r; server=s.whatsapp.net; fi
  user=${user%%:*}
  name=$(sqlite3 -json "file:$WA_HOME/messages.db?mode=ro" \
    "SELECT name FROM chats WHERE jid = '$user@$server' LIMIT 1" 2>/dev/null |
    jq -r "$clean"' .[0].name // "" | clean' 2>/dev/null)
  if [ -n "$name" ]; then echo "\"$name\" ($user@$server)"; else echo "$user@$server (a chat this machine hasn't seen)"; fi
}

tool=$(jq -r '.tool_name // ""' <<<"$input")
case "$tool" in
  *__send_message|*__send_file|*__send_audio_message)
    recipient=$(jq -r '.tool_input.recipient // ""' <<<"$input")
    is_self "$recipient" && decide allow "Sending to your own chat"
    [ -x "$WA_HOME/app/wa-approve" ] && decide allow "WhatsApp: the bridge asks for your Touch ID before sending to $(chat_label "$recipient")"
    decide ask "WhatsApp: send to $(chat_label "$recipient")? Check the chat and the text."
    ;;
  *__add_chat)
    chat=$(jq -r "$clean"' .tool_input.chat // "" | clean' <<<"$input")
    case "$chat" in
      *@*) label=$(chat_label "$chat") ;;
      me|myself|self) label="your own \"Message yourself\" chat" ;;
      *) label="the chat named \"$chat\"" ;;
    esac
    decide ask "WhatsApp: start capturing ${label}? Its messages will be stored on every machine and it can be sent to."
    ;;
  *__download_media)
    decide allow "Download into ~/.whatsapp-mcp/media"
    ;;
esac
exit 0
