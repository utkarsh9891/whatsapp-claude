package main

// #wa allowlist commands sent by the account owner in their own "Message
// yourself" chat. Every linked device receives them, so the allowlist is the
// same on every machine without any file sync. ADR 2026-10-23-f28675.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

const commandPrefix = "#wa"

// Results of applying a command; each maps to the reaction the bridge sends.
const (
	resultApplied   = "applied"
	resultPending   = "pending"
	resultAmbiguous = "ambiguous"
	resultInvalid   = "invalid"
	resultDuplicate = "duplicate"
)

var resultReaction = map[string]string{
	resultApplied:   "✅",
	resultPending:   "⏳",
	resultAmbiguous: "❓",
	resultInvalid:   "❌",
}

// How many applied command IDs to remember per machine.
const maxAppliedCommands = 500

type selfCommand struct {
	Op  string // add | remove | direct | notify | unnotify
	Arg string
}

// parseSelfCommand reports whether text is a #wa command (isCommand) and, if so,
// whether it is well formed (err == nil).
func parseSelfCommand(text string) (cmd selfCommand, isCommand bool, err error) {
	fields := strings.Fields(text)
	if len(fields) == 0 || !strings.EqualFold(fields[0], commandPrefix) {
		return cmd, false, nil
	}
	if len(fields) < 2 {
		return cmd, true, fmt.Errorf("missing command")
	}
	cmd.Op = strings.ToLower(fields[1])
	cmd.Arg = strings.TrimSpace(strings.Join(fields[2:], " "))
	switch cmd.Op {
	case "add", "remove", "notify", "unnotify":
		if cmd.Arg == "" {
			return cmd, true, fmt.Errorf("%s needs a chat name or JID", cmd.Op)
		}
	case "direct":
		cmd.Arg = strings.ToLower(cmd.Arg)
		if cmd.Arg != "on" && cmd.Arg != "off" {
			return cmd, true, fmt.Errorf("direct needs on or off")
		}
	default:
		return cmd, true, fmt.Errorf("unknown command %q", cmd.Op)
	}
	return cmd, true, nil
}

// isSelfChat reports whether chat is the owner's own chat, by phone-number JID or LID.
func isSelfChat(chat, ownPN, ownLID types.JID) bool {
	if chat.User == "" {
		return false
	}
	if ownPN.User != "" && chat.User == ownPN.User && chat.Server == types.DefaultUserServer {
		return true
	}
	return ownLID.User != "" && chat.User == ownLID.User && chat.Server == types.HiddenUserServer
}

// obeyCommand reports whether a message may act as an allowlist command: only one
// the owner's account sent, in a control chat: the owner's own chat or a notification
// chat (ADR 2026-10-27-fc6f0e). The other member of a notification group can't issue one.
func obeyCommand(fromMe bool, chat, ownPN, ownLID types.JID, notifyChats []string) bool {
	if !fromMe {
		return false
	}
	if isSelfChat(chat, ownPN, ownLID) {
		return true
	}
	for _, j := range notifyChats {
		if chat.Server == types.GroupServer && j == chat.ToNonAD().String() {
			return true
		}
	}
	return false
}

// commandChat is where this machine posts #wa commands: the first notification chat,
// else the owner's own chat.
func commandChat(notifyChats []string, ownPN types.JID) types.JID {
	for _, j := range notifyChats {
		if c, err := types.ParseJID(j); err == nil && c.Server == types.GroupServer {
			return c
		}
	}
	return ownPN
}

// notifyPostChat is where `notify <arg>` from this machine is posted: the group being
// made a notification chat, when arg names exactly one group; else commandChat.
func notifyPostChat(arg string, lookup chatLookup, notifyChats []string, ownPN types.JID) types.JID {
	jids := []string{arg}
	if !isJID(arg) {
		jids = lookup(arg)
	}
	if len(jids) == 1 {
		if c, err := types.ParseJID(jids[0]); err == nil && c.Server == types.GroupServer {
			return c
		}
	}
	return commandChat(notifyChats, ownPN)
}

