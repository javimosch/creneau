# Pre-registration

Committed **before any application code exists**. Everything here is a
prediction; nothing is a result.

## Why this document exists

bkn's primitives were derived from an audit of one 85k-line Node backend, and
then fit-checked against that same backend. Nine domains ported cleanly and
the core changed four times. That is a good result and a weak one: the tool was
shaped by the codebase it was measured against, so the measurement could only
ever agree.

Worse, the person running this benchmark helped design the thing being
benchmarked. The standard remedy is to fix the criteria in advance, publicly,
where they cannot be quietly relaxed once the answers start arriving. That is
all this file is.

## The claim under test

From bkn's [VISION](https://github.com/javimosch/bkn/blob/master/VISION.md):

> Most of what a backend does is not core, it is a script.

and its admission rule:

> Admit a primitive that removes a class of application code from every
> embedder. Refuse a query feature that only moves application code into bkn.

**Hypothesis:** a faithful booking backend — the Cal.com model, not a toy — can
be built as application code on bkn's nine primitives, with at most a small
number of additions to the core, and each addition will be a primitive rather
than a query feature.

## Scope: what "faithful" means

The domain is fixed here so it cannot shrink later to fit the answer. All of
this is in scope:

| # | Feature | Primitive under test |
|---|---|---|
| 1 | Organizer availability: weekly rules, a timezone, date overrides | `store` |
| 2 | Event types: duration, buffer before/after, minimum notice, daily cap | `store` |
| 3 | Slot computation: availability minus bookings minus buffers, over a date range | `store` query surface |
| 4 | Book a slot — **never double-book** | `store` atomics, `lock` |
| 5 | Cancel | `store` preconditions |
| 6 | **Reschedule** — cancel and rebook, atomically | *nothing — this is the point* |
| 7 | "My bookings" for a signed-in organizer | collection access policy, `owner` |
| 8 | Team event types on a shared calendar | collection access policy, `org` |
| 9 | Public booking page: a stranger books without an account | `hooks`, `read=public` |
| 10 | Reminder before a booking | `cron` + `script` |
| 11 | Outbound webhook on booked/cancelled | `bkn.http.fetch` |
| 12 | An `.ics` file per booking | `files` |

Explicitly **out** of scope, because they are known bkn non-goals and would
only re-derive answers already written down: realtime updates, full-text
search, an admin UI, payments, and calendar-provider sync (Google/CalDAV).

## Predictions

Stated now so that being wrong is visible.

- **P1.** Booking on a fixed grid is safe using `store put --if-absent` against
  a deterministic id (`<calendar>-<start>`), with no lock and no new primitive.
- **P2.** Arbitrary-duration bookings overlap rather than collide, so a
  deterministic id does not express the constraint. This will need `bkn.lock`
  around read-overlaps-then-write. Prediction: it works, and costs under 20
  lines.
- **P3.** Reschedule cannot be made atomic with today's primitives. The
  compensating pattern — take the new slot first, then release the old, and
  reconcile on failure — will be *correct but not obvious*, and will be the
  single largest piece of reasoning in the codebase.
- **P4.** Slot computation will want a range query that bkn already has
  (`start>=X`, `start<Y`, ordered, limited) and will **not** need a join, an
  OR, or an aggregate.
- **P5.** Every tenant-facing read will be served by a collection access
  policy. Only the public booking write will need a hook, because it is
  unauthenticated and does relational validation.

If P5 holds, bkn's access work paid for itself. If it does not, the honest
reading is that policies are not yet enough for an application.

## Failure criteria

Any one of these means the result is **negative** and gets written up as such
in `docs/ledger.md`. No renegotiation.

- **F1 — the core is too small.** More than **3** changes to bkn are admitted.
- **F2 — the query surface is too narrow.** A refused query feature forces more
  than **150 lines** of compensating application code.
- **F3 — access policies are insufficient.** Any *authenticated* tenant-facing
  endpoint needs a hook script that a collection policy should have covered.
- **F4 — the "less code" claim fails.** Application code exceeds **2,500
  lines**, excluding tests, comments and generated files.
- **F5 — the agent-first claim fails.** A fresh agent, given only the binary,
  cannot complete availability → slots → book from `guide` and `help-json` in
  **10 tool calls or fewer**, without reading source.
- **F6 — atomicity fails.** A single double-booking is observed under a
  concurrency test at c=16 sustained for 60s.

F6 is not negotiable down to "rare". One is a failure.

## Success criteria

Symmetry matters, so these are stated too. A **positive** result is all of:

- 0–3 core changes, each satisfying the admission rule, each recorded.
- Application code under 2,500 lines with the full scope above implemented.
- Zero double-bookings under sustained concurrency.
- The agent test passes.
- At least one finding that surprised the author — a benchmark that confirms
  everything confirms nothing.

## Method

- **Handler lines:** `find . -name '*.go' -not -name '*_test.go' | xargs wc -l`,
  reported as a single number with the command that produced it.
- **Core changes:** every one gets a row in `docs/ledger.md` — what was needed,
  admitted or refused, and the sentence of the admission rule that decided it.
  Refusals are recorded with the same weight as admissions.
- **Concurrency:** a suite that fires overlapping bookings at one calendar and
  asserts the invariant directly against the datastore, not against the API's
  own answer.
- **Agent test:** a subagent with the binary, `guide`, `help-json`, and no
  access to this repository's source. Transcript committed.

## What this cannot show

It is one domain. A positive result says booking fits, not that everything
fits — and booking was chosen partly *because* its hard parts (atomicity
without transactions, multi-tenancy) are ones bkn claims to handle. That is a
fair test of a specific claim, not a general proof, and the write-up will say
so.

The negative control lives elsewhere: privacy analytics, which is expected to
fail on time-bucketed aggregation, and is worth a day precisely to find out
where the rollup stops being enough.
