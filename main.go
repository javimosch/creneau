// Command creneau is a booking backend built on bkn.
//
// This file is the agent-first surface: dispatch, the output contract, guide
// and help-json. The domain lives in domain.go (availability, event types and
// slot computation), booking.go (the commands) and scripts/ (the two writes
// that must not race).
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Version is stamped at build time: -ldflags "-X main.Version=$(git describe --tags --always)".
var Version = "0.0.0-dev"

// Exit codes follow cli-output-spec: 0 success, 80-119 typed failures.
const (
	exitOK          = 0
	exitUsage       = 80
	exitNotFound    = 92
	exitConflict    = 95
	exitUnavailable = 100
)

// sub dispatches "creneau <group> <verb>", so `bookings list` and `event
// create` read the way an agent guesses they do.
func sub(group string, args []string, verbs map[string]func([]string)) {
	if len(args) == 0 {
		fail(exitUsage, "missing_argument", group+" needs a subcommand", "creneau help-json")
	}
	fn, ok := verbs[args[0]]
	if !ok {
		fail(exitUsage, "unknown_command", fmt.Sprintf("unknown %s subcommand %q", group, args[0]),
			"creneau help-json")
	}
	fn(args[1:])
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(exitUsage)
	}
	switch os.Args[1] {
	case "serve":
		serve(os.Args[2:])
	case "lab":
		labCmd(os.Args[2:])
	case "install":
		install(os.Args[2:])
	case "availability":
		sub("availability", os.Args[2:], map[string]func([]string){"set": availabilitySet})
	case "event":
		sub("event", os.Args[2:], map[string]func([]string){"create": eventCreate})
	case "slots":
		slotsCmd(os.Args[2:])
	case "book":
		bookCmd(os.Args[2:])
	case "cancel":
		cancelCmd(os.Args[2:])
	case "reschedule":
		rescheduleCmd(os.Args[2:])
	case "bookings":
		sub("bookings", os.Args[2:], map[string]func([]string){"list": bookingsList})
	case "reconcile":
		reconcile(os.Args[2:])
	case "guide":
		out(guide())
	case "help-json":
		out(helpJSON())
	case "version", "--version", "-v":
		out(map[string]any{"ok": true, "tool": "creneau", "tool_version": Version, "version": "1.0"})
	case "help", "--help", "-h":
		usage()
	default:
		fail(exitUsage, "unknown_command", fmt.Sprintf("unknown command %q", os.Args[1]),
			"creneau help-json", "creneau guide")
	}
}

// --- output contract: data on stdout, context on stderr -------------------

