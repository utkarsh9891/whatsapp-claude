package main

// Armed sends: the owner approves a message pattern for one chat with Touch ID ahead of time,
// and text sends to that chat that fit it go out without asking, until its uses or its window
// run out. Held in memory only, so no file write can create one. ADR 2026-10-30-032c66.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

const (
	maxPatternLen = 150 // the prompt shows the whole pattern
	maxArmTimes   = 20
	maxArmFor     = 24 * time.Hour
	defaultArmFor = 2 * time.Hour
)

// slotKinds are what a slot may hold: the regexp for one value, and how the prompt describes it.
var slotKinds = map[string]struct{ re, says string }{
	"num":  {`[0-9][0-9.,]{0,19}`, "a number"},
	"id":   {`[A-Za-z0-9][A-Za-z0-9._:/#-]{0,63}`, "a short ID"},
	"url":  {`https://[^\s]{1,192}`, "an https link"},
	"text": {`[^\n]{1,100}`, "any line of up to 100 characters"},
}

// compilePattern turns a pattern into a regexp that must match a message's whole text, and
// lists its slot kinds in order of first use. It refuses a pattern the prompt can't show
// whole, one with an unknown slot, and one with no fixed letter or digit.
func compilePattern(pattern string) (*regexp.Regexp, []string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, nil, errors.New("empty pattern")
	}
	shown := strings.ReplaceAll(pattern, "\n", " ")
	if promptSafe(shown, maxPatternLen) != shown {
		return nil, nil, fmt.Errorf("a pattern must be at most %d characters, without control or text-direction characters, so the prompt shows all of it", maxPatternLen)
	}
	var re, fixed strings.Builder
	var kinds []string
	re.WriteString(`\A`)
	for rest := pattern; rest != ""; {
		switch {
		case strings.HasPrefix(rest, "{{"), strings.HasPrefix(rest, "}}"):
			re.WriteString(regexp.QuoteMeta(rest[:1]))
			fixed.WriteString(rest[:1])
			rest = rest[2:]
		case rest[0] == '{':
			end := strings.IndexByte(rest, '}')
			if end < 0 {
				return nil, nil, errors.New("a { without a closing }; write {{ for a literal brace")
			}
			name := rest[1:end]
			kind, ok := slotKinds[name]
			if !ok {
				return nil, nil, fmt.Errorf("unknown slot {%s}; use {num}, {id}, {url} or {text}", promptSafe(name, 20))
			}
			re.WriteString("(?:" + kind.re + ")")
			if !contains(kinds, name) {
				kinds = append(kinds, name)
			}
			rest = rest[end+1:]
		case rest[0] == '}':
			return nil, nil, errors.New("a } without an opening {; write }} for a literal brace")
		default:
			next := strings.IndexAny(rest, "{}")
			if next < 0 {
				next = len(rest)
			}
			re.WriteString(regexp.QuoteMeta(rest[:next]))
			fixed.WriteString(rest[:next])
			rest = rest[next:]
		}
	}
	re.WriteString(`\z`)
	if !strings.ContainsFunc(fixed.String(), func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
		return nil, nil, errors.New("a pattern needs some fixed text, not only slots")
	}
	compiled, err := regexp.Compile(re.String())
	return compiled, kinds, err
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// armedSend is one pattern the owner approved for one chat.
type armedSend struct {
	ID      string
	JID     string
	Pattern string
	re      *regexp.Regexp
	Left    int
	Until   time.Time
}

// armed holds the armed patterns, in memory only.
var armed struct {
	sync.Mutex
	list []*armedSend
}

// now is the clock; tests replace it.
var now = time.Now

// newArmed checks a request to arm and returns the pattern to add, and its slot kinds for the prompt.
func newArmed(jid, pattern, forStr string, times int) (*armedSend, []string, error) {
	re, kinds, err := compilePattern(pattern)
	if err != nil {
		return nil, nil, err
	}
	if times == 0 {
		times = 1
	}
	if times < 1 || times > maxArmTimes {
		return nil, nil, fmt.Errorf("times must be 1 to %d", maxArmTimes)
	}
	d := defaultArmFor
	if forStr != "" {
		if d, err = time.ParseDuration(forStr); err != nil {
			return nil, nil, fmt.Errorf("for: %q is not a duration like 90m or 2h", promptSafe(forStr, 20))
		}
	}
	if d <= 0 || d > maxArmFor {
		return nil, nil, fmt.Errorf("for must be more than 0 and at most %s", formatFor(maxArmFor))
	}
	var id [3]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, nil, err
	}
	return &armedSend{ID: hex.EncodeToString(id[:]), JID: jid, Pattern: strings.TrimSpace(pattern), re: re,
		Left: times, Until: now().Add(d).Truncate(time.Second)}, kinds, nil
}

