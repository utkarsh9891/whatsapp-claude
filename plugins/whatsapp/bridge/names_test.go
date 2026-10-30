package main

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// The owner's saved name beats a business name, which beats the name the person chose, marked ~ (T5).
func TestPickName(t *testing.T) {
	cases := []struct {
		infos []types.ContactInfo
		want  string
	}{
		{nil, ""},
		{[]types.ContactInfo{{PushName: "Meera S"}}, "~Meera S"},
		{[]types.ContactInfo{{PushName: "Mom"}, {FullName: " Meera "}}, "Meera"},
		{[]types.ContactInfo{{FirstName: "Panda"}, {FullName: "Panda Po"}}, "Panda Po"},
		{[]types.ContactInfo{{BusinessName: "Acme Ltd", PushName: "A"}}, "Acme Ltd"},
		{[]types.ContactInfo{{FirstName: "Asha", BusinessName: "Acme"}}, "Asha"},
		{[]types.ContactInfo{{PushName: "  "}}, ""},
	}
	for _, c := range cases {
		if got := pickName(c.infos); got != c.want {
			t.Errorf("pickName(%+v) = %q, want %q", c.infos, got, c.want)
		}
	}
}

func TestNeedsName(t *testing.T) {
	for name, want := range map[string]bool{"": true, "911234567890": true, "100000000000001": true, "+91999": true,
		"~Meera": true, "Meera": false, "Panda 2": false} {
		if got := needsName(name); got != want {
			t.Errorf("needsName(%q) = %v, want %v", name, got, want)
		}
	}
}

// Placeholder names are replaced; saved names and groups are left alone.
func TestRefreshChatNames(t *testing.T) {
	setHome(t)
	st := newTestStore(t)
	at := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	rows := map[string]string{
		"100000000000001@lid":         "100000000000001",
		"911234567890@s.whatsapp.net": "911234567890",
		"91777@s.whatsapp.net":        "~Panda",
		"91888@s.whatsapp.net":        "Asha",
		"91999@s.whatsapp.net":        "91999",
		"1203@g.us":                   "1203",
	}
	for jid, name := range rows {
		if err := st.StoreChat(jid, name, at); err != nil {
			t.Fatal(err)
		}
	}
	names := map[string]string{"100000000000001@lid": "Meera", "911234567890@s.whatsapp.net": "Meera",
		"91777@s.whatsapp.net": "Panda Po", "91888@s.whatsapp.net": "Someone else", "1203@g.us": "Group"}
	n, err := refreshChatNames(st, func(j types.JID) string { return names[j.String()] })
	if err != nil || n != 3 {
		t.Fatalf("renamed %d, err %v; want 3", n, err)
	}
	want := map[string]string{"100000000000001@lid": "Meera", "911234567890@s.whatsapp.net": "Meera",
		"91777@s.whatsapp.net": "Panda Po", "91888@s.whatsapp.net": "Asha", "91999@s.whatsapp.net": "91999", "1203@g.us": "1203"}
	for jid, name := range want {
		if got := st.chatName(jid); got != name {
			t.Errorf("%s named %q, want %q", jid, got, name)
		}
	}
	var last string
	st.db.QueryRow("SELECT last_message_time FROM chats WHERE jid = '91777@s.whatsapp.net'").Scan(&last)
	if last == "" {
		t.Error("renaming lost the chat's time")
	}
}
