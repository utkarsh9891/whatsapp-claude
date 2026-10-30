package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

// A pattern matches only whole messages whose slots hold values of their kind (security model T16).
func TestCompilePattern(t *testing.T) {
	cases := []struct {
		pattern string
		fits    []string
		misses  []string
	}{
		{"Build {id} is green, PR: {url}",
			[]string{"Build 4812 is green, PR: https://github.com/acme/app/pull/77", "Build ONBP-12 is green, PR: https://x.y/z?a=b#c"},
			[]string{"Build 4812 is green, PR: http://x.y", "Build 48 12 is green, PR: https://x.y", "Build 4812 is green, PR: https://x.y and more",
				"oh, Build 4812 is green, PR: https://x.y", "Build 4812 is green, PR: https://x.y\nhttps://evil.example", "Build 4812 is green, PR: "}},
		{"{num} tests passed", []string{"1,204 tests passed", "3.5 tests passed"}, []string{"-1 tests passed", "abc tests passed", "123456789012345678901 tests passed"}},
		{"Done: {text}", []string{"Done: all 3 jobs, no errors", "Done: x"},
			[]string{"Done: " + strings.Repeat("a", 101), "Done: line one\nline two", "Done: "}},
		{"Use {{braces}} for {id}", []string{"Use {braces} for v1.2.3"}, []string{"Use {{braces}} for v1.2.3"}},
		{"two\nlines {id}", []string{"two\nlines a1"}, []string{"two lines a1"}},
		{"Price $5 (approx) [ok] {num}", []string{"Price $5 (approx) [ok] 7"}, []string{"Price 55 (approx) [ok] 7"}},
	}
	for _, c := range cases {
		re, _, err := compilePattern(c.pattern)
		if err != nil {
			t.Fatalf("%q: %v", c.pattern, err)
		}
		for _, m := range c.fits {
			if !re.MatchString(m) {
				t.Errorf("%q should fit %q", m, c.pattern)
			}
		}
		for _, m := range c.misses {
			if re.MatchString(m) {
				t.Errorf("%q should not fit %q", m, c.pattern)
			}
		}
	}
	if _, kinds, _ := compilePattern("{url} {id} then {url} and {text}"); strings.Join(kinds, ",") != "url,id,text" {
		t.Errorf("kinds %v", kinds)
	}
	refused := map[string]string{
		"":                               "empty",
		"   ":                            "empty",
		"{text}":                         "fixed text",
		"{id} - {url} ...":               "fixed text",
		"Hi {name}":                      "unknown slot",
		"Hi {id":                         "without a closing",
		"Hi } there":                     "without an opening",
		strings.Repeat("a", 151):         "at most 150",
		"Hi‮{id}":                        "text-direction",
		"Hi\x1b[31m {id}":                "control",
		"tab\there":                      "control",
		"Hi {" + strings.Repeat("x", 30): "without a closing",
	}
	for p, want := range refused {
		if _, _, err := compilePattern(p); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("compilePattern(%q) = %v, want %q", p, err, want)
		}
	}
	if _, _, err := compilePattern(strings.Repeat("a", 150)); err != nil {
		t.Errorf("150 characters refused: %v", err)
	}
}

func TestArmReason(t *testing.T) {
	cases := []struct {
		name, jid, pattern string
		times              int
		d                  time.Duration
		adding             bool
		want               string
	}{
		{"Asha", "91@s.whatsapp.net", "Build {id} is green: {url}", 1, 2 * time.Hour, false,
			`send messages like "Build {id} is green: {url}" to "Asha" without asking, once in the next 2 h ({id} is a short ID, {url} is an https link)`},
		{"Ops", "1@g.us", "Deploy done", 3, 90 * time.Minute, true,
			`send messages like "Deploy done" to the group "Ops" without asking, up to 3 times in the next 1 h 30 min, and start saving its messages`},
		{"", "91@s.whatsapp.net", "two\nlines {text}", 1, 45 * time.Minute, false,
			`send messages like "two lines {text}" to 91@s.whatsapp.net without asking, once in the next 45 min ({text} is any line of up to 100 characters)`},
	}
	for _, c := range cases {
		_, kinds, err := compilePattern(c.pattern)
		if err != nil {
			t.Fatal(err)
		}
		if got := armReason(c.name, c.jid, c.pattern, kinds, c.times, c.d, c.adding); got != c.want {
			t.Errorf("armReason = %s\nwant          %s", got, c.want)
		}
	}
}

