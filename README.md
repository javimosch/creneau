# creneau

A booking backend you own. Availability, slots, and reservations that never
double-book — one CLI that is the server, the client and the admin tool.

*Créneau* is French for a time slot.

> **Built on [bkn](https://github.com/javimosch/bkn)** — a single-binary
> backend core: namespaced document collections, typed settings, identity,
> files, events, cron, hooks and a sandboxed script runtime over embedded
> SQLite. creneau is application code on top of it, and there is deliberately
> very little of it.

```sh
creneau guide                       # the mental model, embedded in the binary
creneau help-json                   # machine-readable command catalog

creneau availability set --tz Europe/Paris --mon 09:00-17:00 --tue 09:00-17:00
creneau event create intro-30 --minutes 30 --buffer-after 10 --min-notice 4h

creneau slots --event intro-30 --from 2026-09-10 --to 2026-09-12
creneau book  --event intro-30 --at 2026-09-10T14:00:00Z --who ada@example.io
creneau bookings list --upcoming
```

## This repo is also an experiment

creneau exists to answer a question about bkn that its own author cannot
answer by looking at his own code: **is the core the right size?**

bkn's primitives were derived from an audit of one 85k-line Node backend. That
made the first fit-check a confirmation — the tool was shaped by the codebase
it was then measured against. This one is different on purpose: the domain is
somebody else's (the booking model is Cal.com's, faithfully), and the pass/fail
criteria were **written down and committed before a line of application code
existed**.

Read [`docs/pre-registration.md`](docs/pre-registration.md) first. It states
the hypotheses, the measurements, and — the part that matters — exactly what
result would mean bkn's scope is *wrong*.

The findings land in [`docs/ledger.md`](docs/ledger.md): every gap hit, and
whether it was admitted into bkn as a primitive or refused, with the rule
cited. A benchmark whose author can move the goalposts is not a benchmark.

## Status

**The booking core works.** Availability, event types, slot computation,
booking, cancel and reschedule are implemented, plus a public booking surface a
stranger can use without an account. The invariant holds under load: **0
double-bookings across 7,467 attempts at c=16 for 60s**, asserted with SQL
against the datastore rather than against creneau's own answer.

Five of the twelve scoped features are **not** built yet — organizer-scoped
"my bookings" (7), team calendars (8), the reminder cron (10), outbound
webhooks (11) and `.ics` files (12). The pre-registration is the deliverable,
so those are listed as missing rather than quietly dropped, and the scorecard
in [`docs/ledger.md`](docs/ledger.md) marks every criterion it has not earned
with `—` instead of a guess.

```sh
export BKN_URL=http://127.0.0.1:7799      # a running bkn
creneau install                           # declare collections, load the scripts

creneau availability set --calendar default --tz Europe/Paris \
        --mon 09:00-12:00,13:00-17:00 --tue 09:00-17:00
creneau event create intro-30 --minutes 30 --buffer-after 10 --min-notice 4h

creneau slots --event intro-30 --from 2026-10-05 --to 2026-10-05
creneau book  --event intro-30 --at 2026-10-05T12:00:00Z --who ada@example.io
creneau reschedule <id> --at 2026-10-05T14:00:00Z
creneau bookings list --upcoming
```

Running at <https://creneau-dk3.intrane.fr> — dk3, fronted by dk1's Traefik
over an SSH reverse tunnel, alongside the bkn instance it is built on.

### What it found

The first result is not about booking at all. bkn's primitives are reachable
from the CLI, from the script host and from its Go packages — but its **HTTP
API exposes a strict subset**, and the Go packages are all `internal/`. An
application in a separate process therefore cannot call `lock`, `putIfAbsent`
or a two-bound range directly; it has to push that logic into a script.

That is bkn's own thesis working as written — *most of what a backend does is
not core, it is a script* — and it is **not** what P1, P2 and P4 predicted. All
three assumed the application would call the primitive itself. The
[ledger](docs/ledger.md) records it rather than rewriting the prediction.

## Why booking

Three reasons it is a harder test than it looks:

- **Never double-book** is a concurrency problem, and bkn has
  [no transactions](https://github.com/javimosch/bkn/blob/master/AGENTS.md) —
  deliberately. Either its atomic primitives express "reserve this iff free",
  or they do not, and both answers are worth having.
- **Reschedule** is two writes that must both happen or neither. There is no
  primitive for that at all. What it costs to live without one is the sharpest
  question in this repo.
- **Multi-tenant from day one** — personal calendars, team calendars, and a
  public booking page that strangers use without an account. That exercises
  bkn's collection access policies rather than taking their word for it.

## Licence

MIT.
