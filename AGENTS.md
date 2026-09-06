# Working in this repo

creneau is two things at once, and the second one constrains the first:

1. a booking backend somebody could actually run;
2. a pre-registered experiment about whether
   [bkn](https://github.com/javimosch/bkn)'s core is the right size.

## The rules that exist because of (2)

1. **`docs/pre-registration.md` is frozen.** It was committed before any
   application code. Do not edit the scope, the predictions, or the failure
   criteria — not to clarify them, not to fix a typo in a threshold. If it
   turns out to be wrong, that is a finding: record it in the ledger and leave
   the original standing. A pre-registration you can edit is a diary.

2. **Every gap gets a ledger row before it gets a workaround.** If bkn cannot
   do something, write the row in `docs/ledger.md` first — what was missing,
   admitted or refused, the rule that decided it. Then write the code. Rows
   added afterwards get written to justify what was already built.

3. **Refusals are results.** "bkn should not grow this" is a finding worth as
   much as "bkn needs this", and is the more likely correct answer. The
   admission rule refuses query features on purpose.

4. **No scope creep toward passing.** The feature table in the
   pre-registration is the deliverable. Dropping reschedule because it is hard
   would delete the single most informative test in the repo.

## The rules that exist because of (1)

5. **Application code only.** If something belongs in bkn, it goes in bkn — as
   a ledger row and an upstream change, not as a helper here. This repo is
   supposed to be small; that is the measurement.

6. **Agent-first, per the
   [spec family](https://cli-specs.intrane.fr).** stdout is data, stderr is
   context, exit codes are semantic, `guide` and `help-json` are embedded in
   the binary. F5 tests this directly: an agent that cannot drive the tool from
   the binary alone is a failed criterion, not a documentation problem.

7. **Never double-book** is the invariant. Test it against the datastore, not
   against the API's own answer — an API that reports success twice for the
   same slot will happily tell you it never double-booked.

## Measuring

```sh
# application lines, the F4 number
find . -name '*.go' -not -name '*_test.go' -not -path './vendor/*' | xargs wc -l | tail -1
```
