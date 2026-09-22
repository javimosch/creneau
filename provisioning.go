package main

// Self-serve provisioning: a lab creates itself, gets a board, and administers it
// with a capability rather than an account — the same shape as a booking's
// manage_token, one level up.
//
// The admin token is stored HASHED, unlike manage_token. A booking token is a
// throwaway that gates one row; a lab admin token gates the whole tenant, so the
// store should not be able to hand it back. It is shown exactly once, at creation.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slugify turns "Fablab Chambéry!" into "fablab-chambery". Accents are stripped
// rather than transliterated properly — good enough for an id, and the caller can
// always pass its own.
func slugify(s string) string {
	repl := strings.NewReplacer(
		"à", "a", "á", "a", "â", "a", "ä", "a", "ã", "a", "å", "a",
		"è", "e", "é", "e", "ê", "e", "ë", "e",
		"ì", "i", "í", "i", "î", "i", "ï", "i",
		"ò", "o", "ó", "o", "ô", "o", "ö", "o", "õ", "o",
		"ù", "u", "ú", "u", "û", "u", "ü", "u",
		"ç", "c", "ñ", "n", "ß", "ss",
	)
	s = repl.Replace(strings.ToLower(strings.TrimSpace(s)))
	s = nonSlug.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

// --- rate limiting --------------------------------------------------------

// A public write needs a ceiling, or one script fills the store with labs.
// Deliberately in-memory: it resets on restart, which is the right trade for a
// single instance — a real limiter belongs in front, not in here.
type rateLimiter struct {
	mu    sync.Mutex
	hits  map[string][]time.Time
	limit int
	win   time.Duration
}

func newRateLimiter(limit int, win time.Duration) *rateLimiter {
	return &rateLimiter{hits: map[string][]time.Time{}, limit: limit, win: win}
}

func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	keep := make([]time.Time, 0, len(l.hits[key]))
	for _, t := range l.hits[key] {
		if now.Sub(t) < l.win {
			keep = append(keep, t)
		}
	}
	// bound the map so a spray of distinct IPs cannot grow it without limit
	if len(l.hits) > 4096 {
		l.hits = map[string][]time.Time{}
	}
	if len(keep) >= l.limit {
		l.hits[key] = keep
		return false
	}
	l.hits[key] = append(keep, now)
	return true
}

func clientIP(r *http.Request) string {
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		if i := strings.IndexByte(f, ','); i > 0 {
			return strings.TrimSpace(f[:i])
		}
		return strings.TrimSpace(f)
	}
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

var labCreateLimiter = newRateLimiter(5, time.Hour)

// --- provisioning ---------------------------------------------------------

// createLabHandler is the only public write that makes a tenant. It returns the
// admin token once; there is no way to recover it, which is stated in the reply.
func createLabHandler(w http.ResponseWriter, r *http.Request) {
	if !labCreateLimiter.allow(clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "rate_limited",
			"too many labs from this address; try again later")
		return
	}
	var in struct{ ID, Name, Email, TZ string }
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body", "body must be a JSON object")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	if in.Name == "" {
		writeErr(w, http.StatusBadRequest, "missing_argument", "name is required")
		return
	}
	if !strings.Contains(in.Email, "@") || len(in.Email) < 5 {
		writeErr(w, http.StatusBadRequest, "invalid_value",
			"a real email is required — it is the only way to reach you about the board")
		return
	}
	id := in.ID
	if id == "" {
		id = slugify(in.Name)
	}
	if err := validLabID(id); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_value", err.Error())
		return
	}
	if in.TZ == "" {
		in.TZ = "Europe/Paris"
	}

	c := newBkn()
	if _, err := c.get(ns, "labs", id); err == nil {
		writeErr(w, http.StatusConflict, "conflict",
			"the id "+id+" is taken; pass your own \"id\"")
		return
	}
	tok, err := manageToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "could not mint a token")
		return
	}
	rec, err := c.put(ns, "labs", id, doc{
		"id": id, "name": in.Name, "tz": in.TZ, "email": in.Email,
		"machines": []any{}, "created_at": time.Now().UTC().Format(stamp),
		"admin_token_hash": hashToken(tok),
		"trial_ends":       time.Now().UTC().AddDate(0, 0, 30).Format(stamp),
	})
	if err != nil {
		writeBknErr(w, err)
		return
	}
	delete(rec, "admin_token_hash")
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "lab": rec, "board": "/" + id,
		"admin_token": tok,
		"note":        "save the admin token now — it is hashed on our side and cannot be shown again",
		"next":        "POST /" + id + "/v1/machines with Authorization: Bearer <admin_token>",
	})
}

