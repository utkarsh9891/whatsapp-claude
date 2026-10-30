package main

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waAdv"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// TestWriteFixture writes messages.db through the real MessageStore so the
// Python digest tests read exactly what the bridge writes. Runs only when
// FIXTURE_HOME is set (see server/tests).
func TestWriteFixture(t *testing.T) {
	home := os.Getenv("FIXTURE_HOME")
	if home == "" {
		t.Skip("FIXTURE_HOME not set")
	}
	t.Setenv("WA_HOME", home)
	if err := ensureDirs(); err != nil {
		t.Fatal(err)
	}
	s, err := NewMessageStore()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ist := time.FixedZone("IST", 5*3600+1800)
	now := time.Now().In(ist).Truncate(time.Second)
	ago := func(h float64) time.Time { return now.Add(-time.Duration(h * float64(time.Hour))) }
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}

	must(s.StoreChat("100@g.us", "Field Ops North", ago(1)))
	must(s.StoreChat("200@g.us", "Field Ops South", ago(2)))
	must(s.StoreChat("300@g.us", "Not Allowlisted Busy", ago(0.5)))
	must(s.StoreChat("400@g.us", "Old Group", ago(100)))

	msg := func(id, chat, sender, text string, at time.Time, me bool) {
		must(s.StoreMessage(id, chat, sender, text, at, me, "", "", "", nil, nil, nil, 0))
	}
	// North: I replied, then two new messages, one mentioning me and one with a keyword
	msg("n1", "100@g.us", "911111", "morning all", ago(5), false)
	msg("n2", "100@g.us", "919999", "on it", ago(4), true)
	msg("n3", "100@g.us", "912222", "@919999 merchant onboarding failed for 3 leads", ago(2), false)
	msg("n4", "100@g.us", "913333", "app is down at my location", ago(1), false)
	// South: never replied, one old message outside the window
	msg("s0", "200@g.us", "914444", "last week stuff", ago(48), false)
	msg("s1", "200@g.us", "914444", "all good here", ago(2), false)
	must(s.StoreMessage("s2", "200@g.us", "915555", "", ago(1.5), false, "image", "image_x.jpg", "", nil, nil, nil, 0))
}

// TestWriteLookupFixture writes messages.db through the real MessageStore and whatsapp.db through
// whatsmeow's own store (device, contacts, push names, LID map), for the walookup tests. Runs only
// when LOOKUP_FIXTURE_HOME is set (see server/tests/test_walookup.py).
func TestWriteLookupFixture(t *testing.T) {
	home := os.Getenv("LOOKUP_FIXTURE_HOME")
	if home == "" {
		t.Skip("LOOKUP_FIXTURE_HOME not set")
	}
	t.Setenv("WA_HOME", home)
	if err := ensureDirs(); err != nil {
		t.Fatal(err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	s, err := NewMessageStore()
	must(err)
	defer s.Close()
	now := time.Now().Truncate(time.Second)
	ago := func(h float64) time.Time { return now.Add(-time.Duration(h * float64(time.Hour))) }

	ctx := context.Background()
	container, err := sqlstore.New(ctx, "sqlite3", "file:"+waPath("whatsapp.db")+"?_foreign_keys=on", waLog.Noop)
	must(err)
	device := container.NewDevice()
	device.ID = &types.JID{User: "919000000001", Device: 1, Server: types.DefaultUserServer}
	device.Account = &waAdv.ADVSignedDeviceIdentity{Details: []byte{}, AccountSignature: make([]byte, 64), AccountSignatureKey: make([]byte, 32), DeviceSignature: make([]byte, 64)}
	must(container.PutDevice(ctx, device))

	pn := func(u string) types.JID { return types.NewJID(u, types.DefaultUserServer) }
	lid := func(u string) types.JID { return types.NewJID(u, types.HiddenUserServer) }
	// the implementation takes (first, full), whatever the interface names them
	saved := func(j types.JID, full string) { must(device.Contacts.PutContactName(ctx, j, "", full)) }
	push := func(j types.JID, name string) {
		_, _, err := device.Contacts.PutPushName(ctx, j, name)
		must(err)
	}

	// Meera: saved under her number, chatting under her LID (the newer chat)
	must(device.LIDs.PutLIDMapping(ctx, lid("100000000000001"), pn("911234567890")))
	saved(pn("911234567890"), "Meera")
	push(lid("100000000000001"), "Meera S")
	must(s.StoreChat("911234567890@s.whatsapp.net", "911234567890", ago(2000)))
	must(s.StoreChat("100000000000001@lid", "100000000000001", ago(1)))
	// Panda Po: saved, LID chat only
	must(device.LIDs.PutLIDMapping(ctx, lid("100000000000002"), pn("917000000002")))
	saved(pn("917000000002"), "Panda 🐼 Po")
	must(s.StoreChat("100000000000002@lid", "100000000000002", ago(3)))
	// a name that starts with "panda", known only by the name he chose
	push(pn("917000000003"), "Guddu Pandawa")
	must(s.StoreChat("917000000003@s.whatsapp.net", "~Guddu Pandawa", ago(5)))
	// contacts with no chat; two saved with the same name
	saved(pn("917000000004"), "Vijay Pandey - Police")
	saved(pn("917000000005"), "Ravi")
	saved(pn("917000000006"), "Ravi")
	// groups
	must(s.StoreChat("100@g.us", "Field Ops North", ago(1)))
	must(s.StoreChat("200@g.us", "Field Ops South", ago(2)))
	must(s.StoreChat("300@g.us", "Claude pings", ago(4)))
}
