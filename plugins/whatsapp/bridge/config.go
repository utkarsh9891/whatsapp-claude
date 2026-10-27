package main

// Local hardening for the vendored bridge: a private data dir, a chat allowlist,
// a bearer token on the REST API, and safe file paths. See ../README.md.

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"rsc.io/qr"
)

// Config is $WA_HOME/config.json. It is re-read whenever the file changes, so
// editing the allowlist needs no bridge restart.
type Config struct {
	// Chat JIDs (e.g. 1203630xxxx@g.us) whose message content is stored and
	// which may be sent to. Every chat's name is stored regardless, so chats
	// can be discovered and then added here.
	AllowChats []string `json:"allow_chats"`
	// Treat every 1:1 chat as allowlisted.
	AllowDirectChats bool `json:"allow_direct_chats"`
	// REST API port, bound to 127.0.0.1 only.
	Port int `json:"port"`
	// Chat names from #wa add commands not yet matched to a chat on this
	// machine; resolved to a JID when a chat with that exact name is seen.
	AllowNames []string `json:"allow_names,omitempty"`
	// IDs of #wa commands already applied here, so replays are no-ops.
	AppliedCommands []string `json:"applied_commands,omitempty"`
	// Folders files may be sent from, besides the outbox; "~/" is the home
	// folder. Unset means defaultSendFrom. ADR 2026-10-24-f10233.
	SendFrom []string `json:"send_from,omitempty"`
	// Groups reserved for messages from Claude: sends to them need no approval
	// while the group can only reach the owner. ADR 2026-10-27-17d63a.
	NotifyChats []string `json:"notify_chats,omitempty"`
}

var defaultSendFrom = []string{"~/Downloads", "~/Desktop", "~/Documents", "~/Pictures"}

var (
	cfgMu    sync.Mutex
	cfg      Config
	cfgMtime time.Time
)

// waHome is the private data dir: $WA_HOME, else ~/.whatsapp-mcp.
func waHome() string {
	if h := os.Getenv("WA_HOME"); h != "" {
		return h
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".whatsapp-mcp")
}

func waPath(parts ...string) string {
	return filepath.Join(append([]string{waHome()}, parts...)...)
}

func ensureDirs() error {
	for _, d := range []string{waPath(), waPath("media"), waPath("outbox")} {
		if err := os.MkdirAll(d, 0700); err != nil {
			return err
		}
		if err := os.Chmod(d, 0700); err != nil {
			return err
		}
	}
	return nil
}

func currentConfig() Config {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	return loadConfigLocked()
}

// loadConfigLocked returns the config, re-reading the file if it changed. cfgMu must be held.
func loadConfigLocked() Config {
	path := waPath("config.json")
	st, err := os.Stat(path)
	if err != nil {
		if cfg.Port == 0 {
			cfg = Config{Port: 8080}
		}
		return cfg
	}
	if st.ModTime().Equal(cfgMtime) {
		return cfg
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		fmt.Printf("config.json is invalid, keeping previous config: %v\n", err)
		return cfg
	}
	if c.Port == 0 {
		c.Port = 8080
	}
	cfg, cfgMtime = c, st.ModTime()
	return cfg
}

// mutateConfig applies fn to the current config and writes it back atomically (0600).
// It returns fn's result.
func mutateConfig(fn func(*Config) string) (string, error) {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	c := loadConfigLocked()
	c.AllowChats = append([]string(nil), c.AllowChats...)
	c.AllowNames = append([]string(nil), c.AllowNames...)
	c.AppliedCommands = append([]string(nil), c.AppliedCommands...)
	c.NotifyChats = append([]string(nil), c.NotifyChats...)
	result := fn(&c)
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", err
	}
	path := waPath("config.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	cfg, cfgMtime = c, st.ModTime()
	return result, nil
}

// isAllowed reports whether a chat's content may be stored or sent to.
func isAllowed(chatJID string) bool {
	c := currentConfig()
	if c.AllowDirectChats && strings.HasSuffix(chatJID, "@s.whatsapp.net") {
		return true
	}
	for _, j := range c.AllowChats {
		if j == chatJID {
			return true
		}
	}
	return false
}

// loadToken returns the REST bearer token, creating $WA_HOME/token (0600) on first run.
func loadToken() (string, error) {
	path := waPath("token")
	if data, err := os.ReadFile(path); err == nil {
		if t := strings.TrimSpace(string(data)); t != "" {
			return t, nil
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	t := hex.EncodeToString(buf)
	return t, os.WriteFile(path, []byte(t+"\n"), 0600)
}

// requireAuth rejects requests without the bearer token or a JSON content type.
// The content-type check stops a web page from POSTing to the API cross-origin.
func requireAuth(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
			return
		}
		next(w, r)
	}
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._@-]`)

// safeName reduces an untrusted name (sender-supplied filename, chat JID) to a
// single path component.
func safeName(s string) string {
	s = unsafeChars.ReplaceAllString(filepath.Base(s), "_")
	if s == "" || s == "." || s == ".." {
		return "file"
	}
	return s
}

// hasHiddenPart reports whether any component of p starts with "." (.ssh, .aws, ...).
func hasHiddenPart(p string) bool {
	for _, c := range strings.Split(filepath.ToSlash(p), "/") {
		if strings.HasPrefix(c, ".") && c != "." && c != ".." {
			return true
		}
	}
	return false
}

// sendFromDirs lists the folders media may be sent from: the outbox, then the
// configured folders. A configured folder that is relative or hidden is ignored.
func sendFromDirs() []string {
	dirs := currentConfig().SendFrom
	if dirs == nil {
		dirs = defaultSendFrom
	}
	home, _ := os.UserHomeDir()
	out := []string{waPath("outbox")}
	for _, d := range dirs {
		if strings.HasPrefix(d, "~/") {
			d = filepath.Join(home, d[2:])
		}
		if !filepath.IsAbs(d) || hasHiddenPart(d) {
			continue
		}
		out = append(out, filepath.Clean(d))
	}
	return out
}

// sendablePath resolves a file to send, which must be inside a send_from folder,
// after symlinks, with no hidden component below that folder.
func sendablePath(p string) (string, error) {
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(resolved); err != nil || st.IsDir() {
		return "", fmt.Errorf("not a file: %s", p)
	}
	dirs := sendFromDirs()
	for _, d := range dirs {
		base, err := filepath.EvalSymlinks(d)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(base, resolved)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			continue
		}
		if hasHiddenPart(rel) {
			return "", fmt.Errorf("refusing a file in a hidden folder: %s", p)
		}
		return resolved, nil
	}
	return "", fmt.Errorf("media must be inside one of: %s", strings.Join(dirs, ", "))
}

// writeQRPNG saves the pairing QR as $WA_HOME/qr.png so it can be opened
// when the bridge isn't attached to a terminal.
func writeQRPNG(code string) (string, error) {
	c, err := qr.Encode(code, qr.L)
	if err != nil {
		return "", err
	}
	c.Scale = 8
	path := waPath("qr.png")
	return path, os.WriteFile(path, c.PNG(), 0600)
}