func TestArmRefuses(t *testing.T) {
	bad := []struct {
		forStr string
		times  int
		want   string
	}{
		{"", 21, "times must be"},
		{"", -1, "times must be"},
		{"25h", 1, "at most 24 h"},
		{"-5m", 1, "more than 0"},
		{"soon", 1, "not a duration"},
	}
	for _, b := range bad {
		if _, _, err := newArmed("1@g.us", "Done {id}", b.forStr, b.times); err == nil || !strings.Contains(err.Error(), b.want) {
			t.Errorf("for %q times %d: %v, want %q", b.forStr, b.times, err, b.want)
		}
	}
	a, _, err := newArmed("1@g.us", "  Done {id}  ", "", 0)
	if err != nil || a.Left != 1 || a.Pattern != "Done {id}" || a.Until.Sub(now()) > defaultArmFor || len(a.ID) != 6 {
		t.Errorf("defaults: %+v, %v", a, err)
	}
}

// resetArmed empties the armed list and restores the clock after the test.
func resetArmed(t *testing.T) *time.Time {
	t.Helper()
	clock := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	now = func() time.Time { return clock }
	armed.Lock()
	armed.list = nil
	armed.Unlock()
	t.Cleanup(func() {
		now = time.Now
		armed.Lock()
		armed.list = nil
		armed.Unlock()
	})
	return &clock
}

func arm(t *testing.T, jid, pattern, forStr string, times int) *armedSend {
	t.Helper()
	a, _, err := newArmed(jid, pattern, forStr, times)
	if err != nil {
		t.Fatal(err)
	}
	addArmed(a)
	return a
}

// A use needs the same chat and fitting text; uses and the window run out; a failed send gives its use back (T16).
func TestUseArmed(t *testing.T) {
	clock := resetArmed(t)
	asha, ops := "91@s.whatsapp.net", "1@g.us"
	arm(t, asha, "Build {id} is green", "1h", 2)

	if release, forChat := useArmed(ops, "Build 1 is green"); release != nil || forChat {
		t.Error("used for another chat")
	}
	if release, forChat := useArmed(asha, "Build 1 is green, also see evil.example"); release != nil || !forChat {
		t.Error("used for text that doesn't fit, or forChat not reported")
	}
	release, _ := useArmed(asha, "Build 1 is green")
	if release == nil {
		t.Fatal("fitting text not sent")
	}
	if r, _ := useArmed(asha, "Build 2 is green"); r == nil {
		t.Fatal("second use refused")
	}
	if r, forChat := useArmed(asha, "Build 3 is green"); r != nil || forChat {
		t.Error("third use of a two-use pattern allowed")
	}
	release() // the first send failed: its use comes back
	if r, _ := useArmed(asha, "Build 3 is green"); r == nil {
		t.Error("given-back use not usable")
	}

	arm(t, ops, "Deploy {id} done", "30m", 5)
	*clock = clock.Add(31 * time.Minute)
	if r, forChat := useArmed(ops, "Deploy 7 done"); r != nil || forChat {
		t.Error("expired pattern used")
	}
}

func TestDisarm(t *testing.T) {
	resetArmed(t)
	a := arm(t, "91@s.whatsapp.net", "A {id}", "", 1)
	arm(t, "91@s.whatsapp.net", "B {id}", "", 1)
	arm(t, "1@g.us", "C {id}", "", 1)
	arm(t, "2@g.us", "D {id}", "", 1)
	if n := disarm(a.ID, ""); n != 1 {
		t.Errorf("by id: %d", n)
	}
	if n := disarm("", "91@s.whatsapp.net"); n != 1 {
		t.Errorf("by chat: %d", n)
	}
	if n := disarm("", ""); n != 2 {
		t.Errorf("all: %d", n)
	}
	if r, _ := useArmed("1@g.us", "C x"); r != nil {
		t.Error("disarmed pattern used")
	}
}