// formatFor is a window for a prompt: "2 h", "45 min", "1 h 30 min".
func formatFor(d time.Duration) string {
	h, m := int(d.Hours()), int(d.Minutes())%60
	switch {
	case h == 0:
		return fmt.Sprintf("%d min", m)
	case m == 0:
		return fmt.Sprintf("%d h", h)
	default:
		return fmt.Sprintf("%d h %d min", h, m)
	}
}

// armReason is the text of the arming prompt, after "WhatsApp for Claude is trying to".
func armReason(chatName, chatJID, pattern string, kinds []string, times int, d time.Duration, adding bool) string {
	count := "once"
	if times > 1 {
		count = fmt.Sprintf("up to %d times", times)
	}
	reason := fmt.Sprintf("send messages like %q to %s without asking, %s in the next %s",
		promptSafe(strings.ReplaceAll(pattern, "\n", " "), maxPatternLen), chatTarget(chatName, chatJID), count, formatFor(d))
	if len(kinds) > 0 {
		says := make([]string, len(kinds))
		for i, k := range kinds {
			says[i] = fmt.Sprintf("{%s} is %s", k, slotKinds[k].says)
		}
		reason += " (" + strings.Join(says, ", ") + ")"
	}
	if adding {
		reason += ", and start saving its messages"
	}
	return reason
}

// addArmed adds an approved pattern.
func addArmed(a *armedSend) {
	armed.Lock()
	defer armed.Unlock()
	armed.list = append(armed.list, a)
}

// pruneArmed drops expired and used-up patterns; the caller holds the lock.
func pruneArmed() {
	t := now()
	kept := armed.list[:0]
	for _, a := range armed.list {
		if a.Left > 0 && t.Before(a.Until) {
			kept = append(kept, a)
		}
	}
	armed.list = kept
}

// useArmed takes one use of the first live pattern for jid that the whole message fits. release
// gives the use back (the send then failed). forChat reports that something is armed for jid,
// whether or not the message fits.
func useArmed(jid, message string) (release func(), forChat bool) {
	armed.Lock()
	defer armed.Unlock()
	pruneArmed()
	for _, a := range armed.list {
		if a.JID != jid {
			continue
		}
		forChat = true
		if a.re.MatchString(message) {
			a.Left--
			return func() {
				armed.Lock()
				defer armed.Unlock()
				a.Left++
				if a.Left == 1 && !containsArmed(a) {
					armed.list = append(armed.list, a)
				}
			}, true
		}
	}
	return nil, forChat
}

func containsArmed(a *armedSend) bool {
	for _, x := range armed.list {
		if x == a {
			return true
		}
	}
	return false
}

// disarm drops the patterns with this id, or for this chat, or all with neither; it returns how many.
func disarm(id, jid string) int {
	armed.Lock()
	defer armed.Unlock()
	pruneArmed()
	kept, n := armed.list[:0], 0
	for _, a := range armed.list {
		if (id == "" || a.ID == id) && (jid == "" || a.JID == jid) {
			n++
			continue
		}
		kept = append(kept, a)
	}
	armed.list = kept
	return n
}

// ArmRequest is the body of POST /api/arm and /api/disarm.
type ArmRequest struct {
	Recipient string `json:"recipient,omitempty"`
	Pattern   string `json:"pattern,omitempty"`
	For       string `json:"for,omitempty"`
	Times     int    `json:"times,omitempty"`
	ID        string `json:"id,omitempty"`
}

// ArmedInfo describes one armed pattern in a response.
type ArmedInfo struct {
	ID      string `json:"id"`
	Chat    string `json:"chat"`
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
	Left    int    `json:"left"`
	Until   string `json:"until"`
}

// ArmResponse is the reply of the arm endpoints.
type ArmResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Armed   []ArmedInfo `json:"armed,omitempty"`
}

