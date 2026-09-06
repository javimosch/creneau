// Command creneau is a booking backend built on bkn.
//
// This is the CLI shell: the agent-first surface (guide, help-json, version,
// serve) and nothing else. None of the booking domain is implemented, and that
// is deliberate — docs/pre-registration.md was committed before any code, and
// the findings ledger has to stay empty until there is something to find.
//
// Deliberately not counted as application code for the F4 threshold: this file
// is CLI plumbing, not handlers. When the domain lands, the line count that
// matters is the one under the handler package.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"
)

// Version is stamped at build time: -ldflags "-X main.Version=$(git describe --tags --always)".
var Version = "0.0.0-dev"

// Exit codes follow cli-output-spec: 0 success, 80-119 typed failures.
const (
	exitOK          = 0
	exitUsage       = 80
	exitUnavailable = 100
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(exitUsage)
	}
	switch os.Args[1] {
	case "serve":
		serve(os.Args[2:])
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
	fmt.Fprintln(os.Stderr, "\n  creneau serve [--port N] [--host H]")
	fmt.Fprintln(os.Stderr, "  creneau guide | help-json | version")
	fmt.Fprintln(os.Stderr, "\nNothing but the shell is implemented yet. See docs/pre-registration.md.")
}

// --- serve ----------------------------------------------------------------

func serve(args []string) {
	host, port := "127.0.0.1", 7800
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
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /_health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "creneau", "pid": os.Getpid()})
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tool": "creneau", "tool_version": Version})
	})
	mux.HandleFunc("GET /guide", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, guide())
	})
	// Every other route answers honestly rather than 404ing, so anybody who
	// finds this URL learns what it is instead of guessing.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]any{
			"ok": false,
			"error": map[string]any{
				"type":    "not_implemented",
				"message": "creneau has no booking domain yet — only the CLI shell exists",
				"suggestions": []string{
					"GET /guide",
					"https://github.com/javimosch/creneau/blob/master/docs/pre-registration.md",
				},
			},
		})
	})

	addr := host + ":" + strconv.Itoa(port)
	fmt.Fprintf(os.Stderr, "[serve] creneau %s listening on http://%s\n", Version, addr)
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		fail(exitUnavailable, "listen_failed", err.Error())
	}
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
			"status":    "SHELL ONLY — no booking domain is implemented yet.",
			"builds_on": "bkn (github.com/javimosch/bkn) — creneau is application code on its primitives.",
			"purpose": "Also a pre-registered experiment: is bkn's core the right size? " +
				"Criteria were committed before any code.",
		},
		"concepts": map[string]any{
			"creneau": "French for a time slot: one bookable interval on one calendar.",
			"pre-registration": "docs/pre-registration.md fixes the scope, the predictions and the " +
				"failure criteria. It is frozen — if it turns out wrong, that is a finding, not an edit.",
			"ledger": "docs/ledger.md records every gap hit in bkn and whether it was admitted as a " +
				"primitive or refused, with the rule cited. Refusals count as results.",
		},
		"commands": map[string]any{
			"meta":   []string{"creneau guide", "creneau help-json", "creneau version"},
			"server": []string{"creneau serve [--host H] [--port N]"},
		},
		"gotchas": []string{
			"Every route except /_health, /version and /guide returns 501 not_implemented. That is the current honest state, not a bug.",
			"The concurrency criterion (zero double-bookings at c=16) must be measured against a local instance, never through a CDN or a reverse proxy — that would measure the proxy.",
		},
		"see_also": []string{
			"https://github.com/javimosch/creneau",
			"https://github.com/javimosch/bkn",
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
			"serve":     cmd(none, []string{"--host <h>", "--port <n>"}),
			"guide":     cmd(none, none),
			"help-json": cmd(none, none),
			"version":   cmd(none, none),
		},
		"exit_codes": map[string]any{
			"0":   "success",
			"80":  "usage or invalid arguments",
			"100": "service unavailable",
		},
	}
}
