package main

// The page the booking-confirmation mail links to.
//
// A manage token used to exist only in the browser that made the booking, so a
// guest who booked on a phone could not cancel from a laptop. This renders the
// booking for anyone holding the token, and cancels on click — never on GET,
// because mail scanners prefetch links and a prefetched cancel would silently
// free somebody's slot.

import (
	"html"
	"net/http"
	"strings"
)

func bookingPageHandler(w http.ResponseWriter, r *http.Request) {
	labID := r.PathValue("lab")
	if err := validLabID(labID); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_value", err.Error())
		return
	}
	c := newBkn()
	l, err := loadLab(c, labID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "no such lab "+labID)
		return
	}
	id := r.PathValue("id")
	tok := r.URL.Query().Get("t")
	rec, gerr := c.get(ns, "bookings", id)
	if gerr != nil || !ownedBy(rec, labID) {
		writeErr(w, http.StatusNotFound, "not_found", "no such booking")
		return
	}
	if tok == "" || asStr(rec["manage_token"]) != tok {
		writeErr(w, http.StatusForbidden, "forbidden", "a valid manage link is required")
		return
	}
	when := whenLabel(rec)
	machine := machineLabel(l, rec)
	page := strings.NewReplacer(
		"__LABID__", labID,
		"__LABNAME__", html.EscapeString(l.Name),
		"__MACHINE__", html.EscapeString(machine),
		"__WHEN__", html.EscapeString(when),
		"__ID__", html.EscapeString(id),
		"__TOKEN__", html.EscapeString(tok),
		"__STATUS__", html.EscapeString(asStr(rec["status"])),
	).Replace(bookingHTML)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(page))
}

const bookingHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex">
<title>__LABNAME__ — your booking</title>
<style>
:root{--bg:#fbfbfc;--ink:#0b0e13;--mut:#616b78;--bd:#e3e6ea;--card:#fff;--accent:#f26b1d;--no:#c0392b}
@media(prefers-color-scheme:dark){:root{--bg:#0a0c10;--ink:#eceef1;--mut:#8892a0;--bd:#222833;--card:#11151b}}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--ink);font:15px/1.55 system-ui,-apple-system,Segoe UI,Roboto,sans-serif}
.wrap{max-width:520px;margin:8vh auto;padding:0 16px}
h1{font-size:20px;margin:0 0 4px}.sub{color:var(--mut);font-size:13.5px;margin:0 0 18px}
.card{background:var(--card);border:1px solid var(--bd);border-radius:12px;padding:20px}
.k{color:var(--mut);font-size:13px}.v{font-size:16px;font-weight:600;margin-bottom:10px}
button{margin-top:6px;padding:10px 18px;border:1px solid var(--bd);border-radius:8px;
 background:transparent;color:var(--no);font-weight:600;font-size:14.5px;cursor:pointer}
button[disabled]{opacity:.55;cursor:default}
.msg{margin-top:12px;font-size:14px}.ok{color:#1e8449}.bad{color:var(--no)}
a{color:inherit}.fine{color:var(--mut);font-size:12.5px;margin-top:14px}
</style></head><body><div class="wrap">
<h1>__LABNAME__</h1>
<p class="sub">Your booking</p>
<div class="card" id="card">
  <div class="k">Machine</div><div class="v">__MACHINE__</div>
  <div class="k">When</div><div class="v">__WHEN__</div>
  <div class="k">Status</div><div class="v" id="st">__STATUS__</div>
  <button id="cx">Cancel this booking</button>
  <div class="msg" id="msg"></div>
  <p class="fine">Nothing changes until you press the button.
    <a href="/__LABID__">See the whole board</a></p>
</div>
</div>
<script>
var LAB='__LABID__', ID='__ID__', T='__TOKEN__';
var cx=document.getElementById('cx'), msg=document.getElementById('msg');
if(document.getElementById('st').textContent.trim()!=='confirmed'){
 cx.disabled=true;msg.className='msg';msg.textContent='This booking is no longer active.'}
cx.onclick=function(){
 if(!confirm('Cancel this booking?\n\n__MACHINE__ — __WHEN__\n\nThe slot frees up for someone else straight away.'))return;
 cx.disabled=true;msg.className='msg';msg.textContent='Cancelling…';
 fetch('/'+LAB+'/v1/cancel',{method:'POST',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({id:ID,token:T})})
 .then(function(r){return r.json().then(function(j){return{s:r.status,j:j}})})
 .then(function(x){
  if(x.s===200||x.s===409){document.getElementById('st').textContent='cancelled';
   msg.className='msg ok';msg.textContent='Cancelled. The slot is free again.';return}
  cx.disabled=false;msg.className='msg bad';
  msg.textContent=(x.j.error&&x.j.error.message)||'Could not cancel.'})
 .catch(function(){cx.disabled=false;msg.className='msg bad';msg.textContent='Network trouble — try again.'})}
</script></body></html>`
