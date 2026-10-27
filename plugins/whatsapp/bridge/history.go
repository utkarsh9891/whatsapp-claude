package main

// History on add: when a chat is added by a live command, ask the owner's phone
// for the messages before the chat's latest known message. ADR 2026-10-27-a8ea3b.

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"
)

// How many messages to ask for when a chat is added (whatsmeow recommends 50 per request).
const historyOnAddCount = 50

// StoreAnchor records a chat's latest message ID and time (no content), the
// reference point an on-demand history request needs. Older messages don't
// replace a newer anchor.
func (store *MessageStore) StoreAnchor(chatJID, msgID string, fromMe bool, at time.Time) error {
	if msgID == "" || at.IsZero() {
		return nil
	}
	_, err := store.db.Exec(
		`INSERT INTO chat_anchors (jid, msg_id, from_me, ts) VALUES (?, ?, ?, ?)
		ON CONFLICT(jid) DO UPDATE SET msg_id = excluded.msg_id, from_me = excluded.from_me, ts = excluded.ts
		WHERE excluded.ts >= chat_anchors.ts`,
		chatJID, msgID, fromMe, at.Unix(),
	)
	return err
}

// GetAnchor returns a chat's latest known message; sql.ErrNoRows if none is known.
func (store *MessageStore) GetAnchor(chatJID string) (msgID string, fromMe bool, at time.Time, err error) {
	var ts int64
	err = store.db.QueryRow("SELECT msg_id, from_me, ts FROM chat_anchors WHERE jid = ?", chatJID).Scan(&msgID, &fromMe, &ts)
	return msgID, fromMe, time.Unix(ts, 0), err
}

// addedChats lists the chats in after that are not in before.
func addedChats(before, after []string) []string {
	had := map[string]bool{}
	for _, j := range before {
		had[j] = true
	}
	var out []string
	for _, j := range after {
		if !had[j] {
			out = append(out, j)
		}
	}
	return out
}

// historyRoles reports what a history sync may do besides storing allowlisted
// content: update chat names and times (and anchors), and apply #wa commands.
// An on-demand answer is older history of chats already known, so it does
// neither: an old "#wa remove" in it must not undo a newer add.
func historyRoles(t waHistorySync.HistorySync_HistorySyncType) (updateChats, applyCommands bool) {
	if t == waHistorySync.HistorySync_ON_DEMAND {
		return false, false
	}
	return true, true
}

// Chats added while no message of theirs was known (e.g. right after an
// upgrade): history is asked for when their next message arrives. In memory only.
var (
	historyWaitMu sync.Mutex
	historyWait   = map[string]bool{}
)

// takeHistoryWait reports whether chatJID was waiting for a message to fetch history before, and clears it.
func takeHistoryWait(chatJID string) bool {
	historyWaitMu.Lock()
	defer historyWaitMu.Unlock()
	if !historyWait[chatJID] {
		return false
	}
	delete(historyWait, chatJID)
	return true
}

// requestChatHistory asks the phone for the messages before the given one in chatJID.
func requestChatHistory(client *whatsmeow.Client, chatJID, msgID string, fromMe bool, at time.Time) {
	chat, err := types.ParseJID(chatJID)
	if err != nil {
		return
	}
	info := &types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, IsFromMe: fromMe}, ID: msgID, Timestamp: at}
	if _, err := client.SendPeerMessage(context.Background(), client.BuildHistorySyncRequest(info, historyOnAddCount)); err != nil {
		fmt.Printf("History on add: request failed: %v\n", err)
		return
	}
	fmt.Println("History on add: asked the phone for recent messages")
}

// requestHistoryForAdded asks for the recent history of every chat added to the
// allowlist since before was read.
func requestHistoryForAdded(client *whatsmeow.Client, store *MessageStore, before []string) {
	for _, j := range addedChats(before, currentConfig().AllowChats) {
		msgID, fromMe, at, err := store.GetAnchor(j)
		if err == sql.ErrNoRows {
			historyWaitMu.Lock()
			historyWait[j] = true
			historyWaitMu.Unlock()
			fmt.Println("History on add: no message known in this chat yet; asking when the next one arrives")
			continue
		}
		if err != nil {
			fmt.Printf("History on add: %v\n", err)
			continue
		}
		requestChatHistory(client, j, msgID, fromMe, at)
	}
}

// storeHistoryAnchor records the newest message with text or media in a synced
// conversation (history sync lists messages newest first).
func storeHistoryAnchor(store *MessageStore, chatJID string, messages []*waHistorySync.HistorySyncMsg) {
	for _, m := range messages {
		wm := m.GetMessage()
		if wm == nil || wm.GetKey().GetID() == "" || wm.GetMessageTimestamp() == 0 {
			continue
		}
		if extractTextContent(wm.GetMessage()) == "" {
			if mediaType, _, _, _, _, _, _ := extractMediaInfo(wm.GetMessage()); mediaType == "" {
				continue
			}
		}
		if err := store.StoreAnchor(chatJID, wm.GetKey().GetID(), wm.GetKey().GetFromMe(), time.Unix(int64(wm.GetMessageTimestamp()), 0)); err != nil {
			fmt.Printf("Failed to store chat anchor: %v\n", err)
		}
		return
	}
}
