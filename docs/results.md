# Results

Measured 2026-09-22 against the criteria in
[`pre-registration.md`](pre-registration.md), which were committed before any
creneau code was written. The findings behind them are in
[`ledger.md`](ledger.md).

**The headline: incomplete, not passed.** Five of the twelve in-scope features
were never built, so the success criterion — which requires the full scope —
is not met. Every criterion that *could* be evaluated holds. Two could not be
evaluated at all. Reported this way because a pre-registration you settle up
after seeing the numbers is a diary.

## Failure criteria

| # | Criterion | Result | Measurement |
|---|---|---|---|
| F1 | More than **3** core changes admitted | **holds** | 1 admitted, 3 refused ([ledger](ledger.md)) |
| F2 | A refused feature forces >**150 lines** of compensation | **holds, narrowly** | 137 lines across three bkn scripts — 91% of the ceiling |
| F3 | An authenticated tenant endpoint needs a hook a policy should cover | **not evaluated** | creneau never used bkn's identity layer; see below |
| F4 | Application code exceeds **2,500 lines** | **holds for the benchmarked scope** | two numbers, below |
| F5 | A fresh agent cannot do availability → slots → book in ≤10 calls | **FAILED, then fixed** | see below |
| F6 | A single double-booking under c=16 for 60s | **holds** | 0 overlapping pairs, asserted in SQL against the store |

## F4: both numbers, and where the line is

The pre-registered command reports the whole repository:

```
find . -name '*.go' -not -name '*_test.go' | xargs wc -l   ->  4,485
```

That exceeds the 2,500 ceiling. It is not the benchmarked scope. The scope
section put **an admin UI** explicitly out of scope, and 3,034 of those lines
are exactly that and its neighbours — the organizer page, the public board,
SSO, provisioning, recovery, invitations, outbound mail. None of it was
benchmarked and none of it exercises a bkn primitive the domain files do not.

The booking domain itself — `bkn.go`, `booking.go`, `domain.go` — is **931
lines**, plus 137 lines of bkn scripts.

Both numbers are published because choosing between them after seeing them is
the failure mode the pre-registration exists to prevent. The scope boundary
that separates them was fixed in advance, which is what makes the split
legitimate rather than convenient. A reader who rejects the boundary should
read F4 as failed.

## F5: the agent-first criterion failed

At first measurement, `creneau guide` and `creneau help-json` advertised
`GET /v1/slots` and `POST /v1/book`. Tenancy had moved every public route under
`/{lab}/`, and both documented routes returned **404**. A fresh agent following
the documented path could not book at all — not slowly, not at all.

This is not a documentation nit. The guide is the agent-first surface; when it
drifts from the routes, the product is broken for its primary caller and
nothing fails loudly. Fixed in this commit, with a test that fails if either
surface advertises a public route that is not lab-scoped.

Honesty note: F5 requires a *fresh* agent, and the only agent available had
written the code. The failure above is objective — the routes 404. The pass
after fixing is **not** an independent result, and is recorded as unverified.

## F3: not evaluated, and why that is a finding

creneau calls exactly two bkn surfaces: `/v1/store/` and `/v1/script/`. It
never used bkn's `auth`, `files`, `events`, `cron`, `hooks` or `lock` over
HTTP. Identity is creneau's own — capability tokens, hashed admin tokens,
sessions, and SSO through an external broker.

So the criterion was never exercised, and the reason is the surface asymmetry
recorded in the ledger: an out-of-process application cannot reach the
primitives that collection policies are built on. It did not decline bkn's
identity layer on the merits; it could not get to it and built its own.

## Scope not completed

Five of twelve in-scope features were not built:

| # | Feature | Primitive it would have tested |
|---|---|---|
| 2 (part) | Minimum notice, daily cap | `store` |
| 7 | "My bookings" for a signed-in organizer | collection access policy, `owner` |
| 8 | Team event types on a shared calendar | collection access policy, `org` |
| 10 | Reminder before a booking | `cron` + `script` |
| 11 | Outbound webhook on booked/cancelled | `bkn.http.fetch` |
| 12 | An `.ics` file per booking | `files` |

Those are precisely the features that would have tested `cron`, `files`,
`hooks` and the access policies — four of the nine primitives. **The benchmark
exercised `store` and `script` and nothing else.** Any claim about the core
being the right size rests on two primitives out of nine.

## What the experiment did establish

- Never double-booking without transactions is achievable on bkn's primitives,
  and the proof is a SQL assertion against the datastore rather than the API's
  own report. (During this run the harness produced a **false PASS twice** —
  once POSTing to a route tenancy had moved, once when sqlite3 could not open
  the database and an empty string slipped through the guard. It now fails
  closed. A concurrency test that cannot reach the code under test reports a
  perfect score forever.)
- The surface asymmetry finding, which none of the four predictions anticipated
  and which changed how the application was built.
- One admission that removed application code from every embedder, and three
  refusals recorded with the same weight.
