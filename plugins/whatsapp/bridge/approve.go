package main

// Send approval: every send to anyone but the owner waits for the owner's Touch ID,
// whoever calls the API; notification chats send unattended while they can only
// reach the owner. ADR 2026-10-27-17d63a.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// How long a send waits for the owner, a little over the helper's own 120 s.
const approvalTimeout = 130 * time.Second

// A notification chat may have at most this many members, the owner included.
const maxNotifyMembers = 2

var errNotApproved = errors.New("not approved: Touch ID was declined or timed out")

// approver asks the owner to approve reason; nil means approved. Tests replace it.
var approver = runApprover

// approveMu keeps to one prompt at a time.
var approveMu sync.Mutex

// The helper's file name; macOS starts its prompt with it ("WhatsApp for Claude is trying to …").
const approverName = "WhatsApp for Claude"

// approverPath is the helper next to the running bridge binary (installed together).
func approverPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), approverName)
}

func runApprover(reason string) error {
	approveMu.Lock()
	defer approveMu.Unlock()
	path := approverPath()
	if st, err := os.Stat(path); path == "" || err != nil || st.IsDir() {
		return fmt.Errorf("no approval helper at %s; sends to others are refused (reinstall with install.sh)", path)
	}
	ctx, cancel := context.WithTimeout(context.Background(), approvalTimeout)
	defer cancel()
	err := exec.CommandContext(ctx, path, reason).Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &exit) && exit.ExitCode() == 2:
		return errors.New("not sent: Touch ID isn't available (lid closed, or locked out after failed attempts)")
	default:
		return errNotApproved
	}
}

// promptSafe makes untrusted text (member-chosen chat names, message text) safe for
// the system prompt: no control or text-direction characters, at most max runes.
func promptSafe(s string, max int) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			r = ' '
		case r < 32 || (r >= 127 && r <= 159), r == 0x200e, r == 0x200f,
			r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
			continue
		}
		if n == max {
			b.WriteString("…")
			break
		}
		b.WriteRune(r)
		n++
	}
	return strings.TrimSpace(b.String())
}

// chatTarget names a chat for a prompt: `the group "Name"`, `"Name"` for a person, or the JID if no name is known.
func chatTarget(chatName, chatJID string) string {
	name := promptSafe(chatName, 60)
	switch {
	case name == "":
		return chatJID
	case strings.HasSuffix(chatJID, "@"+types.GroupServer):
		return fmt.Sprintf("the group %q", name)
	default:
		return fmt.Sprintf("%q", name)
	}
}

// approvalReason is the text of the Touch ID prompt, which macOS shows after
// "WhatsApp for Claude is trying to".
func approvalReason(chatName, chatJID, message, mediaPath string, adding bool) string {
	what := fmt.Sprintf("%q", promptSafe(message, 150))
	if mediaPath != "" {
		what = fmt.Sprintf("the file %q", promptSafe(filepath.Base(mediaPath), 80))
		if message != "" {
			what += fmt.Sprintf(" with %q", promptSafe(message, 100))
		}
	}
	reason := fmt.Sprintf("send %s to %s", what, chatTarget(chatName, chatJID))
	if adding {
		reason += " and start saving its messages"
	}
	return reason
}

// notifyReason is the prompt for making a chat a notification chat from this machine.
func notifyReason(store *MessageStore, arg string) string {
	target := fmt.Sprintf("the group %q", promptSafe(arg, 60))
	if isJID(arg) {
		target = chatTarget(store.chatName(arg), arg)
	}
	return fmt.Sprintf("let Claude message %s without asking you each time", target)
}

// isNotifyChat reports whether jid is listed in notify_chats.
func isNotifyChat(jid string) bool {
	for _, j := range currentConfig().NotifyChats {
		if j == jid {
			return true
		}
	}
	return false
}

// notifyMembersOK checks that a notification chat can only reach the owner: a group
// with at most maxNotifyMembers members, the owner among them.
func notifyMembersOK(chat types.JID, members []types.GroupParticipant, ownPN, ownLID types.JID) error {
	if chat.Server != types.GroupServer {
		return errors.New("a notification chat must be a group")
	}
	if len(members) > maxNotifyMembers {
		return fmt.Errorf("it has %d members; a notification chat may have at most %d, you included", len(members), maxNotifyMembers)
	}
	for _, m := range members {
		for _, j := range []types.JID{m.JID, m.PhoneNumber, m.LID} {
			if (ownPN.User != "" && j.User == ownPN.User && j.Server == types.DefaultUserServer) ||
				(ownLID.User != "" && j.User == ownLID.User && j.Server == types.HiddenUserServer) {
				return nil
			}
		}
	}
	return errors.New("you are not a member of it")
}

// sendApproval decides whether a send may go out: to the owner's own chat, yes; to a
// notification chat that can only reach the owner, yes; anything else waits for Touch ID.
// asked reports that the owner approved it with Touch ID. adding says the chat isn't
// allowlisted yet, which the prompt mentions.
func sendApproval(client *whatsmeow.Client, store *MessageStore, chat types.JID, message, mediaPath string, adding bool) (asked bool, err error) {
	ownPN, ownLID := ownJIDs(client)
	if isSelfChat(chat, ownPN, ownLID) {
		return false, nil
	}
	if isNotifyChat(chat.String()) {
		info, err := client.GetGroupInfo(context.Background(), chat)
		if err == nil {
			err = notifyMembersOK(chat, info.Participants, ownPN, ownLID)
		}
		if err == nil {
			return false, nil
		}
		fmt.Printf("Notification chat not usable unattended (%v); asking for Touch ID\n", err)
	}
	if err := approver(approvalReason(store.chatName(chat.String()), chat.String(), message, mediaPath, adding)); err != nil {
		return false, err
	}
	return true, nil
}

// syncAddedChat tells the owner's other machines to add jid, by posting "#wa add <jid>" to
// the first notification chat; with none it does nothing rather than write to the
// self-chat (ADR 2026-10-27-fc6f0e). It reports whether it posted.
func syncAddedChat(client *whatsmeow.Client, jid string) bool {
	notify := currentConfig().NotifyChats
	self, _ := ownJIDs(client)
	post := commandChat(notify, self)
	if post.Server != types.GroupServer {
		return false
	}
	text := commandPrefix + " add " + jid
	if _, err := client.SendMessage(context.Background(), post, &waE2E.Message{Conversation: &text}); err != nil {
		fmt.Printf("Failed to sync an added chat to other machines: %v\n", err)
		return false
	}
	return true
}

// allowSentChat adds a chat the owner approved a send to, on this machine, and returns
// the allowlist before the change.
func allowSentChat(jid string) (before []string, err error) {
	_, err = mutateConfig(func(c *Config) string {
		before = append([]string(nil), c.AllowChats...)
		c.AllowChats = addUnique(c.AllowChats, jid)
		return resultApplied
	})
	return before, err
}

// chatName is a chat's stored name, or "" if unknown.
func (store *MessageStore) chatName(jid string) string {
	var name string
	store.db.QueryRow("SELECT COALESCE(name, '') FROM chats WHERE jid = ?", jid).Scan(&name)
	return name
}
