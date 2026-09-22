#!/usr/bin/env bash
# F6: zero double-bookings at c=16 sustained for 60s.
#
# The invariant is asserted directly against the datastore with SQL, never
# against creneau's own answer — an API that reports success twice for the
# same slot will happily tell you it never double-booked (AGENTS.md rule 7).
#
#   CRENEAU_URL=http://127.0.0.1:7801 BKN_DB=/path/to/bkn.db ./test/concurrency.sh [seconds] [concurrency]
set -uo pipefail

SECS=${1:-60}
CONC=${2:-16}
CRENEAU_URL=${CRENEAU_URL:-http://127.0.0.1:7801}
BKN_DB=${BKN_DB:?set BKN_DB to the bkn SQLite file}
EVENT=${EVENT:-intro-30}

command -v sqlite3 >/dev/null || { echo "sqlite3 is required to assert the invariant" >&2; exit 2; }

# A deliberately small pool of starts, so 16 writers collide constantly rather
# than politely spreading out. Overlapping (not just identical) starts are the
# point: a deterministic id would catch the identical ones by itself.
STARTS=(
  2026-11-02T09:00:00Z 2026-11-02T09:15:00Z 2026-11-02T09:30:00Z
  2026-11-02T09:45:00Z 2026-11-02T10:00:00Z 2026-11-02T10:15:00Z
)

echo "firing c=$CONC at $CRENEAU_URL for ${SECS}s over ${#STARTS[@]} overlapping starts"
end=$(( $(date +%s) + SECS ))
tmp=$(mktemp -d)

worker() {
  local n=$1 ok=0 conflict=0 other=0
  while [ "$(date +%s)" -lt "$end" ]; do
    local at=${STARTS[$((RANDOM % ${#STARTS[@]}))]}
    local code
    code=$(curl -s -o /dev/null -w '%{http_code}' -m 10 -X POST "$CRENEAU_URL/v1/book" \
      -H 'Content-Type: application/json' \
      -d "{\"event\":\"$EVENT\",\"at\":\"$at\",\"who\":\"w$n@example.io\"}")
    case "$code" in
      200) ok=$((ok+1));;
      409) conflict=$((conflict+1));;
      *)   other=$((other+1));;
    esac
  done
  echo "$ok $conflict $other" > "$tmp/w$n"
}

for i in $(seq 1 "$CONC"); do worker "$i" & done
wait

booked=0; conflicts=0; errors=0
for f in "$tmp"/w*; do
  read -r a b c < "$f"; booked=$((booked+a)); conflicts=$((conflicts+b)); errors=$((errors+c))
done
rm -rf "$tmp"

echo "  accepted: $booked   refused(409): $conflicts   other: $errors"

# The invariant, straight from the datastore: two confirmed bookings on one
# calendar whose intervals overlap. Touching ends do not count.
overlaps=$(sqlite3 "file:$BKN_DB?mode=ro" "
  SELECT COUNT(*) FROM records a JOIN records b
    ON a.ns = b.ns AND a.coll = b.coll AND a.id < b.id
  WHERE a.ns = 'creneau' AND a.coll = 'bookings'
    AND json_extract(a.doc,'\$.calendar') = json_extract(b.doc,'\$.calendar')
    AND json_extract(a.doc,'\$.status') = 'confirmed'
    AND json_extract(b.doc,'\$.status') = 'confirmed'
    AND json_extract(a.doc,'\$.start') <  json_extract(b.doc,'\$.end')
    AND json_extract(b.doc,'\$.start') <  json_extract(a.doc,'\$.end');")

confirmed=$(sqlite3 "file:$BKN_DB?mode=ro" "
  SELECT COUNT(*) FROM records WHERE ns='creneau' AND coll='bookings'
    AND json_extract(doc,'\$.status')='confirmed';")

echo "  confirmed in store: $confirmed"
echo "  overlapping pairs:  $overlaps"

if [ "$overlaps" -ne 0 ]; then
  echo "F6 FAILED — $overlaps overlapping confirmed pairs. One is a failure." >&2
  sqlite3 "file:$BKN_DB?mode=ro" "
    SELECT json_extract(doc,'\$.start')||' -> '||json_extract(doc,'\$.end')||'  '||id
    FROM records WHERE ns='creneau' AND coll='bookings'
      AND json_extract(doc,'\$.status')='confirmed' ORDER BY 1;" >&2
  exit 1
fi

if [ "$booked" -eq 0 ]; then
  echo "F6 INCONCLUSIVE — nothing was accepted, so nothing was contended." >&2
  exit 2
fi

echo "F6 PASSED — 0 double-bookings, $booked accepted under c=$CONC for ${SECS}s"
