// Reserve a slot iff it is free.
//
// bkn has no transactions, so "is it free" and "take it" are two statements.
// The lease is what makes them behave as one: every writer for a calendar
// takes the same lock, so the overlap read cannot go stale before the write
// lands. This is P2's predicted shape, and it is why the atomic step lives in
// a script rather than in creneau itself — `lock` has no HTTP route.
function main(input) {
  var cal = input.calendar, start = input.start, end = input.end;
  if (!cal || !start || !end) {
    return { ok: false, status: 400, error: "calendar, start and end are required" };
  }

  var lock = bkn.lock.acquire("creneau-cal-" + cal, 15);
  if (!lock) return { ok: false, status: 409, error: "calendar busy, retry" };

  try {
    // Two intervals overlap iff s < end AND e > start. Touching ends do not
    // overlap, which is what makes back-to-back slots bookable.
    var clash = bkn.store.list("creneau/bookings", {
      where: { calendar: cal, status: "confirmed", start: { lt: end }, end: { gt: start } },
      limit: 1
    });
    if (clash && clash.length) {
      return { ok: false, status: 409, error: "slot taken", conflict: clash[0].id };
    }

    var rec = bkn.store.put("creneau/bookings", {
      calendar: cal,
      event: input.event || "",
      start: start,
      end: end,
      who: input.who || "",
      name: input.name || "",
      status: "confirmed",
      rescheduled_from: input.rescheduled_from || "",
      created_at: bkn.now()
    }, bkn.id());

    bkn.events.emit("creneau", "booking.created",
      { subject: rec.id, data: { calendar: cal, start: start, who: rec.who } });
    return { ok: true, status: 200, booking: rec };
  } finally {
    bkn.lock.release(lock.key, lock.owner);
  }
}