func out(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func fail(code int, typ, msg string, suggestions ...string) {
	out(map[string]any{"ok": false, "error": map[string]any{
		"code": code, "type": typ, "message": msg,
		"recoverable": false, "suggestions": suggestions,
	}})
	os.Exit(code)
}

func usage() {
	fmt.Fprintln(os.Stderr, "creneau — a booking backend you own, built on bkn")
	fmt.Fprintln(os.Stderr, "\n  creneau install                       declare collections + load scripts into bkn")
	fmt.Fprintln(os.Stderr, "  creneau availability set --tz Europe/Paris --mon 09:00-17:00")
	fmt.Fprintln(os.Stderr, "  creneau event create intro-30 --minutes 30 --buffer-after 10")
	fmt.Fprintln(os.Stderr, "  creneau slots --event intro-30 --from 2026-09-10 --to 2026-09-12")
	fmt.Fprintln(os.Stderr, "  creneau book --event intro-30 --at <rfc3339> --who ada@example.io")
	fmt.Fprintln(os.Stderr, "  creneau cancel <id> | reschedule <id> --at <rfc3339>")
	fmt.Fprintln(os.Stderr, "  creneau bookings list [--upcoming] | reconcile")
	fmt.Fprintln(os.Stderr, "  creneau serve [--port N] [--host H]")
	fmt.Fprintln(os.Stderr, "  creneau guide | help-json | version")
	fmt.Fprintln(os.Stderr, "\nNeeds a running bkn: BKN_URL (default http://127.0.0.1:7799), BKN_ADMIN_TOKEN.")
}

// --- serve ----------------------------------------------------------------

// corsMiddleware lets a browser board on another origin call the public surface.
// The board is a static client that reads /v1/slots and posts /v1/book — the same
// two calls an agent makes, just driven by a mouse. Without this it cannot.
// --origin may be repeated; with none set, no CORS headers are emitted at all
// (same-origin only), so the default stays closed.
func corsMiddleware(next http.Handler, allowed []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			for _, a := range allowed {
				if a == origin || a == "*" {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
					w.Header().Set("Access-Control-Max-Age", "600")
					break
				}
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func serve(args []string) {
	host, port := "127.0.0.1", 7800
	var origins []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--port":
			if i+1 < len(args) {
				i++
				n, err := strconv.Atoi(args[i])
				if err != nil {
					fail(exitUsage, "invalid_value", "--port must be a number")
				}
				port = n
			}
		case "--host":
			if i+1 < len(args) {
				i++
				host = args[i]
			}
		case "--origin":
			if i+1 < len(args) {
				i++
				origins = append(origins, args[i])
			}
		}
	}
	if env := os.Getenv("CRENEAU_ORIGINS"); env != "" && len(origins) == 0 {
		for _, o := range strings.Split(env, ",") {
			if o = strings.TrimSpace(o); o != "" {
				origins = append(origins, o)
			}
		}
	}

	mux := http.NewServeMux()
	// Every public surface is lab-scoped: /{lab} is the board and
	// /{lab}/v1/... is its API. The lab id is part of the path because it is
	// part of the calendar key, so a request cannot forget which tenant it is.
	mux.HandleFunc("GET /", labIndexHandler)
	mux.HandleFunc("GET /{lab}", labBoardHandler)
	mux.HandleFunc("GET /{lab}/admin", labAdminPageHandler)
	// Self-serve: a lab creates itself and administers it with a capability.
	mux.HandleFunc("POST /v1/labs", createLabHandler)
	// Losing the admin token must not orphan a lab.
	mux.HandleFunc("POST /v1/labs/recover", recoverStartHandler)
	mux.HandleFunc("POST /v1/labs/recover/confirm", recoverConfirmHandler)
	mux.HandleFunc("POST /{lab}/v1/machines", addMachineHandler)
	mux.HandleFunc("GET /{lab}/v1/admin", adminViewHandler)
	// The browser exchanges the admin key for a session, and never stores the key.
	mux.HandleFunc("POST /{lab}/v1/session", sessionStartHandler)
	mux.HandleFunc("DELETE /{lab}/v1/session", sessionEndHandler)
	mux.HandleFunc("POST /{lab}/v1/machines/{machine}/closures", closeDayHandler)
	mux.HandleFunc("POST /{lab}/v1/admin/cancel", adminCancelHandler)
	mux.HandleFunc("GET /_health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "creneau", "pid": os.Getpid()})
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tool": "creneau", "tool_version": Version})
	})
	mux.HandleFunc("GET /guide", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, guide())
	})
	// The public booking page: a stranger reads slots and books one without
	// an account. Both are unauthenticated on purpose; everything that needs
	// an organizer is a CLI verb, not a route.
	mux.HandleFunc("GET /{lab}/v1/slots", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		labID := r.PathValue("lab")
		event, from, to := q.Get("machine"), q.Get("from"), q.Get("to")
		if event == "" {
			event = q.Get("event") // the pre-tenancy name, still accepted
		}
		if event == "" {
			writeErr(w, http.StatusBadRequest, "missing_argument", "machine is required")
			return
		}
		if _, err := loadLab(newBkn(), labID); err != nil {
			writeErr(w, http.StatusNotFound, "not_found", "no such lab "+labID)
			return
		}
		event = calendarFor(labID, event)
		if from == "" {
			from = time.Now().UTC().Format("2006-01-02")
		}
		if to == "" {
			to = from
		}
		slots, ev, err := loadSlots(newBkn(), event, from, to)
		if err != nil {
			writeBknErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "event": ev.Slug,
			"calendar": ev.Calendar, "count": len(slots), "slots": slots})
	})

	// Managing your own booking without an account: the capability is the token
	// minted at book time, not a session. Wrong or missing token is a 403, and a
	// booking that is already cancelled is a 409 rather than a quiet success.
	mux.HandleFunc("GET /{lab}/v1/booking/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, tok := r.PathValue("id"), r.URL.Query().Get("t")
		rec, err := newBkn().get(ns, "bookings", id)
		if err != nil || !ownedBy(rec, r.PathValue("lab")) {
			writeErr(w, http.StatusNotFound, "not_found", "no such booking")
			return
		}
		if tok == "" || rec["manage_token"] != tok {
			writeErr(w, http.StatusForbidden, "forbidden", "a valid manage token is required")
			return
		}
		delete(rec, "manage_token")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "booking": rec})
	})
	mux.HandleFunc("POST /{lab}/v1/cancel", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ ID, Token string }
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid_body", "body must be JSON")
			return
		}
		if in.ID == "" || in.Token == "" {
			writeErr(w, http.StatusBadRequest, "missing_argument", "id and token are required")
			return
		}
		c := newBkn()
		rec, err := c.get(ns, "bookings", in.ID)
		if err != nil || !ownedBy(rec, r.PathValue("lab")) {
			writeErr(w, http.StatusNotFound, "not_found", "no such booking")
			return
		}
		if rec["manage_token"] != in.Token {
			writeErr(w, http.StatusForbidden, "forbidden", "a valid manage token is required")
			return
		}
		out, cerr := cancelBooking(c, in.ID)
		if cerr != nil {
			if be, ok := cerr.(*bknError); ok && be.Status == http.StatusConflict {
				writeErr(w, http.StatusConflict, "conflict", "this booking is not confirmed any more")
				return
			}
			writeErr(w, http.StatusBadGateway, "upstream", cerr.Error())
			return
		}
		delete(out, "manage_token")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "booking": out})
	})
	mux.HandleFunc("POST /{lab}/v1/book", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Machine, Event, At, Who, Name string }
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid_body", "body must be a JSON object")
			return
		}
		labID := r.PathValue("lab")
		if body.Machine != "" {
			body.Event = body.Machine
		}
		if body.Event == "" || body.At == "" || body.Who == "" {
			writeErr(w, http.StatusBadRequest, "missing_argument", "machine, at and who are required")
			return
		}
		c := newBkn()
		l, lerr := loadLab(c, labID)
		if lerr != nil {
			writeErr(w, http.StatusNotFound, "not_found", "no such lab "+labID)
			return
		}
		if _, ok := l.machineByID(body.Event); !ok {
			writeErr(w, http.StatusNotFound, "not_found", "no machine "+body.Event+" in "+labID)
			return
		}
		rec, err := book(c, calendarFor(labID, body.Event), body.At, body.Who, body.Name)
		if err != nil {
			writeBknErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "booking": rec})
	})

	// Anything else answers honestly rather than 404ing, so anybody who finds
	// this URL learns what it is instead of guessing.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"ok": false,
			"error": map[string]any{
				"type":        "no_such_route",
				"message":     "creneau serves the public booking surface only; organizer actions are CLI verbs",
				"suggestions": []string{"GET /guide", "GET /v1/slots?event=<slug>", "POST /v1/book"},
			},
		})
	})

	addr := host + ":" + strconv.Itoa(port)
	fmt.Fprintf(os.Stderr, "[serve] creneau %s listening on http://%s\n", Version, addr)
	srv := &http.Server{Addr: addr, Handler: corsMiddleware(mux, origins), ReadHeaderTimeout: 10 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		fail(exitUnavailable, "listen_failed", err.Error())
	}
}