func armInfo(store *MessageStore, a *armedSend) ArmedInfo {
	return ArmedInfo{ID: a.ID, Chat: a.JID, Name: store.chatName(a.JID), Pattern: a.Pattern, Left: a.Left,
		Until: a.Until.Format(time.RFC3339)}
}

// armSend checks and arms a pattern after the owner's Touch ID.
func armSend(client *whatsmeow.Client, store *MessageStore, req ArmRequest) (*armedSend, error) {
	if !client.IsConnected() {
		return nil, errors.New("not connected to WhatsApp; a send now would fail too")
	}
	chat, err := parseRecipient(req.Recipient)
	if err != nil {
		return nil, err
	}
	chat = chat.ToNonAD()
	ownPN, ownLID := ownJIDs(client)
	if isSelfChat(chat, ownPN, ownLID) {
		return nil, errors.New("sends to your own chat need no arming")
	}
	if isNotifyChat(chat.String()) {
		return nil, errors.New("sends to a notification chat need no arming")
	}
	a, kinds, err := newArmed(chat.String(), req.Pattern, req.For, req.Times)
	if err != nil {
		return nil, err
	}
	reason := armReason(store.chatName(a.JID), a.JID, a.Pattern, kinds, a.Left, a.Until.Sub(now()).Round(time.Minute), !isAllowed(a.JID))
	if err := approver(reason); err != nil {
		return nil, err
	}
	addArmed(a)
	return a, nil
}

// registerArmHandlers serves /api/arm, /api/armed and /api/disarm.
func registerArmHandlers(client *whatsmeow.Client, store *MessageStore, token string) {
	reply := func(w http.ResponseWriter, status int, resp ArmResponse) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(resp)
	}
	handle := func(path string, fn func(ArmRequest) (int, ArmResponse)) {
		http.HandleFunc(path, requireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				reply(w, http.StatusMethodNotAllowed, ArmResponse{Message: "method not allowed"})
				return
			}
			var req ArmRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				reply(w, http.StatusBadRequest, ArmResponse{Message: "invalid request"})
				return
			}
			status, resp := fn(req)
			reply(w, status, resp)
		}))
	}
	handle("/api/arm", func(req ArmRequest) (int, ArmResponse) {
		a, err := armSend(client, store, req)
		switch {
		case errors.Is(err, errNotApproved):
			return http.StatusForbidden, ArmResponse{Message: err.Error()}
		case err != nil:
			return http.StatusBadRequest, ArmResponse{Message: err.Error()}
		}
		fmt.Printf("Armed %s for %s (%d uses, until %s)\n", a.ID, a.JID, a.Left, a.Until.Format(time.RFC3339))
		return http.StatusOK, ArmResponse{Success: true, Armed: []ArmedInfo{armInfo(store, a)},
			Message: fmt.Sprintf("armed %s: %d message(s) to %s until %s", a.ID, a.Left,
				chatTarget(store.chatName(a.JID), a.JID), a.Until.Format("15:04"))}
	})
	handle("/api/armed", func(ArmRequest) (int, ArmResponse) {
		armed.Lock()
		defer armed.Unlock()
		pruneArmed()
		resp := ArmResponse{Success: true, Message: fmt.Sprintf("%d armed", len(armed.list))}
		for _, a := range armed.list {
			resp.Armed = append(resp.Armed, armInfo(store, a))
		}
		return http.StatusOK, resp
	})
	handle("/api/disarm", func(req ArmRequest) (int, ArmResponse) {
		jid := ""
		if req.Recipient != "" {
			chat, err := parseRecipient(req.Recipient)
			if err != nil {
				return http.StatusBadRequest, ArmResponse{Message: err.Error()}
			}
			jid = chat.ToNonAD().String()
		}
		return http.StatusOK, ArmResponse{Success: true, Message: fmt.Sprintf("disarmed %d", disarm(req.ID, jid))}
	})
}

// parseRecipient is a recipient as a JID: given as one, or a phone number for a 1:1 chat.
func parseRecipient(recipient string) (types.JID, error) {
	if recipient == "" {
		return types.JID{}, errors.New("recipient is required")
	}
	if !strings.Contains(recipient, "@") {
		return types.JID{User: recipient, Server: types.DefaultUserServer}, nil
	}
	jid, err := types.ParseJID(recipient)
	if err != nil {
		return types.JID{}, fmt.Errorf("Error parsing JID: %v", err)
	}
	return jid, nil
}