// sendApproval: a fitting text send goes out without a prompt; files and misfits ask, and a
// declined misfit says it didn't fit (T4, T16).
func TestSendApprovalArmed(t *testing.T) {
	setHome(t)
	resetArmed(t)
	st := newTestStore(t)
	client := &whatsmeow.Client{Store: &store.Device{}}
	chat := types.NewJID("91", types.DefaultUserServer)
	prompts := 0
	approver = func(string) error { prompts++; return errNotApproved }
	t.Cleanup(func() { approver = runApprover })
	arm(t, chat.String(), "Build {id} is green", "", 1)

	if _, _, err := sendApproval(client, st, chat, "Build 9 is green", "/x/report.pdf", false); err == nil || prompts != 1 {
		t.Errorf("file send used the armed pattern: %v, prompts %d", err, prompts)
	}
	_, _, err := sendApproval(client, st, chat, "something else", "", false)
	if !errors.Is(err, errNotApproved) || !strings.Contains(err.Error(), "doesn't fit") || prompts != 2 {
		t.Errorf("misfit: %v, prompts %d", err, prompts)
	}
	asked, release, err := sendApproval(client, st, chat, "Build 9 is green", "", false)
	if err != nil || !asked || release == nil || prompts != 2 {
		t.Errorf("fitting send: asked %v release %v err %v prompts %d", asked, release != nil, err, prompts)
	}
	if _, _, err := sendApproval(client, st, chat, "Build 10 is green", "", false); err == nil || strings.Contains(err.Error(), "doesn't fit") {
		t.Errorf("used-up pattern: %v", err)
	}
}

// The endpoints sit behind the token and JSON content type (T6), always answer JSON, and arm
// nothing while WhatsApp is disconnected, before any prompt.
func TestArmHandlers(t *testing.T) {
	setHome(t)
	resetArmed(t)
	st := newTestStore(t)
	prompts := 0
	approver = func(string) error { prompts++; return nil }
	t.Cleanup(func() { approver = runApprover })
	registerArmHandlers(&whatsmeow.Client{Store: &store.Device{}}, st, "tok")
	srv := httptest.NewServer(http.DefaultServeMux)
	defer srv.Close()
	post := func(path, body, token string) (int, ArmResponse) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out ArmResponse
		if res.StatusCode != http.StatusUnauthorized {
			if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
				t.Fatalf("%s: reply not JSON: %v", path, err)
			}
		}
		return res.StatusCode, out
	}
	if code, _ := post("/api/arm", `{"recipient":"91@s.whatsapp.net","pattern":"Done {id}"}`, "wrong"); code != http.StatusUnauthorized {
		t.Errorf("wrong token: %d", code)
	}
	if code, out := post("/api/arm", `{"recipient":"91@s.whatsapp.net","pattern":"Done {id}"}`, "tok"); code != http.StatusBadRequest || !strings.Contains(out.Message, "not connected") || prompts != 0 {
		t.Errorf("disconnected arm: %d %q, prompts %d", code, out.Message, prompts)
	}
	if code, out := post("/api/arm", `not json`, "tok"); code != http.StatusBadRequest || out.Success {
		t.Errorf("bad body: %d %+v", code, out)
	}
	arm(t, "91@s.whatsapp.net", "Done {id}", "", 2)
	if code, out := post("/api/armed", `{}`, "tok"); code != http.StatusOK || len(out.Armed) != 1 || out.Armed[0].Left != 2 || out.Armed[0].Pattern != "Done {id}" {
		t.Errorf("armed: %d %+v", code, out)
	}
	if _, out := post("/api/disarm", `{"recipient":"91@s.whatsapp.net"}`, "tok"); out.Message != "disarmed 1" {
		t.Errorf("disarm: %+v", out)
	}
}
