package main

import (
	"database/sql"
	"reflect"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"google.golang.org/protobuf/proto"

	waProto "go.mau.fi/whatsmeow/binary/proto"
)

func newTestStore(t *testing.T) *MessageStore {
	t.Helper()
	t.Setenv("WA_HOME", t.TempDir())
	if err := ensureDirs(); err != nil {
		t.Fatal(err)
	}
	s, err := NewMessageStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAnchorKeepsNewest(t *testing.T) {
	s := newTestStore(t)
	if _, _, _, err := s.GetAnchor("100@g.us"); err != sql.ErrNoRows {
		t.Fatalf("empty store: err = %v, want ErrNoRows", err)
	}
	t0 := time.Unix(1_800_000_000, 0)
	for _, a := range []struct {
		id string
		me bool
		at time.Time
	}{
		{"m2", true, t0.Add(time.Minute)},
		{"m1", false, t0},              // older: must not replace m2
		{"", false, t0.Add(time.Hour)}, // no ID: ignored
		{"m3", false, time.Time{}},     // no time: ignored
	} {
		if err := s.StoreAnchor("100@g.us", a.id, a.me, a.at); err != nil {
			t.Fatal(err)
		}
	}
	id, me, at, err := s.GetAnchor("100@g.us")
	if err != nil || id != "m2" || !me || !at.Equal(t0.Add(time.Minute)) {
		t.Fatalf("anchor = %q %v %v %v, want m2 true %v", id, me, at, err, t0.Add(time.Minute))
	}
	if err := s.StoreAnchor("100@g.us", "m4", false, t0.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if id, _, _, _ := s.GetAnchor("100@g.us"); id != "m4" {
		t.Fatalf("newer message: anchor = %q, want m4", id)
	}
}

func TestAddedChats(t *testing.T) {
	got := addedChats([]string{"a@g.us", "b@g.us"}, []string{"a@g.us", "c@g.us", "b@g.us", "d@lid"})
	if want := []string{"c@g.us", "d@lid"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("addedChats = %v, want %v", got, want)
	}
	if got := addedChats([]string{"a@g.us"}, []string{"a@g.us"}); got != nil {
		t.Fatalf("nothing added: %v", got)
	}
}

// An on-demand answer is old history: it must not move chat times or replay #wa commands (security model T13).
func TestHistoryRoles(t *testing.T) {
	if u, c := historyRoles(waHistorySync.HistorySync_ON_DEMAND); u || c {
		t.Fatalf("ON_DEMAND: updateChats=%v applyCommands=%v, want false false", u, c)
	}
	for _, typ := range []waHistorySync.HistorySync_HistorySyncType{
		waHistorySync.HistorySync_INITIAL_BOOTSTRAP, waHistorySync.HistorySync_RECENT, waHistorySync.HistorySync_FULL,
	} {
		if u, c := historyRoles(typ); !u || !c {
			t.Fatalf("%v: updateChats=%v applyCommands=%v, want true true", typ, u, c)
		}
	}
}

func TestStoreHistoryAnchorSkipsEmpty(t *testing.T) {
	s := newTestStore(t)
	hm := func(id string, ts uint64, text string) *waHistorySync.HistorySyncMsg {
		m := &waWeb.WebMessageInfo{Key: &waProto.MessageKey{ID: proto.String(id), FromMe: proto.Bool(false)}, MessageTimestamp: proto.Uint64(ts)}
		if text != "" {
			m.Message = &waProto.Message{Conversation: proto.String(text)}
		}
		return &waHistorySync.HistorySyncMsg{Message: m}
	}
	// newest first: a message with no text or media (e.g. a reaction) is skipped
	storeHistoryAnchor(s, "100@g.us", []*waHistorySync.HistorySyncMsg{hm("r1", 1_800_000_300, ""), hm("t1", 1_800_000_200, "hi"), hm("t0", 1_800_000_100, "older")})
	if id, _, _, err := s.GetAnchor("100@g.us"); err != nil || id != "t1" {
		t.Fatalf("anchor = %q (%v), want t1", id, err)
	}
}

func TestTakeHistoryWait(t *testing.T) {
	historyWaitMu.Lock()
	historyWait["100@g.us"] = true
	historyWaitMu.Unlock()
	if !takeHistoryWait("100@g.us") {
		t.Fatal("waiting chat not reported")
	}
	if takeHistoryWait("100@g.us") || takeHistoryWait("200@g.us") {
		t.Fatal("reported twice, or a chat that wasn't waiting")
	}
}
