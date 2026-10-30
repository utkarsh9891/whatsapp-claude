package main

// 1:1 chat names from contacts on both addresses (phone number and LID): the owner's saved name,
// else a business name, else the name the person chose, marked "~". ADR 2026-10-30-7e2c9d.

import (
	"context"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// pickName is the best name among a person's contact entries.
func pickName(infos []types.ContactInfo) string {
	for _, field := range []func(types.ContactInfo) string{
		func(c types.ContactInfo) string { return c.FullName },
		func(c types.ContactInfo) string { return c.FirstName },
		func(c types.ContactInfo) string { return c.BusinessName },
	} {
		for _, c := range infos {
			if name := strings.TrimSpace(field(c)); name != "" {
				return name
			}
		}
	}
	for _, c := range infos {
		if name := strings.TrimSpace(c.PushName); name != "" {
			return "~" + name
		}
	}
	return ""
}

// needsName reports that a 1:1 chat's stored name is a placeholder (empty or digits) or a
// self-chosen "~" name that a saved one may replace.
func needsName(name string) bool {
	return strings.TrimLeft(name, "+0123456789") == "" || strings.HasPrefix(name, "~")
}

// isPersonJID reports a 1:1 chat address: a phone number or a LID.
func isPersonJID(jid types.JID) bool {
	return jid.Server == types.DefaultUserServer || jid.Server == types.HiddenUserServer
}

// contactName names a person from their contacts under jid and its other address, or "".
func contactName(client *whatsmeow.Client, jid types.JID) string {
	if !isPersonJID(jid) || client.Store == nil || client.Store.Contacts == nil {
		return ""
	}
	ctx := context.Background()
	jid = jid.ToNonAD()
	jids := []types.JID{jid}
	if client.Store.LIDs != nil {
		var other types.JID
		var err error
		if jid.Server == types.HiddenUserServer {
			other, err = client.Store.LIDs.GetPNForLID(ctx, jid)
		} else {
			other, err = client.Store.LIDs.GetLIDForPN(ctx, jid)
		}
		if err == nil && !other.IsEmpty() {
			jids = append(jids, other.ToNonAD())
		}
	}
	var infos []types.ContactInfo
	for _, j := range jids {
		if c, err := client.Store.Contacts.GetContact(ctx, j); err == nil && c.Found {
			infos = append(infos, c)
		}
	}
	return pickName(infos)
}

// refreshChatNames renames 1:1 chats still named by a placeholder when name knows better, and
// returns how many it renamed.
func refreshChatNames(store *MessageStore, name func(types.JID) string) (int, error) {
	rows, err := store.db.Query("SELECT jid, COALESCE(name, '') FROM chats WHERE jid NOT LIKE '%@g.us'")
	if err != nil {
		return 0, err
	}
	type rename struct{ jid, name string }
	var todo []rename
	for rows.Next() {
		var jidStr, current string
		if err := rows.Scan(&jidStr, &current); err != nil {
			rows.Close()
			return 0, err
		}
		jid, err := types.ParseJID(jidStr)
		if err != nil || !isPersonJID(jid) || !needsName(current) {
			continue
		}
		if better := name(jid); better != "" && better != current {
			todo = append(todo, rename{jidStr, better})
		}
	}
	rows.Close()
	for _, r := range todo {
		if _, err := store.db.Exec("UPDATE chats SET name = ? WHERE jid = ?", r.name, r.jid); err != nil {
			return 0, fmt.Errorf("renaming a chat: %w", err)
		}
	}
	return len(todo), nil
}