// chatLookup returns the JIDs of chats whose name equals name (case-insensitive, trimmed).
type chatLookup func(name string) []string

func normName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), " "))
}

// selfAliases name the owner's own chat, which has no name to match.
var selfAliases = map[string]bool{"me": true, "myself": true, "self": true}

func isJID(arg string) bool {
	return strings.Contains(arg, "@") && !strings.ContainsAny(arg, " \t")
}

func addUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

func without(list []string, drop map[string]bool) []string {
	out := list[:0]
	for _, x := range list {
		if !drop[x] {
			out = append(out, x)
		}
	}
	return out
}

// applySelfCommand applies cmd to config.json once per msgID and returns one of the result* constants.
// selfJIDs are the owner's own chat JIDs, used for the me/myself/self alias.
func applySelfCommand(msgID string, cmd selfCommand, lookup chatLookup, selfJIDs []string) (string, error) {
	var jids []string
	self := selfAliases[normName(cmd.Arg)]
	switch {
	case (cmd.Op == "add" || cmd.Op == "remove") && self:
		jids = selfJIDs
	case cmd.Op != "direct" && !self && !isJID(cmd.Arg):
		jids = lookup(cmd.Arg) // outside the config lock: it queries the message store
	}
	return mutateConfig(func(c *Config) string {
		for _, id := range c.AppliedCommands {
			if msgID != "" && id == msgID {
				return resultDuplicate
			}
		}
		result := resultApplied
		switch cmd.Op {
		case "add":
			switch {
			case isJID(cmd.Arg):
				c.AllowChats = addUnique(c.AllowChats, cmd.Arg)
			case self:
				if len(jids) == 0 {
					return resultInvalid // not paired: own JIDs unknown
				}
				for _, j := range jids {
					c.AllowChats = addUnique(c.AllowChats, j)
				}
				c.AllowNames = without(c.AllowNames, selfAliases) // e.g. left pending by an older bridge
			case len(jids) == 1:
				c.AllowChats = addUnique(c.AllowChats, jids[0])
			case len(jids) == 0:
				c.AllowNames = addUnique(c.AllowNames, normName(cmd.Arg))
				result = resultPending
			default:
				result = resultAmbiguous
			}
		case "remove":
			if isJID(cmd.Arg) {
				c.AllowChats = without(c.AllowChats, map[string]bool{cmd.Arg: true})
			} else {
				drop := map[string]bool{}
				for _, j := range jids {
					drop[j] = true
				}
				c.AllowChats = without(c.AllowChats, drop)
				c.AllowNames = without(c.AllowNames, map[string]bool{normName(cmd.Arg): true})
			}
		case "direct":
			c.AllowDirectChats = cmd.Arg == "on"
		case "notify":
			// a group this machine has seen, named exactly or by JID; also allowlisted so replies are stored
			target := ""
			switch {
			case self:
				return resultInvalid // the self-chat sends without asking already
			case isJID(cmd.Arg):
				target = cmd.Arg
			case len(jids) == 1:
				target = jids[0]
			case len(jids) > 1:
				return resultAmbiguous
			default:
				return resultInvalid
			}
			if !strings.HasSuffix(target, "@g.us") {
				return resultInvalid
			}
			c.AllowChats = addUnique(c.AllowChats, target)
			c.NotifyChats = addUnique(c.NotifyChats, target)
		case "unnotify":
			drop := map[string]bool{}
			if isJID(cmd.Arg) {
				drop[cmd.Arg] = true
			}
			for _, j := range jids {
				drop[j] = true
			}
			c.NotifyChats = without(c.NotifyChats, drop)
		}
		if msgID != "" {
			c.AppliedCommands = append(c.AppliedCommands, msgID)
			if n := len(c.AppliedCommands); n > maxAppliedCommands {
				c.AppliedCommands = c.AppliedCommands[n-maxAppliedCommands:]
			}
		}
		return result
	})
}

