package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestPromptSafe(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Field Ops North", "Field Ops North"},
		{"Evil‮Message\nyourself", "EvilMessage yourself"},
		{"a\x00b\x1bc‏d⁦e", "abcde"},
		{"  padded\t", "padded"},
		{strings.Repeat("x", 10), "xxxx…"},
	}
	for _, c := range cases {
		max := 80
		if strings.HasPrefix(c.in, "xxx") {
			max = 4
		}
		if got := promptSafe(c.in, max); got != c.want {
			t.Errorf("promptSafe(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestApprovalReason(t *testing.T) {
	cases := []struct{ name, jid, msg, media, want string }{
		{"Field Ops North", "1@g.us", "fix is live", "",
			`send WhatsApp "fix is live" to "Field Ops North" (1@g.us)`},
		{"", "91@s.whatsapp.net", "hi", "",
			`send WhatsApp "hi" to 91@s.whatsapp.net`},
		{"Ops", "1@g.us", "", "/Users/x/Downloads/report.pdf",
			`send WhatsApp the file "report.pdf" to "Ops" (1@g.us)`},
		{"Ops", "1@g.us", "see this", "/Users/x/Downloads/report.pdf",
			`send WhatsApp the file "report.pdf" with "see this" to "Ops" (1@g.us)`},
	}
	for _, c := range cases {
		if got := approvalReason(c.name, c.jid, c.msg, c.media); got != c.want {
			t.Errorf("approvalReason = %s\nwant             %s", got, c.want)
		}
	}
	long := approvalReason("Ops", "1@g.us", strings.Repeat("a", 500), "")
	if strings.Count(long, "a") > 160 || !strings.Contains(long, "…") {
		t.Errorf("long text not cut: %d chars", len(long))
	}
}

// Notification chats only send unattended while they can only reach the owner (security model T15).
func TestNotifyMembersOK(t *testing.T) {
	pn := types.NewJID("919999", types.DefaultUserServer)
	lid := types.NewJID("12345", types.HiddenUserServer)
	group := types.NewJID("1203", types.GroupServer)
	me := types.GroupParticipant{JID: lid, PhoneNumber: pn, LID: lid}
	bot := types.GroupParticipant{JID: types.NewJID("777", types.HiddenUserServer)}
	other := types.GroupParticipant{JID: types.NewJID("888", types.HiddenUserServer)}
	ok := [][]types.GroupParticipant{{me}, {me, bot}, {{JID: pn}}, {{JID: lid}, bot}}
	for _, m := range ok {
		if err := notifyMembersOK(group, m, pn, lid); err != nil {
			t.Errorf("%v: %v, want ok", m, err)
		}
	}
	if err := notifyMembersOK(group, []types.GroupParticipant{me, bot, other}, pn, lid); err == nil {
		t.Error("three members allowed")
	}
	if err := notifyMembersOK(group, []types.GroupParticipant{bot, other}, pn, lid); err == nil {
		t.Error("group without the owner allowed")
	}
	if err := notifyMembersOK(group, []types.GroupParticipant{{JID: types.NewJID("919999", types.GroupServer)}}, pn, lid); err == nil {
		t.Error("owner's digits on another server counted as the owner")
	}
	if err := notifyMembersOK(types.NewJID("911111", types.DefaultUserServer), nil, pn, lid); err == nil {
		t.Error("a 1:1 chat allowed as a notification chat")
	}
}

// The helper decides; a missing helper or any non-zero exit means no send (security model T4, T14).
func TestRunApprover(t *testing.T) {
	path := approverPath()
	t.Cleanup(func() { os.Remove(path) })
	if err := runApprover("x"); err == nil || !strings.Contains(err.Error(), "no approval helper") {
		t.Fatalf("missing helper: %v, want refusal", err)
	}
	for code, want := range map[int]string{0: "", 1: "not approved", 2: "Touch ID isn't available", 7: "not approved"} {
		script := "#!/bin/sh\n[ \"$1\" = 'send it' ] || exit 9\nexit " + string(rune('0'+code)) + "\n"
		if err := os.WriteFile(path, []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
		err := runApprover("send it")
		if want == "" && err != nil || want != "" && (err == nil || !strings.Contains(err.Error(), want)) {
			t.Errorf("exit %d: err %v, want %q", code, err, want)
		}
	}
	if filepath.Base(path) != "wa-approve" {
		t.Errorf("helper path %s", path)
	}
}

func TestNotifyCommands(t *testing.T) {
	setHome(t)
	lookup := lookupFrom(map[string]string{"5@g.us": "Claude pings", "6@g.us": "Twin", "8@g.us": "twin", "91@s.whatsapp.net": "Asha"})
	apply := func(id, text, want string) {
		t.Helper()
		cmd, _, err := parseSelfCommand(text)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := applySelfCommand(id, cmd, lookup, []string{"919999@s.whatsapp.net"}); err != nil || got != want {
			t.Fatalf("%s: got %q err %v, want %q", text, got, err, want)
		}
	}
	apply("n1", "#wa notify claude PINGS", resultApplied)
	if !isNotifyChat("5@g.us") || !isAllowed("5@g.us") {
		t.Fatal("notify by name: not a notification chat, or not allowlisted")
	}
	apply("n2", "#wa notify Twin", resultAmbiguous)
	apply("n3", "#wa notify Asha", resultInvalid) // 1:1 chats can't be notification chats
	apply("n4", "#wa notify 91@s.whatsapp.net", resultInvalid)
	apply("n5", "#wa notify me", resultInvalid) // the self-chat needs no approval anyway
	apply("n6", "#wa notify Never Seen", resultInvalid)
	apply("n7", "#wa notify 9@g.us", resultApplied)
	if c := currentConfig(); len(c.NotifyChats) != 2 {
		t.Fatalf("notify_chats = %v", c.NotifyChats)
	}
	apply("n8", "#wa unnotify Claude pings", resultApplied)
	apply("n9", "#wa unnotify 9@g.us", resultApplied)
	if c := currentConfig(); len(c.NotifyChats) != 0 || !isAllowed("5@g.us") {
		t.Fatalf("unnotify: notify_chats %v; the chat must stay allowlisted", c.NotifyChats)
	}
}
