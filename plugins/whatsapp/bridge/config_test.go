package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("WA_HOME", dir)
	cfgMu.Lock()
	cfg, cfgMtime = Config{}, time.Time{}
	cfgMu.Unlock()
	if err := ensureDirs(); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeConfig(t *testing.T, body string) {
	t.Helper()
	path := waPath("config.json")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	// Force a distinct mtime so the reload is observed.
	future := time.Now().Add(time.Duration(len(body)) * time.Second)
	os.Chtimes(path, future, future)
}

func TestIsAllowed(t *testing.T) {
	setHome(t)
	if isAllowed("1@g.us") {
		t.Fatal("no config must allow nothing")
	}
	writeConfig(t, `{"allow_chats":["1@g.us"]}`)
	if !isAllowed("1@g.us") || isAllowed("2@g.us") || isAllowed("91@s.whatsapp.net") {
		t.Fatal("allowlist not applied")
	}
	writeConfig(t, `{"allow_chats":[],"allow_direct_chats":true}`)
	if isAllowed("1@g.us") || !isAllowed("91@s.whatsapp.net") || !isAllowed("12345@lid") || isAllowed("status@broadcast") {
		t.Fatal("allow_direct_chats must cover 1:1 chats by phone number and LID, and nothing else")
	}
	writeConfig(t, `{not json`)
	if !isAllowed("91@s.whatsapp.net") {
		t.Fatal("invalid config must keep the previous one")
	}
}

func TestEnsureDirsPrivate(t *testing.T) {
	dir := setHome(t)
	for _, d := range []string{dir, filepath.Join(dir, "media"), filepath.Join(dir, "outbox")} {
		st, err := os.Stat(d)
		if err != nil || st.Mode().Perm() != 0700 {
			t.Fatalf("%s: want 0700, got %v (%v)", d, st.Mode().Perm(), err)
		}
	}
}

func TestSafeName(t *testing.T) {
	for in, want := range map[string]string{
		"../../.zshrc":         ".zshrc",
		"/etc/passwd":          "passwd",
		"..":                   "file",
		"":                     "file",
		"a b;rm -rf.pdf":       "a_b_rm_-rf.pdf",
		"120363@g.us":          "120363@g.us",
		"91:12@s.whatsapp.net": "91_12@s.whatsapp.net",
	} {
		if got := safeName(in); got != want {
			t.Errorf("safeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSendablePath(t *testing.T) {
	dir := setHome(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	file := func(rel string) string {
		p := filepath.Join(home, rel)
		os.MkdirAll(filepath.Dir(p), 0700)
		os.WriteFile(p, []byte("x"), 0600)
		return p
	}
	ok := func(label, p string) {
		t.Helper()
		if _, err := sendablePath(p); err != nil {
			t.Errorf("%s: rejected: %v", label, err)
		}
	}
	bad := func(label, p string) {
		t.Helper()
		if _, err := sendablePath(p); err == nil {
			t.Errorf("%s: accepted %s", label, p)
		}
	}

	outbox := filepath.Join(dir, "outbox", "ok.png")
	os.WriteFile(outbox, []byte("x"), 0600)
	ok("outbox", outbox)
	ok("Downloads", file("Downloads/report.pdf"))
	ok("Desktop nested", file("Desktop/a/b/photo.jpg"))
	ok("Documents", file("Documents/notes.txt"))
	ok("Pictures", file("Pictures/x.png"))

	key := file(".ssh/id_rsa")
	bad("~/.ssh", key)
	bad("home root", file("plain.txt"))
	bad("project dir", file("projects/app/main.go"))
	bad("hidden folder inside Downloads", file("Downloads/.secret/a.pdf"))
	bad("hidden file inside Documents", file("Documents/.env"))
	bad("dot-dot out of Downloads", filepath.Join(home, "Downloads", "..", ".ssh", "id_rsa"))
	bad("WA token", filepath.Join(dir, "token"))
	bad("a folder", filepath.Join(home, "Downloads"))
	bad("missing file", filepath.Join(home, "Downloads", "nope.pdf"))
	link := filepath.Join(home, "Downloads", "innocent.pdf")
	os.Symlink(key, link)
	bad("symlink in Downloads to ~/.ssh", link)

	writeConfig(t, `{"send_from":["~/.config","relative/dir","~/Work"]}`)
	file(".config/x.txt")
	bad("hidden configured folder ignored", filepath.Join(home, ".config", "x.txt"))
	bad("defaults replaced by send_from", file("Downloads/r2.pdf"))
	ok("configured folder", file("Work/deck.pdf"))
	ok("outbox always allowed", outbox)
}

func TestLoadTokenStableAndPrivate(t *testing.T) {
	setHome(t)
	a, err := loadToken()
	if err != nil || len(a) != 64 {
		t.Fatalf("token %q err %v", a, err)
	}
	b, _ := loadToken()
	if a != b {
		t.Fatal("token not reused")
	}
	if st, _ := os.Stat(waPath("token")); st.Mode().Perm() != 0600 {
		t.Fatalf("token perms %v", st.Mode().Perm())
	}
}

func TestRequireAuth(t *testing.T) {
	h := requireAuth("secret", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	cases := []struct {
		auth, ctype string
		want        int
	}{
		{"", "application/json", http.StatusUnauthorized},
		{"Bearer wrong", "application/json", http.StatusUnauthorized},
		{"Bearer secret", "text/plain", http.StatusUnsupportedMediaType},
		{"Bearer secret", "application/json", http.StatusOK},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodPost, "/api/send", nil)
		if c.auth != "" {
			r.Header.Set("Authorization", c.auth)
		}
		r.Header.Set("Content-Type", c.ctype)
		w := httptest.NewRecorder()
		h(w, r)
		if w.Code != c.want {
			t.Errorf("auth=%q ctype=%q: got %d want %d", c.auth, c.ctype, w.Code, c.want)
		}
	}
}

func TestWriteQRPNG(t *testing.T) {
	setHome(t)
	path, err := writeQRPNG("2@abc,def,ghi")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if len(data) < 8 || string(data[1:4]) != "PNG" {
		t.Fatal("not a PNG")
	}
}