// resolvePendingName moves a pending allowlist name to allow_chats once a chat with that name is seen.
func resolvePendingName(jid, name string) {
	pending := currentConfig().AllowNames
	if len(pending) == 0 || name == "" {
		return
	}
	n := normName(name)
	match := false
	for _, p := range pending {
		if p == n {
			match = true
			break
		}
	}
	if !match {
		return
	}
	if _, err := mutateConfig(func(c *Config) string {
		c.AllowChats = addUnique(c.AllowChats, jid)
		c.AllowNames = without(c.AllowNames, map[string]bool{n: true})
		return resultApplied
	}); err != nil {
		fmt.Printf("Failed to resolve pending allowlist name: %v\n", err)
	}
}

// lookupChatsByName is the production chatLookup over the message store.
func (store *MessageStore) lookupChatsByName(name string) []string {
	rows, err := store.db.Query("SELECT jid, COALESCE(name, '') FROM chats")
	if err != nil {
		return nil
	}
	defer rows.Close()
	want := normName(name)
	var jids []string
	for rows.Next() {
		var jid, n string
		if rows.Scan(&jid, &n) == nil && normName(n) == want {
			jids = append(jids, jid)
		}
	}
	return jids
}

func ownJIDs(client *whatsmeow.Client) (pn, lid types.JID) {
	if client.Store.ID != nil {
		pn = client.Store.ID.ToNonAD()
	}
	return pn, client.Store.LID
}

// selfChatJIDs are the owner's own chat JIDs (phone number and LID), without device parts.
func selfChatJIDs(client *whatsmeow.Client) []string {
	pn, lid := ownJIDs(client)
	var out []string
	for _, j := range []types.JID{pn, lid.ToNonAD()} {
		if j.User != "" {
			out = append(out, j.String())
		}
	}
	return out
}

// handleSelfCommand applies a #wa command if text is one sent by the owner in
// their own chat. It reports whether text was a command. live controls the reaction.
func handleSelfCommand(client *whatsmeow.Client, store *MessageStore, chat, sender types.JID, fromMe bool, msgID, text string, live bool) bool {
	pn, lid := ownJIDs(client)
	if !obeyCommand(fromMe, chat, pn, lid, currentConfig().NotifyChats) {
		return false
	}
	cmd, isCommand, perr := parseSelfCommand(text)
	if !isCommand {
		return false
	}
	result := resultInvalid
	if perr == nil {
		before := currentConfig().AllowChats
		var err error
		if result, err = applySelfCommand(msgID, cmd, store.lookupChatsByName, selfChatJIDs(client)); err != nil {
			fmt.Printf("Failed to apply allowlist command: %v\n", err)
			return true
		}
		// Replays from the pairing sync (not live) come with history already
		if live && (cmd.Op == "add" || cmd.Op == "notify") {
			requestHistoryForAdded(client, store, before)
		}
	}
	fmt.Printf("Allowlist command %s: %s\n", cmd.Op, result) // the argument is a chat name; keep it out of logs
	if emoji, ok := resultReaction[result]; ok && live && msgID != "" {
		reaction := client.BuildReaction(chat, sender, msgID, emoji)
		if _, err := client.SendMessage(context.Background(), chat, reaction); err != nil {
			fmt.Printf("Failed to react to allowlist command: %v\n", err)
		}
	}
	return true
}

// pendingCommand is a #wa command found in history sync, applied after the sync in time order.
type pendingCommand struct {
	at     time.Time
	chat   types.JID
	sender types.JID
	msgID  string
	text   string
}

func applyHistoryCommands(client *whatsmeow.Client, store *MessageStore, cmds []pendingCommand) {
	sort.Slice(cmds, func(i, j int) bool { return cmds[i].at.Before(cmds[j].at) })
	for _, c := range cmds {
		handleSelfCommand(client, store, c.chat, c.sender, true, c.msgID, c.text, false)
	}
}
