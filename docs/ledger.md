# Findings ledger

Every gap hit while building creneau on bkn, and what was done about it.

Refusals are recorded with the same weight as admissions: a benchmark that only
lists what got added is a feature request in disguise.

The rule each row is judged against, from
[bkn's VISION](https://github.com/javimosch/bkn/blob/master/VISION.md):

> Admit a primitive that removes a class of application code from every
> embedder. Refuse a query feature that only moves application code into bkn.

| # | What was missing | Verdict | Why | bkn change |
|---|---|---|---|---|
| 1 | `putIfAbsent` over HTTP | **Refused** | The primitive exists in the core, the CLI and the script host — only the HTTP API lacks a route. bkn's answer to "my application needs this" is a script, and `POST /v1/script/{name}/run` is exposed. Adding a route would not remove application code, it would move it from JS to the caller's language. | none |
| 2 | `lock` over HTTP | **Refused** | Same shape as row 1, and the same answer. Note the cost, which P2 did not predict: the atomic step has to live in a script, so creneau's booking logic is split across two languages. That is a real tax, paid once. | none |
| 4 | Installing a script over HTTP | **Refused** | There is no create route — `bkn script create` is CLI-only, so `creneau install` shells out to the binary rather than pretending it can provision a remote instance. Provisioning is an administrative act on a machine you already have, not something an application does to a server it merely talks to. | none |
| 3 | Two bounds on one field over HTTP (`?start=gte:A&start=lt:B`) | **Admitted** | Not a new query feature — the six operators already exist and both other surfaces express a range. `storeList` reads `vals[0]`, so a repeated query parameter silently collapses to the first and the second bound is **dropped without an error**. Making the existing surface reachable removes a script from every embedder that needs a range read. | `internal/server/server.go`: iterate all `vals`, not `vals[0]` |

## The surface asymmetry behind rows 1-3

All three rows are one finding seen from three angles, and it is the first
result this experiment produced:

> bkn's primitives are reachable from the **CLI**, from the **script host**, and
> from the **Go packages** — but the **HTTP API** exposes a strict subset, and
> the Go packages are all `internal/`, so an out-of-process application cannot
> import them.

An application that is a separate process therefore cannot call `lock`,
`putIfAbsent`, or a range query directly. It must push that logic into a script
and invoke it. That is bkn's own thesis working exactly as written —

> Most of what a backend does is not core, it is a script.

— and it is **not** what P1, P2 and P4 predicted. All three assumed the
application would call the primitive itself. They are not wrong about the
primitive; they are wrong about who calls it. Recorded here rather than
quietly rewritten, per the rule that a pre-registration you can edit is a diary.

## How the predictions held

Features 1-6 and 9 of the scoped twelve are implemented. 7, 8, 10, 11 and 12
are not, so this is an interim reading and the scorecard below says so. Nothing
here is a final result.

| | Prediction | Outcome |
|---|---|---|
| **P1** | fixed-grid booking is safe with `put --if-absent` on a deterministic id, no lock | **Not used.** Arbitrary durations are the general case and P2's lease subsumes the grid case, so the deterministic id was never needed. `putIfAbsent` is also unreachable over HTTP (row 1). Untested as stated. |
| **P2** | arbitrary durations need `bkn.lock` around read-overlaps-then-write; works, under 20 lines | **Held.** `scripts/creneau-book.js` is 44 lines with comments; the lease + overlap read + write is 18. |
| **P3** | reschedule cannot be atomic; the compensating pattern will be correct but not obvious, and the largest piece of reasoning in the repo | **Held, exactly.** It is the longest comment block here, it needed a fifth compensating step, and it still leaves one window open — a crash between writing the new booking and retiring the old. `creneau reconcile` closes it afterwards. The honest cost of no transactions is a reconciler. |
| **P4** | slot computation wants a range bkn already has; no join, OR or aggregate | **Half right, and the half it got wrong is the interesting one.** No join, OR or aggregate was needed. But the range is not reachable over HTTP, so it had to move into a script (row 3). Right about the primitive, wrong about who can call it. |
| **P5** | every tenant-facing read served by a collection policy; only the public write needs a hook | **Not yet tested** — features 7 and 8 are unbuilt. Already deviating though: the public booking write needed **no bkn hook at all**, because creneau is itself an HTTP server. A hook is for when bkn is the only server you have. |

**The finding that surprised the author** (a success criterion in its own right,
since a benchmark that confirms everything confirms nothing): P1, P2 and P4 all
predicted the *application* would call bkn's primitives. It cannot. The
primitives are real and they work, but an out-of-process application reaches
them only by pushing logic into a script — and that splits the domain across
two languages. bkn's thesis says exactly this; the predictions did not.

## Score against the pre-registered criteria

**Interim — 7 of 12 features built.** Rows are marked `—` where the evidence
does not exist yet, rather than guessed at.

| Criterion | Threshold | Result |
|---|---|---|
| F1 core changes admitted | ≤ 3 | **1** (row 3) — verdict recorded, upstream change not yet made |
| F2 compensating code for a refusal | ≤ 150 lines | **136** lines of JS for rows 1-2 |
| F3 authenticated endpoint needing a hook | none | — (features 7, 8 unbuilt) |
| F4 application lines | < 2,500 | **1,240** Go + 136 JS = **1,376** |
| F5 agent completes a booking | ≤ 10 calls | — (not run) |
| F6 double-bookings at c=16 | 0 | **PASSED — 0** |

### F6, in full

`./test/concurrency.sh 60 16` against a local bkn, asserting with SQL against
the datastore rather than against creneau's own answer:

```
firing c=16 for 60s over 6 overlapping starts
  accepted: 3   refused(409): 7464   other: 0
  confirmed in store: 3
  overlapping pairs:  0
```

7,467 attempts, 3 accepted. Three is not a weak number here — it is the
**maximum independent set** of those six overlapping intervals (09:00-09:30,
09:30-10:00, 10:00-10:30). Every booking that could be taken was taken, and
every one that could not be was refused.