func writeErr(w http.ResponseWriter, status int, typ, msg string) {
	writeJSON(w, status, map[string]any{"ok": false,
		"error": map[string]any{"type": typ, "message": msg}})
}

// writeBknErr keeps bkn's meaning: a taken slot is a 409 to the browser too.
func writeBknErr(w http.ResponseWriter, err error) {
	if be, ok := err.(*bknError); ok {
		switch be.Status {
		case 409:
			writeErr(w, http.StatusConflict, "conflict", be.Msg)
			return
		case 404:
			writeErr(w, http.StatusNotFound, "not_found", be.Msg)
			return
		}
	}
	writeErr(w, http.StatusBadGateway, "bkn_error", err.Error())
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// --- guide + catalog ------------------------------------------------------

func guide() map[string]any {
	return map[string]any{
		"creneau":   Version,
		"one_liner": "A booking backend you own: availability, slots, and reservations that never double-book.",
		"model": map[string]any{
			"builds_on": "bkn (github.com/javimosch/bkn). creneau stores everything in bkn collections " +
				"under the `creneau` namespace and runs two bkn scripts for the writes that must not race.",
			"needs": "A running bkn. Set BKN_URL (default http://127.0.0.1:7799) and BKN_ADMIN_TOKEN, " +
				"then run `creneau install` once to declare the collections and load the scripts.",
			"shape": "An organizer publishes weekly availability in a timezone. An event type fixes a " +
				"duration and its guard rails. A slot is availability minus bookings minus buffers " +
				"minus minimum notice. Booking one is atomic.",
		},
		"loop": []string{
			"1. creneau install",
			"2. creneau availability set --calendar default --tz Europe/Paris --mon 09:00-17:00 --tue 09:00-17:00",
			"3. creneau event create intro-30 --calendar default --minutes 30 --buffer-after 10 --min-notice 4h",
			"4. creneau slots --event intro-30 --from 2026-09-10 --to 2026-09-12",
			"5. creneau book --event intro-30 --at <a start from step 4> --who ada@example.io",
			"6. creneau bookings list --upcoming",
		},
		"concepts": map[string]any{
			"creneau":      "French for a time slot: one bookable interval on one calendar.",
			"calendar":     "The unit of contention. Every booking write for one calendar takes the same lease, which is what makes 'never double-book' true.",
			"availability": "Weekly windows per weekday in a timezone, plus date overrides. An override with no windows closes that date.",
			"event type":   "Duration, buffer before/after, minimum notice and an optional daily cap. Slots step by the duration, so with no buffers back-to-back bookings are possible and touching ends do not overlap. With a buffer, a candidate is tested INCLUDING ITS OWN buffers: if buffer-after is 10, the slot ending at 09:00 needs 09:00-09:10 free too, so a booking at 09:00 removes it as well as the one at 09:30.",
			"slot":         "Computed, never stored. Asking twice can legitimately give different answers, because somebody may have booked in between.",
			"reschedule":   "Not atomic — bkn has no transactions. It takes the new slot, then retires the old one under a compare-and-set, and deletes the new one if that fails. A crash in between leaves two confirmed bookings, which `creneau reconcile` cleans up. docs/ledger.md explains why this is the honest design rather than a bug.",
			"times":        "Every stored time is RFC3339 in UTC, because bkn compares filter values as text and only fixed-width UTC sorts correctly.",
		},
		"commands": map[string]any{
			"setup":    []string{"creneau install [--bkn <path>] [--dry-run]"},
			"organize": []string{"creneau availability set [--calendar c] [--tz Z] [--mon|--tue|--wed|--thu|--fri|--sat|--sun 09:00-12:00,13:00-17:00] [--closed 2026-12-25]", "creneau event create <slug> [--calendar c] [--minutes 30] [--buffer-before N] [--buffer-after N] [--min-notice 4h] [--daily-cap N]"},
			"booking":  []string{"creneau slots --event <slug> [--from D] [--to D]", "creneau book --event <slug> --at <rfc3339> --who <email> [--name N]", "creneau cancel <id>", "creneau reschedule <id> --at <rfc3339>", "creneau bookings list [--calendar c] [--who e] [--upcoming] [--limit N]", "creneau reconcile [--dry-run]"},
			"server":   []string{"creneau serve [--host H] [--port N] [--origin https://site]  # public: GET /v1/slots, POST /v1/book"},
			"meta":     []string{"creneau guide", "creneau help-json", "creneau version"},
		},
		"exit_codes": map[string]any{
			"0": "success", "80": "usage or invalid arguments", "92": "not found",
			"95":  "conflict — the slot was taken, or the booking changed underneath you",
			"100": "bkn unreachable or refused",
		},
		"gotchas": []string{
			"Run `creneau install` before anything else, or every command fails with no_collection.",
			"`--at` must be an exact slot start from `creneau slots`. Booking an arbitrary time inside a window is refused only if it overlaps something; creneau does not snap you to the grid.",
			"Cancelling twice is a 95 conflict, not a silent success — the second cancel finds the booking already cancelled.",
			"The concurrency criterion (zero double-bookings at c=16) must be measured against a local instance, never through a CDN or a reverse proxy — that would measure the proxy.",
			"Slot computation reads bookings through a bkn script, not over bkn's HTTP API, because a range needs two bounds on one field and the query string keeps only the first. See docs/ledger.md row 3.",
		},
		"see_also": []string{
			"https://github.com/javimosch/creneau",
			"https://github.com/javimosch/bkn",
			"docs/pre-registration.md — the frozen criteria this repo is measured against",
			"docs/ledger.md — every gap hit in bkn, admitted or refused",
		},
	}
}

func helpJSON() map[string]any {
	none := []string{}
	cmd := func(args, flags []string) map[string]any {
		return map[string]any{"args": args, "flags": flags}
	}
	return map[string]any{
		"tool":    "creneau",
		"version": Version,
		"commands": map[string]any{
			"install":          cmd(none, []string{"--bkn <path>", "--dry-run"}),
			"availability set": cmd(none, []string{"--calendar <c>", "--tz <zone>", "--mon <w>", "--tue <w>", "--wed <w>", "--thu <w>", "--fri <w>", "--sat <w>", "--sun <w>", "--closed <dates>"}),
			"event create":     cmd([]string{"slug"}, []string{"--calendar <c>", "--minutes <n>", "--buffer-before <n>", "--buffer-after <n>", "--min-notice <4h>", "--daily-cap <n>"}),
			"slots":            cmd(none, []string{"--event <slug>", "--from <date>", "--to <date>"}),
			"book":             cmd(none, []string{"--event <slug>", "--at <rfc3339>", "--who <email>", "--name <n>"}),
			"cancel":           cmd([]string{"id"}, none),
			"reschedule":       cmd([]string{"id"}, []string{"--at <rfc3339>"}),
			"bookings list":    cmd(none, []string{"--calendar <c>", "--who <email>", "--upcoming", "--limit <n>"}),
			"reconcile":        cmd(none, []string{"--dry-run"}),
			"serve":            cmd(none, []string{"--host <h>", "--port <n>"}),
			"guide":            cmd(none, none),
			"help-json":        cmd(none, none),
			"version":          cmd(none, none),
		},
		"env": map[string]any{
			"BKN_URL":         "where bkn is, default http://127.0.0.1:7799",
			"BKN_ADMIN_TOKEN": "bkn admin token, if that instance requires one",
		},
		"routes": map[string]any{
			"GET /v1/slots": "public: ?event=<slug>&from=<date>&to=<date>",
			"POST /v1/book": "public: {event, at, who, name}",
			"GET /guide":    "this guide",
			"GET /_health":  "liveness",
		},
		"exit_codes": map[string]any{
			"0":   "success",
			"80":  "usage or invalid arguments",
			"92":  "not found",
			"95":  "conflict — slot taken, or the booking changed underneath you",
			"100": "bkn unreachable or refused",
		},
	}
}
