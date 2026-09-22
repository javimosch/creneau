package main

// The organizer's page. Same principle as the board: a static client that speaks
// the public API. It asks for the admin key once, trades it for a session, and
// from then on holds only the session — so the long-lived tenant key never lands
// in localStorage.

import (
	"html"
	"net/http"
	"strings"
)

const adminHTML = `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex">
<title>__LAB__ — admin</title>
<link rel="icon" href="data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 viewBox=%220 0 100 100%22%3E%3Ctext y=%22.9em%22 font-size=%2290%22%3E%F0%9F%9B%A0%EF%B8%8F%3C/text%3E%3C/svg%3E">
<style>
:root{--bg:#fbfbfc;--surf:#fff;--s2:#f2f4f7;--s3:#e6eaed;--ink:#0f1318;--mut:#59626d;
--bd:rgba(10,18,28,.12);--ac:#c2410c;--acb:#f97316;--acs:rgba(249,115,22,.11);--ok:#0f9d63;--no:#c8443b;
--mono:ui-monospace,"SF Mono",Menlo,Consolas,monospace;
--sans:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif}
@media(prefers-color-scheme:dark){:root{--bg:#0a0c0f;--surf:#13171c;--s2:#191e25;--s3:#222831;
--ink:#e8eaed;--mut:#8a919b;--bd:rgba(255,255,255,.085);--ac:#fdba74;--acb:#fb923c;
--acs:rgba(251,146,60,.12);--ok:#34d399;--no:#f87171}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--ink);font:15.5px/1.6 var(--sans)}
.wrap{max-width:900px;margin:0 auto;padding:24px 18px 70px}
h1{font-size:21px;margin:0 0 2px;letter-spacing:-.02em}
.sub{color:var(--mut);font-size:13.5px;margin:0 0 22px}
.card{border:1px solid var(--bd);border-radius:12px;background:var(--surf);padding:20px;margin-bottom:18px}
.card h2{margin:0 0 4px;font-size:16px}
.card p.h{margin:0 0 16px;color:var(--mut);font-size:13.5px}
label{display:block;font-size:12.5px;color:var(--mut);margin:0 0 4px}
input,select{width:100%;padding:10px 12px;border-radius:8px;border:1px solid var(--bd);
background:var(--bg);color:var(--ink);font:14.5px var(--sans)}
.row{display:grid;grid-template-columns:repeat(auto-fit,minmax(130px,1fr));gap:10px;margin-bottom:12px}
button{padding:10px 16px;border-radius:8px;border:0;background:var(--acb);color:#fff;
font:600 14px var(--sans);cursor:pointer}
button.g{background:transparent;border:1px solid var(--bd);color:var(--ink)}
button.d{background:transparent;border:1px solid var(--bd);color:var(--no);padding:6px 11px;font-size:12.5px}
button:disabled{opacity:.5;cursor:default}
table{width:100%;border-collapse:collapse;font-size:14px}
th{text-align:left;font:600 11px var(--mono);letter-spacing:.09em;text-transform:uppercase;
color:var(--mut);padding:0 8px 8px 0}
td{padding:9px 8px 9px 0;border-top:1px solid var(--bd);vertical-align:middle}
.msg{font-size:13.5px;min-height:18px;margin-top:10px}
.msg.ok{color:var(--ok)}.msg.no{color:var(--no)}
.pill{font:600 10.5px var(--mono);letter-spacing:.06em;text-transform:uppercase;
border:1px solid var(--bd);border-radius:99px;padding:3px 8px;color:var(--mut)}
.top{display:flex;align-items:baseline;justify-content:space-between;gap:12px;flex-wrap:wrap}
.hidden{display:none}
code{font-family:var(--mono);font-size:12.5px}
.foot{color:var(--mut);font-size:12.5px;margin-top:8px}
</style></head><body><div class="wrap">
<div class="top"><div><h1 id="labname">__LAB__</h1><p class="sub" id="subline">admin</p></div>
<button class="g hidden" id="out">Sign out</button></div>

<div class="card" id="unlock">
  <h2>Unlock this lab</h2>
  <p class="h">Paste the admin token you were given when the board was created.
  It is exchanged for a session that expires — the token itself is never stored in this browser.</p>
  <label for="key">Admin token</label>
  <input id="key" type="password" autocomplete="off" spellcheck="false">
  <div style="margin-top:12px"><button id="go">Unlock</button></div>
  <div class="msg" id="umsg"></div>
  <p class="foot">Lost it? <code>POST /v1/labs/recover</code> with your lab id or signup email.</p>
</div>

<div id="app" class="hidden">
  <div class="card">
    <h2>Machines</h2>
    <p class="h">Closing a day blocks new bookings on that machine. It does not cancel bookings already made.</p>
    <table><thead><tr><th>Machine</th><th>Slot</th><th>Cooldown</th><th>Close a day</th></tr></thead>
    <tbody id="machines"></tbody></table>
    <div class="msg" id="mmsg"></div>
  </div>

  <div class="card">
    <h2>Add a machine</h2>
    <p class="h">It becomes bookable immediately, on its own calendar.</p>
    <div class="row">
      <div><label for="nid">id</label><input id="nid" placeholder="laser"></div>
      <div><label for="nname">name</label><input id="nname" placeholder="Trotec Speedy 400"></div>
      <div><label for="nmin">slot (min)</label><input id="nmin" type="number" value="60"></div>
      <div><label for="ncool">cooldown (min)</label><input id="ncool" type="number" value="0"></div>
    </div>
    <div class="row">
      <div><label for="nopen">open</label><input id="nopen" value="09:00-18:00"></div>
      <div><label for="ndays">days</label><input id="ndays" value="mon,tue,wed,thu,fri,sat"></div>
    </div>
    <button id="add">Add machine</button>
    <div class="msg" id="amsg"></div>
  </div>

  <div class="card">
    <h2>Bookings <span class="pill" id="bcount"></span></h2>
    <p class="h">Everything confirmed in this lab. Cancelling here frees the slot at once.</p>
    <table><thead><tr><th>Machine</th><th>When</th><th>Who</th><th></th></tr></thead>
    <tbody id="bookings"></tbody></table>
    <div class="msg" id="bmsg"></div>
  </div>
</div>
</div>
<script>
var LAB='__LABID__', SK='creneau.session.'+LAB, sess=null;
function api(path,opts){opts=opts||{};opts.headers=opts.headers||{};
 if(sess)opts.headers['Authorization']='Bearer '+sess;
 if(opts.body)opts.headers['Content-Type']='application/json';
 return fetch('/'+LAB+path,opts).then(function(r){return r.json().then(function(j){return{s:r.status,j:j}})})}
function say(el,txt,cls){var e=document.getElementById(el);e.className='msg'+(cls?' '+cls:'');e.textContent=txt}
function esc(s){var d=document.createElement('div');d.textContent=s==null?'':s;return d.innerHTML}

function unlock(){
 var key=document.getElementById('key').value.trim();
 if(!key){say('umsg','Paste the token first.','no');return}
 say('umsg','checking…');
 fetch('/'+LAB+'/v1/session',{method:'POST',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({admin_token:key})})
 .then(function(r){return r.json().then(function(j){return{s:r.status,j:j}})})
 .then(function(x){
  if(x.s!==200){say('umsg',(x.j.error&&x.j.error.message)||'That token was not accepted.','no');return}
  sess=x.j.session; try{localStorage.setItem(SK,JSON.stringify({t:sess,exp:x.j.expires}))}catch(e){}
  document.getElementById('key').value='';
  enter(x.j.expires)})
 .catch(function(){say('umsg','Network trouble.','no')})}

function enter(exp){
 document.getElementById('unlock').classList.add('hidden');
 document.getElementById('app').classList.remove('hidden');
 document.getElementById('out').classList.remove('hidden');
 if(exp)document.getElementById('subline').textContent='admin · session valid until '+exp.slice(0,10);
 load()}

function load(){
 api('/v1/admin').then(function(x){
  if(x.s!==200){signout();return}
  var l=x.j.lab;
  document.getElementById('labname').textContent=l.name||l.id;
  var mt=document.getElementById('machines');mt.innerHTML='';
  (l.machines||[]).forEach(function(m){
   var tr=document.createElement('tr');
   tr.innerHTML='<td><b>'+esc(m.name)+'</b><br><code>'+esc(m.id)+'</code></td>'+
     '<td>'+(m.minutes||60)+' min</td><td>'+(m.cooldown||0)+' min</td>'+
     '<td><input type="date" data-day="'+esc(m.id)+'" style="max-width:150px">'+
     ' <button class="d" data-close="'+esc(m.id)+'">Close</button>'+
     ' <button class="g" data-open="'+esc(m.id)+'" style="padding:6px 11px;font-size:12.5px">Reopen</button></td>';
   mt.appendChild(tr)});
  mt.querySelectorAll('[data-close],[data-open]').forEach(function(b){
   b.onclick=function(){
    var id=b.getAttribute('data-close')||b.getAttribute('data-open');
    var reopen=!!b.getAttribute('data-open');
    var day=mt.querySelector('[data-day="'+id+'"]').value;
    if(!day){say('mmsg','Pick a date first.','no');return}
    api('/v1/machines/'+encodeURIComponent(id)+'/closures',
      {method:'POST',body:JSON.stringify({day:day,reopen:reopen})})
    .then(function(x){ say('mmsg', x.s===200 ? (reopen?'Reopened '+day+'.':'Closed '+day+' on '+id+'.')
      : ((x.j.error&&x.j.error.message)||'Could not change that day.'), x.s===200?'ok':'no')})}});
  var bt=document.getElementById('bookings');bt.innerHTML='';
  document.getElementById('bcount').textContent=x.j.count+' confirmed';
  if(!x.j.bookings.length){bt.innerHTML='<tr><td colspan="4" style="color:var(--mut)">Nothing booked yet.</td></tr>'}
  x.j.bookings.forEach(function(b){
   var tr=document.createElement('tr');
   tr.innerHTML='<td><code>'+esc(b.machine)+'</code></td><td>'+esc((b.start||'').replace('T',' ').replace('Z',' UTC'))+
     '</td><td>'+esc(b.who)+'</td><td><button class="d" data-cancel="'+esc(b.id)+'">Cancel</button></td>';
   bt.appendChild(tr)});
  bt.querySelectorAll('[data-cancel]').forEach(function(b){
   b.onclick=function(){
    b.disabled=true;b.textContent='…';
    api('/v1/admin/cancel',{method:'POST',body:JSON.stringify({id:b.getAttribute('data-cancel')})})
    .then(function(x){ if(x.s===200||x.s===409){load()} else {b.disabled=false;b.textContent='Cancel';
      say('bmsg',(x.j.error&&x.j.error.message)||'Could not cancel.','no')}})}});
 })}

function addMachine(){
 var body={id:document.getElementById('nid').value.trim(),
  name:document.getElementById('nname').value.trim(),
  minutes:parseInt(document.getElementById('nmin').value,10)||60,
  cooldown:parseInt(document.getElementById('ncool').value,10)||0,
  open:document.getElementById('nopen').value.trim(),
  days:document.getElementById('ndays').value.trim()};
 if(!body.id){say('amsg','An id is required.','no');return}
 say('amsg','adding…');
 api('/v1/machines',{method:'POST',body:JSON.stringify(body)}).then(function(x){
  if(x.s===200){say('amsg','Added '+body.id+'.','ok');
   document.getElementById('nid').value='';document.getElementById('nname').value='';load()}
  else say('amsg',(x.j.error&&x.j.error.message)||'Could not add it.','no')})}

function signout(){
 if(sess){api('/v1/session',{method:'DELETE'}).catch(function(){})}
 try{localStorage.removeItem(SK)}catch(e){}
 sess=null;
 document.getElementById('app').classList.add('hidden');
 document.getElementById('out').classList.add('hidden');
 document.getElementById('unlock').classList.remove('hidden');
 document.getElementById('subline').textContent='admin';
 say('umsg','Signed out. The session was revoked on the server.','ok')}

document.getElementById('go').onclick=unlock;
document.getElementById('key').addEventListener('keydown',function(e){if(e.key==='Enter')unlock()});
document.getElementById('add').onclick=addMachine;
document.getElementById('out').onclick=signout;
try{var saved=JSON.parse(localStorage.getItem(SK)||'null');
 if(saved&&saved.t){sess=saved.t;enter(saved.exp)}}catch(e){}
</script></body></html>`

// labAdminPageHandler hands over the organizer client. No auth here on purpose:
// the page is inert until someone unlocks it, and every call it makes is
// authorised server-side. noindex, because a lab's admin page is not a document.
func labAdminPageHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("lab")
	if validLabID(id) != nil {
		writeErr(w, http.StatusNotFound, "not_found", "no such lab")
		return
	}
	l, err := loadLab(newBkn(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "no such lab "+id)
		return
	}
	page := adminHTML
	page = strings.ReplaceAll(page, "__LAB__", html.EscapeString(l.Name))
	page = strings.ReplaceAll(page, "__LABID__", l.ID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex")
	_, _ = w.Write([]byte(page))
}
