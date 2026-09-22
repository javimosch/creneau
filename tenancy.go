package main

// Tenancy rides on the key that already decides everything: the calendar.
//
// A machine's calendar was already its resource ("the calendar IS the resource"),
// and the booking lease is already `creneau-cal-<calendar>`. So scoping a calendar
// to `<lab>:<machine>` makes two labs independent for free — no change to the
// conflict query, no change to the lease, no new contention. The lab id is part
// of the key, not a filter that could be forgotten.

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// A lab id lands in URLs and in calendar keys, so keep it boring.
var labIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$`)

// reserved paths that can never be a lab id, because they are routes
var reservedLabIDs = map[string]bool{
	"v1": true, "_health": true, "guide": true, "version": true,
	"favicon.ico": true, "robots.txt": true, "admin": true, "api": true,
}

type machine struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Minutes  int    `json:"minutes"`
	Cooldown int    `json:"cooldown"`
}

type lab struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	TZ        string    `json:"tz"`
	Machines  []machine `json:"machines"`
	CreatedAt string    `json:"created_at"`
	// Who can administer this lab, and its agent identity. Without these the
	// admin view silently dropped them, so an organizer could not see who had
	// access — the owners were stored correctly, just invisible.
	Owners      []owner `json:"owners"`
	AgentHandle string  `json:"agent_handle,omitempty"`
	TrialEnds   string  `json:"trial_ends,omitempty"`
}

// calendarFor is the one place the tenancy key is built. Everything else —
// availability ids, event ids, booking filters, the lease — derives from it.
func calendarFor(labID, machineID string) string { return labID + ":" + machineID }

func validLabID(id string) error {
	if !labIDRe.MatchString(id) {
		return fmt.Errorf("a lab id is 3-40 chars of a-z, 0-9 and hyphens, starting and ending alphanumeric")
	}
	if reservedLabIDs[id] {
		return fmt.Errorf("%q is a reserved path", id)
	}
	return nil
}

func labFrom(d doc) lab {
	l := lab{
		ID:          asStr(d["id"]),
		Name:        asStr(d["name"]),
		TZ:          asStr(d["tz"]),
		CreatedAt:   asStr(d["created_at"]),
		Owners:      ownersOf(d),
		AgentHandle: asStr(d["agent_handle"]),
		TrialEnds:   asStr(d["trial_ends"]),
	}
	if raw, ok := d["machines"].([]any); ok {
		for _, m := range raw {
			md, ok := m.(map[string]any)
			if !ok {
				continue
			}
			l.Machines = append(l.Machines, machine{
				ID: asStr(md["id"]), Name: asStr(md["name"]),
				Minutes: asInt(md["minutes"]), Cooldown: asInt(md["cooldown"]),
			})
		}
	}
	return l
}

func loadLab(c *bkn, id string) (lab, error) {
	d, err := c.get(ns, "labs", id)
	if err != nil {
		return lab{}, err
	}
	return labFrom(d), nil
}

func (l lab) machineByID(id string) (machine, bool) {
	for _, m := range l.Machines {
		if m.ID == id {
			return m, true
		}
	}
	return machine{}, false
}

// --- CLI ------------------------------------------------------------------

func labCmd(args []string) {
	if len(args) == 0 {
		fail(exitUsage, "missing_argument", "lab needs a subcommand",
			"creneau lab create <id> --name '...' | lab list | lab show <id> | lab add-machine <id> <machine> | lab rotate <id> | lab delete <id> --yes")
	}
	switch args[0] {
	case "create":
		labCreate(args[1:])
	case "list":
		labList()
	case "show":
		labShow(args[1:])
	case "add-machine":
		labAddMachine(args[1:])
	case "rotate":
		labRotate(args[1:])
	case "delete":
		labDelete(args[1:])
	default:
		fail(exitUsage, "invalid_value", "unknown lab subcommand "+args[0],
			"creneau lab create|list|show|add-machine")
	}
}

func labCreate(args []string) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fail(exitUsage, "missing_argument", "lab create needs an id",
			"creneau lab create chambery --name 'Fablab Chambéry'")
	}
	id := args[0]
	fs := flag.NewFlagSet("lab create", flag.ExitOnError)
	name := fs.String("name", "", "display name")
	tz := fs.String("tz", "Europe/Paris", "IANA timezone")
	_ = fs.Parse(args[1:])
	if err := validLabID(id); err != nil {
		fail(exitUsage, "invalid_value", err.Error())
	}
	if *name == "" {
		*name = id
	}
	c := newBkn()
	if _, err := c.get(ns, "labs", id); err == nil {
		fail(exitConflict, "conflict", "lab "+id+" already exists", "creneau lab show "+id)
	}
	// Mint an admin token here too, or a CLI-created lab cannot use its own
	// admin page — the HTTP path did this and the CLI did not, which made the
	// two produce labs with different capabilities.
	tok, terr := manageToken()
	if terr != nil {
		fail(exitUnavailable, "internal", "could not mint an admin token")
	}
	rec, err := c.put(ns, "labs", id, doc{
		"id": id, "name": *name, "tz": *tz,
		"machines": []any{}, "created_at": time.Now().UTC().Format(stamp),
		"admin_token_hash": hashToken(tok),
		"trial_ends":       time.Now().UTC().AddDate(0, 0, 30).Format(stamp),
	})
	if err != nil {
		failBkn(err)
	}
	delete(rec, "admin_token_hash")
	out(map[string]any{"ok": true, "lab": rec, "board": "/" + id,
		"admin": "/" + id + "/admin", "admin_token": tok,
		"note": "the admin token is shown once — it is stored hashed"})
}

func labList() {
	c := newBkn()
	recs, err := c.list(ns, "labs", nil)
	if err != nil {
		failBkn(err)
	}
	labs := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		l := labFrom(r)
		labs = append(labs, map[string]any{
			"id": l.ID, "name": l.Name, "machines": len(l.Machines), "board": "/" + l.ID,
		})
	}
	out(map[string]any{"ok": true, "count": len(labs), "labs": labs})
}

func labShow(args []string) {
	if len(args) == 0 {
		fail(exitUsage, "missing_argument", "lab show needs an id")
	}
	l, err := loadLab(newBkn(), args[0])
	if err != nil {
		failBkn(err)
	}
	out(map[string]any{"ok": true, "lab": l, "board": "/" + l.ID})
}

// labAddMachine registers a machine AND provisions the two records that make it
// bookable — its own availability and its own event type, both keyed on the
// tenancy calendar. One verb, so a lab cannot end up half-configured.
func labAddMachine(args []string) {
	if len(args) < 2 || strings.HasPrefix(args[1], "-") {
		fail(exitUsage, "missing_argument", "add-machine needs a lab id and a machine id",
			"creneau lab add-machine chambery laser --name 'Trotec Speedy 400' --minutes 60 --cooldown 15")
	}
	labID, machID := args[0], args[1]
	fs := flag.NewFlagSet("add-machine", flag.ExitOnError)
	name := fs.String("name", "", "display name")
	minutes := fs.Int("minutes", 60, "slot length")
	cooldown := fs.Int("cooldown", 0, "minutes blocked after each job")
	open := fs.String("open", "09:00-18:00", "daily opening hours")
	days := fs.String("days", "mon,tue,wed,thu,fri,sat", "days the machine is available")
	_ = fs.Parse(args[2:])
	if !labIDRe.MatchString(machID) {
		fail(exitUsage, "invalid_value", "a machine id is a-z, 0-9 and hyphens")
	}
	if *name == "" {
		*name = machID
	}

	c := newBkn()
	l, err := loadLab(c, labID)
	if err != nil {
		fail(exitUnavailable, "not_found", "no such lab "+labID, "creneau lab create "+labID)
	}
	if _, exists := l.machineByID(machID); exists {
		fail(exitConflict, "conflict", "machine "+machID+" already exists in "+labID)
	}

	if err := provisionMachine(c, l, machID, *name, *minutes, *cooldown, *open, *days); err != nil {
		failBkn(err)
	}
	l.Machines = append(l.Machines, machine{ID: machID, Name: *name})
	fmt.Fprintf(os.Stderr, "[lab] %s now has %d machines\n", labID, len(l.Machines))
	out(map[string]any{"ok": true, "lab": labID, "machine": machID, "calendar": calendarFor(labID, machID), "board": "/" + labID})
}

// ownedBy keeps one lab from even naming another lab's booking. The manage
// token already gates the action; this makes a cross-tenant id a 404 instead
// of a 403, so ids from one lab reveal nothing about another.
func ownedBy(rec doc, labID string) bool {
	if labID == "" {
		return false
	}
	return strings.HasPrefix(asStr(rec["calendar"]), labID+":")
}

// labRotate mints a fresh admin token for an existing lab — needed for labs
// created before `lab create` minted one, and useful as an operator escape
// hatch when a token leaks.
func labRotate(args []string) {
	if len(args) == 0 {
		fail(exitUsage, "missing_argument", "lab rotate needs an id", "creneau lab rotate <id>")
	}
	c := newBkn()
	if _, err := loadLab(c, args[0]); err != nil {
		fail(exitUnavailable, "not_found", "no such lab "+args[0])
	}
	tok, terr := manageToken()
	if terr != nil {
		fail(exitUnavailable, "internal", "could not mint a token")
	}
	if _, err := c.patchIf(ns, "labs", args[0], doc{"admin_token_hash": hashToken(tok)}, nil); err != nil {
		failBkn(err)
	}
	out(map[string]any{"ok": true, "lab": args[0], "admin_token": tok,
		"admin": "/" + args[0] + "/admin",
		"note":  "any previous admin token and existing sessions for this lab stop working"})
}

// labDelete removes a lab and everything scoped to it. Destructive and
// irreversible, so it refuses without --yes and refuses while any booking is
// still upcoming — the same rule machine delete follows.
func labDelete(args []string) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fail(exitUsage, "missing_argument", "lab delete needs an id",
			"creneau lab delete <id> --yes")
	}
	id := args[0]
	fs := flag.NewFlagSet("lab delete", flag.ExitOnError)
	yes := fs.Bool("yes", false, "confirm: this cannot be undone")
	force := fs.Bool("force", false, "delete even if bookings are still upcoming")
	_ = fs.Parse(args[1:])

	c := newBkn()
	l, err := loadLab(c, id)
	if err != nil {
		fail(exitUnavailable, "not_found", "no such lab "+id)
	}
	if !*yes {
		fail(exitUsage, "confirmation_required",
			"deleting "+id+" removes its machines, availability and bookings",
			"creneau lab delete "+id+" --yes")
	}
	upcoming := 0
	for _, m := range l.Machines {
		upcoming += futureBookings(c, calendarFor(id, m.ID))
	}
	if upcoming > 0 && !*force {
		fail(exitConflict, "conflict",
			"this lab has "+itoa(upcoming)+" upcoming booking(s)",
			"cancel them, or pass --force")
	}
	for _, m := range l.Machines {
		cal := calendarFor(id, m.ID)
		_ = c.del(ns, "availability", cal)
		_ = c.del(ns, "events", cal)
	}
	// Bookings are kept as history but become unreachable; sessions must go, or
	// a revoked lab would still have live credentials pointing at nothing.
	if recs, lerr := c.list(ns, "sessions", nil); lerr == nil {
		for _, s := range recs {
			if asStr(s["lab"]) == id {
				_ = c.del(ns, "sessions", asStr(s["id"]))
			}
		}
	}
	if err := c.del(ns, "labs", id); err != nil {
		failBkn(err)
	}
	out(map[string]any{"ok": true, "deleted": id, "machines_removed": len(l.Machines),
		"note": "past bookings are kept as history but are no longer reachable"})
}
