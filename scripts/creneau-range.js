// Bookings overlapping a window, for slot computation.
//
// Exists only because a range needs two bounds on one field and the HTTP API
// keeps just the first (ledger row 3). From a script the store expresses it
// directly. Delete this the day that row is fixed upstream.
function main(input) {
  var cal = input.calendar, from = input.from, to = input.to;
  if (!cal || !from || !to) {
    return { ok: false, status: 400, error: "calendar, from and to are required" };
  }
  var rows = bkn.store.list("creneau/bookings", {
    where: { calendar: cal, status: "confirmed", start: { lt: to }, end: { gt: from } },
    order_by: "start", limit: (input.limit || 500)
  });
  return { ok: true, status: 200, bookings: rows || [] };
}