// labAdmin authorises an organizer action against the lab's admin token.
func labAdmin(r *http.Request, c *bkn) (lab, bool) {
	id := r.PathValue("lab")
	d, err := c.get(ns, "labs", id)
	if err != nil {
		return lab{}, false
	}
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	tok = strings.TrimSpace(tok)
	want := asStr(d["admin_token_hash"])
	if tok == "" || want == "" || hashToken(tok) != want {
		return lab{}, false
	}
	return labFrom(d), true
}

// addMachineHandler is add-machine over HTTP: the organizer surface that removes
// the need for SSH. It provisions availability + event type in one call, exactly
// as the CLI verb does.
func addMachineHandler(w http.ResponseWriter, r *http.Request) {
	c := newBkn()
	l, ok := labSession(r, c)
	if !ok {
		writeErr(w, http.StatusForbidden, "forbidden", "a valid session or admin token is required")
		return
	}
	var in struct {
		ID, Name, Open, Days string
		Minutes, Cooldown    int
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body", "body must be a JSON object")
		return
	}
	in.ID = slugify(in.ID)
	if in.ID == "" {
		writeErr(w, http.StatusBadRequest, "missing_argument", "id is required")
		return
	}
	if _, exists := l.machineByID(in.ID); exists {
		writeErr(w, http.StatusConflict, "conflict", "machine "+in.ID+" already exists")
		return
	}
	if in.Name == "" {
		in.Name = in.ID
	}
	if in.Minutes <= 0 {
		in.Minutes = 60
	}
	if in.Open == "" {
		in.Open = "09:00-18:00"
	}
	if in.Days == "" {
		in.Days = "mon,tue,wed,thu,fri,sat"
	}
	if err := provisionMachine(c, l, in.ID, in.Name, in.Minutes, in.Cooldown, in.Open, in.Days); err != nil {
		writeBknErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "lab": l.ID, "machine": in.ID,
		"calendar": calendarFor(l.ID, in.ID), "board": "/" + l.ID,
	})
}

// adminViewHandler is what an organizer actually needs to see: the machines and
// every confirmed booking, which the public surface deliberately never exposes.
func adminViewHandler(w http.ResponseWriter, r *http.Request) {
	c := newBkn()
	l, ok := labSession(r, c)
	if !ok {
		writeErr(w, http.StatusForbidden, "forbidden", "a valid lab admin token is required")
		return
	}
	recs, err := c.list(ns, "bookings", nil)
	if err != nil {
		writeBknErr(w, err)
		return
	}
	bookings := []map[string]any{}
	for _, b := range recs {
		if !ownedBy(b, l.ID) || asStr(b["status"]) != "confirmed" {
			continue
		}
		bookings = append(bookings, map[string]any{
			"id": b["id"], "machine": strings.TrimPrefix(asStr(b["calendar"]), l.ID+":"),
			"start": b["start"], "end": b["end"], "who": b["who"],
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "lab": l, "bookings": bookings, "count": len(bookings),
	})
}

// provisionMachine is shared by the CLI verb and the HTTP route so the two can
// never drift into configuring a machine differently.
func provisionMachine(c *bkn, l lab, machID, name string, minutes, cooldown int, open, days string) error {
	cal := calendarFor(l.ID, machID)
	rules := doc{}
	for _, d := range strings.Split(days, ",") {
		if d = strings.TrimSpace(d); d != "" {
			rules[d] = []string{open}
		}
	}
	if _, err := c.put(ns, "availability", cal, doc{
		"calendar": cal, "tz": l.TZ, "rules": rules, "overrides": doc{},
	}); err != nil {
		return err
	}
	if _, err := c.put(ns, "events", cal, doc{
		"slug": cal, "calendar": cal, "minutes": minutes,
		"buffer_before": 0, "buffer_after": cooldown,
		"min_notice_minutes": 0, "daily_cap": 0,
	}); err != nil {
		return err
	}
	l.Machines = append(l.Machines, machine{ID: machID, Name: name, Minutes: minutes, Cooldown: cooldown})
	ms := make([]any, 0, len(l.Machines))
	for _, m := range l.Machines {
		ms = append(ms, doc{"id": m.ID, "name": m.Name, "minutes": m.Minutes, "cooldown": m.Cooldown})
	}
	patch := doc{"machines": ms}
	_, err := c.patchIf(ns, "labs", l.ID, patch, nil)
	if err != nil {
		return fmt.Errorf("could not record the machine on the lab: %w", err)
	}
	return nil
}
