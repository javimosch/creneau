package main

// The organizer surface, and the credential split behind it.
//
// The admin token is an opaque 128-bit key with no expiry — right for an agent or
// a curl, wrong for a browser: a tenant-wide credential that never rotates sitting
// in localStorage means XSS or a borrowed laptop is permanent access.
//
// So the browser never holds it. It exchanges the admin token ONCE for a session:
// short-lived, sliding, revocable by deleting a row. That is the TTL-plus-refresh
// behaviour a JWT would give, without a signing key to manage — and revocable,
// which a stateless JWT is not without a denylist that would itself be a table.

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	sessionTTL     = 14 * 24 * time.Hour
	sessionRenewAt = 7 * 24 * time.Hour // slide the expiry when less than this remains
)

// sessionStart exchanges the admin token for a session token.
func sessionStartHandler(w http.ResponseWriter, r *http.Request) {
	c := newBkn()
	l, ok := labAdmin(r, c)
	if !ok {
		// Go matches JSON keys case-insensitively but NOT underscore-insensitively,
		// so "admin_token" needs the tag or it silently stays empty.
		var in struct {
			AdminToken string `json:"admin_token"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in)
		if in.AdminToken != "" {
			r.Header.Set("Authorization", "Bearer "+in.AdminToken)
			l, ok = labAdmin(r, c)
		}
	}
	if !ok {
		writeErr(w, http.StatusForbidden, "forbidden", "a valid lab admin token is required")
		return
	}
	tok, err := manageToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "could not mint a session")
		return
	}
	exp := time.Now().UTC().Add(sessionTTL)
	if _, err := c.put(ns, "sessions", hashToken(tok), doc{
		"lab": l.ID, "expires": exp.Format(stamp), "created_at": time.Now().UTC().Format(stamp),
	}); err != nil {
		writeBknErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "lab": l.ID, "session": tok,
		"expires": exp.Format(stamp),
		"note":    "keep the session, not the admin token — this one expires and can be revoked",
	})
}

// labSession authorises by session OR admin token, so agents keep using the key
// while the browser uses the session. It slides the expiry on use.
func labSession(r *http.Request, c *bkn) (lab, bool) {
	if l, ok := labAdmin(r, c); ok {
		return l, true
	}
	tok := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if tok == "" {
		return lab{}, false
	}
	key := hashToken(tok)
	d, err := c.get(ns, "sessions", key)
	if err != nil {
		return lab{}, false
	}
	exp, perr := time.Parse(stamp, asStr(d["expires"]))
	if perr != nil || time.Now().UTC().After(exp) {
		return lab{}, false
	}
	labID := asStr(d["lab"])
	if labID != r.PathValue("lab") {
		return lab{}, false
	}
	// sliding window: extend only when it is worth a write
	if time.Until(exp) < sessionRenewAt {
		_, _ = c.patchIf(ns, "sessions", key,
			doc{"expires": time.Now().UTC().Add(sessionTTL).Format(stamp)}, nil)
	}
	l, err := loadLab(c, labID)
	if err != nil {
		return lab{}, false
	}
	return l, true
}

func sessionEndHandler(w http.ResponseWriter, r *http.Request) {
	tok := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if tok == "" {
		writeErr(w, http.StatusBadRequest, "missing_argument", "a session is required")
		return
	}
	// Deleting the row is the whole revocation story — no denylist, no waiting
	// for an expiry we cannot take back.
	_ = newBkn().del(ns, "sessions", hashToken(tok))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "revoked": true})
}

// --- maintenance windows --------------------------------------------------

// closeDayHandler blocks a machine for a whole day. This is the gap that
// actively misled members: a machine down for repair kept accepting bookings,
// because availability supported `overrides` and nothing ever wrote one.
func closeDayHandler(w http.ResponseWriter, r *http.Request) {
	c := newBkn()
	l, ok := labSession(r, c)
	if !ok {
		writeErr(w, http.StatusForbidden, "forbidden", "a valid session or admin token is required")
		return
	}
	machID := r.PathValue("machine")
	if _, exists := l.machineByID(machID); !exists {
		writeErr(w, http.StatusNotFound, "not_found", "no machine "+machID+" in "+l.ID)
		return
	}
	var in struct {
		Day    string   `json:"day"`
		Hours  []string `json:"hours"`
		Reopen bool     `json:"reopen"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body", "body must be a JSON object")
		return
	}
	if _, err := time.Parse("2006-01-02", in.Day); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_value", "day must be YYYY-MM-DD")
		return
	}
	cal := calendarFor(l.ID, machID)
	avDoc, err := c.get(ns, "availability", cal)
	if err != nil {
		writeBknErr(w, err)
		return
	}
	ov := map[string]any{}
	if raw, isMap := avDoc["overrides"].(map[string]any); isMap {
		ov = raw
	}
	if in.Reopen {
		delete(ov, in.Day)
	} else {
		// An empty list closes the day; a list of windows narrows it instead.
		windows := in.Hours
		if windows == nil {
			windows = []string{}
		}
		ov[in.Day] = windows
	}
	avDoc["overrides"] = ov
	if _, err := c.put(ns, "availability", cal, avDoc); err != nil {
		writeBknErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "machine": machID, "day": in.Day,
		"closed": !in.Reopen && len(in.Hours) == 0, "hours": ov[in.Day],
		"note": "existing bookings are untouched — closing a day does not cancel what is already booked",
	})
}

// --- organizer cancel -----------------------------------------------------

// adminCancelHandler lets the organizer cancel any booking in their lab. Until
// now only the member holding the manage token could, so a lab could not clear
// a slot for a machine it had just taken out of service.
func adminCancelHandler(w http.ResponseWriter, r *http.Request) {
	c := newBkn()
	l, ok := labSession(r, c)
	if !ok {
		writeErr(w, http.StatusForbidden, "forbidden", "a valid session or admin token is required")
		return
	}
	var in struct{ ID string }
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in); err != nil || in.ID == "" {
		writeErr(w, http.StatusBadRequest, "missing_argument", "id is required")
		return
	}
	rec, err := c.get(ns, "bookings", in.ID)
	if err != nil || !ownedBy(rec, l.ID) {
		writeErr(w, http.StatusNotFound, "not_found", "no such booking in this lab")
		return
	}
	out, cerr := cancelBooking(c, in.ID)
	if cerr != nil {
		if be, isBkn := cerr.(*bknError); isBkn && be.Status == http.StatusConflict {
			writeErr(w, http.StatusConflict, "conflict", "this booking is not confirmed any more")
			return
		}
		writeBknErr(w, cerr)
		return
	}
	delete(out, "manage_token")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "booking": out})
}
