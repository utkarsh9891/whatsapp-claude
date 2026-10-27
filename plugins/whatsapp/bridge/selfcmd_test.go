package main

import (
	"fmt"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestParseSelfCommand(t *testing.T) {
	cases := []struct {
		in        string
		op, arg   string
		isCommand bool
		ok        bool
	}{
		{"#wa add Field Ops North", "add", "Field Ops North", true, true},
		{"  #WA   ADD   Field  Ops ", "add", "Field Ops", true, true},
		{"#wa remove 120@g.us", "remove", "120@g.us", true, true},
		{"#wa direct ON", "direct", "on", true, true},
		{"#wa direct maybe", "", "", true, false},
		{"#wa add", "", "", true, false},
		{"#wa", "", "", true, false},
		{"#wa delete everything", "", "", true, false},
		{"hello #wa add x", "", "", false, false},
		{"#wallet add x", "", "", false, false},
		{"", "", "", false, false},
	}
	for _, c := range cases {
		cmd, isCommand, err := parseSelfCommand(c.in)
		if isCommand != c.isCommand || (err == nil) != (c.ok || !c.isCommand) {
			t.Errorf("%q: isCommand=%v err=%v", c.in, isCommand, err)
			continue
		}
		if c.ok && (cmd.Op != c.op || cmd.Arg != c.arg) {
			t.Errorf("%q: got %+v", c.in, cmd)
		}
	}
}

func TestIsSelfChat(t *testing.T) {
	pn := types.NewJID("919999", types.DefaultUserServer)
	lid := types.NewJID("12345", types.HiddenUserServer)
	for chat, want := range map[types.JID]bool{
		types.NewJID("919999", types.DefaultUserServer): true,
		types.NewJID("12345", types.HiddenUserServer):   true,
		types.NewJID("911111", types.DefaultUserServer): false,
		types.NewJID("919999", types.GroupServer):       false, // same digits, a group
		types.NewJID("12345", types.DefaultUserServer):  false, // LID digits on the PN server
		{}: false,
	} {
		if got := isSelfChat(chat, pn, lid); got != want {
			t.Errorf("isSelfChat(%s) = %v, want %v", chat, got, want)
		}
	}
	if isSelfChat(types.NewJID("", types.DefaultUserServer), types.JID{}, types.JID{}) {
		t.Error("unpaired bridge must not treat anything as self-chat")
	}
}

func lookupFrom(chats map[string]string) chatLookup {
	return func(name string) []string {
		var out []string
		for jid, n := range chats {
			if normName(n) == normName(name) {
				out = append(out, jid)
			}
		}
		return out
	}
}

func TestApplySelfCommand(t *testing.T) {
	setHome(t)
	chats := map[string]string{"1@g.us": "Field Ops North", "2@g.us": "Dup", "3@g.us": "dup"}
	lookup := lookupFrom(chats)
	apply := func(id, text, want string) {
		t.Helper()
		cmd, _, err := parseSelfCommand(text)
		if err != nil {
			t.Fatal(err)
		}
		got, err := applySelfCommand(id, cmd, lookup, []string{"919999@s.whatsapp.net", "12345@lid"})
		if err != nil || got != want {
			t.Fatalf("%s: got %q err %v, want %q", text, got, err, want)
		}
	}

	apply("m1", "#wa add field ops   NORTH", resultApplied)
	if !isAllowed("1@g.us") {
		t.Fatal("name add not applied")
	}
	apply("m1", "#wa add field ops north", resultDuplicate) // same message replayed
	apply("m2", "#wa add Dup", resultAmbiguous)
	if isAllowed("2@g.us") || isAllowed("3@g.us") {
		t.Fatal("ambiguous name must add nothing")
	}
	apply("m3", "#wa add 9@g.us", resultApplied)
	apply("m4", "#wa add Not Seen Yet", resultPending)
	if c := currentConfig(); len(c.AllowNames) != 1 || c.AllowNames[0] != "not seen yet" {
		t.Fatalf("pending name not kept: %+v", c.AllowNames)
	}
	resolvePendingName("7@g.us", "Not  seen YET")
	if c := currentConfig(); !isAllowed("7@g.us") || len(c.AllowNames) != 0 {
		t.Fatalf("pending name not resolved: %+v", c)
	}
	apply("m5", "#wa remove Field Ops North", resultApplied)
	apply("m6", "#wa remove 9@g.us", resultApplied)
	if isAllowed("1@g.us") || isAllowed("9@g.us") {
		t.Fatal("remove not applied")
	}
	apply("m7", "#wa direct on", resultApplied)
	if !isAllowed("91@s.whatsapp.net") {
		t.Fatal("direct on not applied")
	}
	if c := currentConfig(); c.Port != 8080 || len(c.AppliedCommands) != 7 {
		t.Fatalf("config after commands: %+v", c)
	}
}

func TestAppliedCommandsCapped(t *testing.T) {
	setHome(t)
	cmd := selfCommand{Op: "direct", Arg: "on"}
	for i := 0; i < maxAppliedCommands+20; i++ {
		applySelfCommand(string(rune('a'+i%26))+string(rune(i)), cmd, lookupFrom(nil), nil)
	}
	if n := len(currentConfig().AppliedCommands); n != maxAppliedCommands {
		t.Fatalf("applied_commands len %d", n)
	}
}

func TestMutateConfigKeepsHandEdits(t *testing.T) {
	setHome(t)
	writeConfig(t, `{"allow_chats":["5@g.us"],"port":9090}`)
	if _, err := applySelfCommand("x", selfCommand{Op: "add", Arg: "6@g.us"}, lookupFrom(nil), nil); err != nil {
		t.Fatal(err)
	}
	c := currentConfig()
	if c.Port != 9090 || !isAllowed("5@g.us") || !isAllowed("6@g.us") {
		t.Fatalf("hand-edited config lost: %+v", c)
	}
}

func TestCommandsOnlyFromSelf(t *testing.T) {
	pn := types.NewJID("919999", types.DefaultUserServer)
	lid := types.NewJID("12345", types.HiddenUserServer)
	group := types.NewJID("120363", types.GroupServer)
	other := types.NewJID("911111", types.DefaultUserServer)
	notify := []string{"120999@g.us"}
	pings := types.NewJID("120999", types.GroupServer)
	if !obeyCommand(true, pn, pn, lid, nil) || !obeyCommand(true, lid, pn, lid, nil) {
		t.Fatal("owner's own chat must be obeyed")
	}
	if obeyCommand(false, pn, pn, lid, nil) {
		t.Fatal("a message not from me in my chat must be ignored")
	}
	if obeyCommand(true, group, pn, lid, notify) || obeyCommand(true, other, pn, lid, notify) {
		t.Fatal("my #wa messages in other chats must be ignored")
	}
	// ADR 2026-10-27-fc6f0e: notification chats are control chats, for my messages only
	if !obeyCommand(true, pings, pn, lid, notify) {
		t.Fatal("my command in a notification chat must be obeyed")
	}
	if obeyCommand(false, pings, pn, lid, notify) {
		t.Fatal("the other member of a notification chat must not issue commands")
	}
	if obeyCommand(true, types.NewJID("120999", types.DefaultUserServer), pn, lid, notify) {
		t.Fatal("notification chat digits on another server must not count")
	}
}

func TestSelfAlias(t *testing.T) {
	setHome(t)
	self := []string{"919999@s.whatsapp.net", "12345@lid"}
	writeConfig(t, `{"allow_names":["myself"]}`)               // left pending by an earlier "#wa add myself"
	for i, alias := range []string{"me", "MySelf", " self "} { // "me" first: it must also clear the pending "myself"
		got, err := applySelfCommand(fmt.Sprint("a", i), selfCommand{Op: "add", Arg: alias}, lookupFrom(map[string]string{"x@g.us": "me"}), self)
		if err != nil || got != resultApplied {
			t.Fatalf("%q: %q %v", alias, got, err)
		}
	}
	c := currentConfig()
	if i := 0; len(c.AllowNames) != i {
		t.Fatalf("pending alias names left: %+v", c.AllowNames)
	}
	if !isAllowed("919999@s.whatsapp.net") || !isAllowed("12345@lid") || isAllowed("x@g.us") || len(c.AllowChats) != 2 {
		t.Fatalf("alias must add exactly the self JIDs, not a chat named \"me\": %+v", c.AllowChats)
	}
	if len(c.AllowNames) != 0 {
		t.Fatalf("stale pending alias name not cleared: %+v", c.AllowNames)
	}
	if got, _ := applySelfCommand("r", selfCommand{Op: "remove", Arg: "me"}, lookupFrom(nil), self); got != resultApplied || isAllowed("12345@lid") {
		t.Fatal("remove me not applied")
	}
	if got, _ := applySelfCommand("u", selfCommand{Op: "add", Arg: "me"}, lookupFrom(nil), nil); got != resultInvalid {
		t.Fatalf("unpaired bridge must refuse the alias, got %q", got)
	}
}

func TestAddMeClearsPendingMyself(t *testing.T) {
	setHome(t)
	writeConfig(t, `{"allow_names":["myself","field ops east"]}`)
	if got, _ := applySelfCommand("m", selfCommand{Op: "add", Arg: "me"}, lookupFrom(nil), []string{"1@lid"}); got != resultApplied {
		t.Fatal(got)
	}
	if n := currentConfig().AllowNames; len(n) != 1 || n[0] != "field ops east" {
		t.Fatalf("want only the unrelated pending name kept, got %+v", n)
	}
}

func TestCommandChat(t *testing.T) {
	pn := types.NewJID("919999", types.DefaultUserServer)
	if got := commandChat(nil, pn); got != pn {
		t.Fatalf("no notification chat: %v, want the self-chat", got)
	}
	if got := commandChat([]string{"bad", "5@g.us", "6@g.us"}, pn); got.String() != "5@g.us" {
		t.Fatalf("got %v, want the first notification group", got)
	}
	lookup := lookupFrom(map[string]string{"7@g.us": "Todo", "8@g.us": "Twin", "9@g.us": "twin", "91@s.whatsapp.net": "Asha"})
	for arg, want := range map[string]string{
		"7@g.us": "7@g.us", "Todo": "7@g.us", // the group being made a notification chat
		"Twin": "5@g.us", "Asha": "5@g.us", "Never Seen": "5@g.us", "91@s.whatsapp.net": "5@g.us",
	} {
		if got := notifyPostChat(arg, lookup, []string{"5@g.us"}, pn); got.String() != want {
			t.Errorf("notifyPostChat(%q) = %v, want %s", arg, got, want)
		}
	}
}
