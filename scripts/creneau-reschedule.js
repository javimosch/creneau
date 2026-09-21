// Move a booking to a new time.
//
// P3 predicted this cannot be atomic, and it cannot. Two documents change and
// bkn has no transaction, so the honest design is a compensating sequence with
// a single visible failure mode, written down rather than hidden:
//
//   1. take the calendar lease        - serialises against other writers
//   2. verify the new window is free  - under the lease, so it stays free
//   3. write the new booking          - carries rescheduled_from
//   4. cancel the old, iff still confirmed  (compare-and-set)
//   5. if 4 fails, delete the new one - compensate, nobody saw it
//
// The gap is a crash between 3 and 4: the new booking exists and the old one
// is still confirmed, so the calendar holds two. Nothing here can close that
// window. `creneau reconcile` closes it afterwards by finding any confirmed
// booking whose rescheduled_from still points at a confirmed booking, and
// cancelling the older one. That is the cost of no transactions, paid in a
// reconciler rather than in a lie.
function main(input) {
  var id = input.id, start = input.start, end = input.end;
  if (!id || !start || !end) {
    return { ok: false, status: 400, error: "id, start and end are required" };
  }

  var old = bkn.store.get("creneau/bookings", id);
  if (!old) return { ok: false, status: 404, error: "booking not found" };
  if (old.status !== "confirmed") {
    return { ok: false, status: 409, error: "booking is " + old.status + ", not confirmed" };
  }

  var cal = old.calendar;
  var lock = bkn.lock.acquire("creneau-cal-" + cal, 15);
  if (!lock) return { ok: false, status: 409, error: "calendar busy, retry" };

  var created = null;
  try {
    var clash = bkn.store.list("creneau/bookings", {
      where: { calendar: cal, status: "confirmed", start: { lt: end }, end: { gt: start } },
      limit: 2
    });
    // The booking being moved is allowed to "clash" with itself.
    for (var i = 0; i < (clash || []).length; i++) {
      if (clash[i].id !== id) {
        return { ok: false, status: 409, error: "slot taken", conflict: clash[i].id };
      }
    }

    created = bkn.store.put("creneau/bookings", {
      calendar: cal, event: old.event, start: start, end: end,
      who: old.who, name: old.name, status: "confirmed",
      rescheduled_from: id, created_at: bkn.now()
    }, bkn.id());

    // Compare-and-set: only this caller may retire that booking.
    var cancelled = null;
    try {
      cancelled = bkn.store.patch("creneau/bookings", id,
        { status: "cancelled", cancelled_at: bkn.now(), rescheduled_to: created.id },
        { if: { status: "confirmed" } });
    } catch (e) {
      cancelled = null;
    }
    if (!cancelled) {
      bkn.store.delete("creneau/bookings", created.id);   // step 5
      return { ok: false, status: 409, error: "booking changed underneath the reschedule" };
    }

    bkn.events.emit("creneau", "booking.rescheduled",
      { subject: created.id, data: { from: id, calendar: cal, start: start } });
    return { ok: true, status: 200, booking: created, cancelled: id };
  } finally {
    bkn.lock.release(lock.key, lock.owner);
  }
}
