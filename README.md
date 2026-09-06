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

**Scoping.** Nothing is implemented. The pre-registration is committed; the
code is not written. That order is the whole point.

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
