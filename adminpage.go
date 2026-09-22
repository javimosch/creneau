package main

// The organizer's page. Same principle as the board: a static client that speaks
// the public API. It asks for the admin key once, trades it for a session, and
// from then on holds only the session — so the long-lived tenant key never lands
// in localStorage.

import (
	"encoding/json"
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
.tabs{display:flex;gap:6px;margin:6px 0 14px;flex-wrap:wrap}
.tab{background:transparent;color:var(--mut);border:1px solid var(--bd);border-radius:999px;
 padding:7px 15px;font-size:14px;font-weight:600;cursor:pointer}
.tab.on{background:var(--ink);color:var(--bg);border-color:var(--ink)}
.agent code{display:block;background:var(--bg);border:1px solid var(--bd);border-radius:8px;
 padding:10px;margin:8px 0;word-break:break-all;white-space:pre-wrap}
code{font-family:var(--mono);font-size:12.5px}
.foot{color:var(--mut);font-size:12.5px;margin-top:8px}
</style></head><body><div class="wrap">
<div class="top"><div><h1 id="labname">__LAB__</h1><p class="sub" id="subline">admin</p></div>
<span><a id="pub" href="/__LABID__" style="margin-right:10px;font-size:13.5px">View the public board &rarr;</a>
<button class="g hidden" id="out">Sign out</button></span></div>

<div class="card" id="unlock">
  <h2>Sign in to manage this board</h2>
  <p class="h">Paste the admin token from your signup email. We swap it for a
  sign-in that lasts two weeks, and never keep the token itself in this browser.</p>
  <label for="key">Admin token</label>
  <input id="key" type="password" autocomplete="off" spellcheck="false">
  <div style="margin-top:12px"><button id="go">Sign in</button></div>
  <div id="sso" class="hidden" style="margin-top:16px;padding-top:16px;border-top:1px solid var(--bd)">
    <p class="h" style="margin:0 0 10px">Or use an account you have linked:</p>
    <div id="ssobtns"></div>
  </div>
  <div class="msg" id="umsg"></div>
  <p class="foot">Lost the token? <a href="#" id="recov">Email me a recovery link</a> &mdash;
  it goes to the address this board signed up with.</p>
</div>

<div id="app" class="hidden">
  <div class="tabs" role="tablist">
    <button class="tab on" data-tab="machines" role="tab">Machines</button>
    <button class="tab" data-tab="bookings" role="tab">Bookings</button>
    <button class="tab" data-tab="access" role="tab">Access</button>
  </div>

  <div class="panel" id="p-machines">
  <div class="card">
    <h2>Machines</h2>
    <p class="h">Your machines and when they can be booked. Closing a day (for maintenance,
    say) stops new bookings on that machine — bookings people already made stay put.</p>
    <table><thead><tr><th>Machine</th><th>Booking length</th><th>Gap between</th><th>Hours</th><th>Days open</th><th></th></tr></thead>
    <tbody id="machines"></tbody></table>
    <div class="msg" id="mmsg"></div>
  </div>

  <div class="card">
    <h2>Add a machine</h2>
    <p class="h">It can be booked straight away, on its own calendar — machines never block each other.</p>
    <div class="row">
      <div><label for="nid">Short name (in links)</label><input id="nid" placeholder="laser"></div>
      <div><label for="nname">Name people see</label><input id="nname" placeholder="Trotec Speedy 400"></div>
      <div><label for="nmin">Booking length (minutes)</label><input id="nmin" type="number" value="60"></div>
      <div><label for="ncool">Gap between bookings (minutes)</label><input id="ncool" type="number" value="0"></div>
    </div>
    <div class="row">
      <div><label for="nopen">Opening hours</label><input id="nopen" value="09:00-18:00"></div>
      <div><label for="ndays">Days open</label><input id="ndays" value="mon,tue,wed,thu,fri,sat"></div>
    </div>
    <button id="add">Add machine</button>
    <div class="msg" id="amsg"></div>
  </div>

  </div><!-- /p-machines -->

  <div class="panel hidden" id="p-access">
  <div class="card">
    <h2>Who can manage this board</h2>
    <p class="h">Link an account and you sign in with it instead of pasting the admin token.</p>
    <div id="access"></div>
    <div id="linkrow" style="margin-top:12px"></div>
    <div class="msg" id="gmsg"></div>
  </div>

  <div class="card">
    <h2>Robot account</h2>
    <p class="h">A login for software rather than a person — a booking bot, a door controller,
    a script that opens next week's slots. It signs in without a browser, and it works at every
    intrane app, not just this board.</p>
    <div id="agentbox"></div>
    <div class="msg" id="rmsg"></div>
  </div>
  </div><!-- /p-access -->

  <div class="panel hidden" id="p-bookings">
  <div class="card">
    <h2>Bookings <span class="pill" id="bcount"></span></h2>
    <p class="h">Every booking made on this board. Cancelling frees the slot immediately.</p>
    <table><thead><tr><th>Machine</th><th>When</th><th>Who</th><th></th></tr></thead>
    <tbody id="bookings"></tbody></table>
    <div class="msg" id="bmsg"></div>
  </div>
</div>
</div>
<script>
var LAB='__LABID__', SK='creneau.session.'+LAB, sess=null, PROVIDERS=__PROVIDERS__;
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
 // One link per configured provider. This used to hardcode PROVIDERS[0], so with
 // intrane first there was no way to attach a google or github identity at all.
 // The link offers belong beside the owner list they change, not in the header.
 if(PROVIDERS.length){var host=document.getElementById('linkrow');
  host.innerHTML='';
  PROVIDERS.forEach(function(p){
   var a=document.createElement('a');
   a.className='lnk';a.setAttribute('data-prov',p);
   a.style.cssText='display:inline-block;margin:0 8px 8px 0;padding:8px 14px;'+
    'border:1px solid var(--bd);border-radius:8px;text-decoration:none;color:inherit;font-size:13.5px';
   a.href='/'+LAB+'/auth/start?link='+encodeURIComponent(sess)+'&provider='+encodeURIComponent(p);
   a.textContent='Link my '+p+' account';
   host.appendChild(a)})}
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
   var i=esc(m.id);
   tr.innerHTML='<td><input data-f="name" data-m="'+i+'" value="'+esc(m.name)+'" style="min-width:150px">'+
     '<br><code>'+i+'</code></td>'+
     '<td><input data-f="minutes" data-m="'+i+'" type="number" value="'+(m.minutes||60)+'" style="width:72px"></td>'+
     '<td><input data-f="cooldown" data-m="'+i+'" type="number" value="'+(m.cooldown||0)+'" style="width:72px"></td>'+
     '<td><input data-f="open" data-m="'+i+'" placeholder="09:00-18:00" style="width:120px"></td>'+
     '<td><input data-f="days" data-m="'+i+'" placeholder="mon,tue,…" style="width:150px"></td>'+
     '<td style="white-space:nowrap"><button data-save="'+i+'" style="padding:6px 11px;font-size:12.5px">Save</button> '+
     '<button class="d" data-del="'+i+'">Remove</button></td>';
   mt.appendChild(tr);
   var tr2=document.createElement('tr');
   tr2.innerHTML='<td colspan="6" style="border-top:0;padding-top:0">'+
     '<span style="color:var(--mut);font-size:12.5px">maintenance:</span> '+
     '<input type="date" data-day="'+i+'" style="max-width:150px;display:inline-block;width:auto"> '+
     '<button class="d" data-close="'+i+'">Close day</button> '+
     '<button class="g" data-open="'+i+'" style="padding:6px 11px;font-size:12.5px">Reopen</button></td>';
   mt.appendChild(tr2)});
  mt.querySelectorAll('[data-save]').forEach(function(b){
   b.onclick=function(){
    var id=b.getAttribute('data-save'), body={};
    mt.querySelectorAll('[data-m="'+id+'"]').forEach(function(f){
     var k=f.getAttribute('data-f'), v=f.value.trim();
     if(v==='')return;
     body[k]=(k==='minutes'||k==='cooldown')?parseInt(v,10):v});
    b.disabled=true;b.textContent='…';
    api('/v1/machines/'+encodeURIComponent(id),{method:'PATCH',body:JSON.stringify(body)})
    .then(function(x){ b.disabled=false;b.textContent='Save';
     say('mmsg', x.s===200?('Saved '+id+'. Bookings already made are unchanged.')
       :((x.j.error&&x.j.error.message)||'Could not save.'), x.s===200?'ok':'no');
     if(x.s===200)load()})}});
  mt.querySelectorAll('[data-del]').forEach(function(b){
   b.onclick=function(){
    var id=b.getAttribute('data-del');
    if(!confirm('Remove '+id+'? This refuses if it still has upcoming bookings.'))return;
    b.disabled=true;
    api('/v1/machines/'+encodeURIComponent(id),{method:'DELETE'})
    .then(function(x){ b.disabled=false;
     say('mmsg', x.s===200?('Removed '+id+'.'):((x.j.error&&x.j.error.message)||'Could not remove.'),
       x.s===200?'ok':'no');
     if(x.s===200)load()})}});
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
  var ac=document.getElementById('access');
  var ows=(l.owners||[]);
  // An account you have already linked should not keep offering to link it.
  // The lab's own robot is an owner via intrane. Counting it would mark intrane
  // as "linked" when no person has linked anything, and hide the offer you want.
  var linked={};ows.forEach(function(o){
   if(o.provider && o.email!==(l.agent_handle||''))linked[o.provider]=1});
  document.querySelectorAll('.lnk').forEach(function(a){
   var pv=a.getAttribute('data-prov');if(!pv)return;
   if(linked[pv]){a.textContent='\u2713 '+pv+' linked';a.removeAttribute('href');
    a.style.opacity='.65';a.style.cursor='default'}
   else{a.textContent='Link my '+pv+' account';
    a.href='/'+LAB+'/auth/start?link='+encodeURIComponent(sess)+'&provider='+encodeURIComponent(pv);
    a.style.opacity='';a.style.cursor=''}});
  var h2='<table><thead><tr><th>Owner</th><th>Via</th></tr></thead><tbody>';
  if(!ows.length){h2+='<tr><td colspan="2" style="color:var(--mut)">No accounts linked yet — you are signed in with the admin token.</td></tr>'}
  ows.forEach(function(o){h2+='<tr><td>'+esc(o.email||o.sub)+'</td><td><code>'+esc(o.provider)+'</code></td></tr>'});
  h2+='</tbody></table>';
  renderAgent(l.agent_handle||'');
  if(l.trial_ends){h2+='<p class="h" style="margin:4px 0 0">Trial ends '+esc(l.trial_ends.slice(0,10))+'.</p>'}
  ac.innerHTML=h2;

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

if(PROVIDERS.length){
 var box=document.getElementById('ssobtns');
 PROVIDERS.forEach(function(p){
  var a=document.createElement('a');
  a.href='/'+LAB+'/auth/start?provider='+encodeURIComponent(p);
  a.textContent='Sign in with '+p;
  a.style.cssText='display:inline-block;margin:0 8px 8px 0;padding:9px 15px;border:1px solid var(--bd);'+
   'border-radius:8px;text-decoration:none;color:var(--ink);font-weight:600;font-size:14px';
  box.appendChild(a)});
 document.getElementById('sso').classList.remove('hidden')}
function renderAgent(handle){
 var box=document.getElementById('agentbox');if(!box)return;
 if(!handle){
  box.innerHTML='<p class="h">This board does not have one yet.</p>';
  var mk=document.createElement('button');mk.className='g';mk.textContent='Create a robot account';
  mk.onclick=makeAgent;box.appendChild(mk);return}
 box.className='agent';
 box.innerHTML=
  '<p class="h">Username: <code>'+esc(handle)+'</code></p>'+
  '<p class="h">Give the password to your script once; it signs in with no browser:</p>'+
  '<code>curl -u \''+esc(handle)+':PASSWORD\' \\\n  \'https://idp.intrane.fr/authorize?response_type=code&amp;client_id=…\'</code>'+
  '<p class="h">Lost the password? It cannot be read back — issue a new one, which stops the old '+
  'one working. Revoking deletes the account entirely.</p>';
 var row=document.createElement('div');row.style.marginTop='10px';
 var rot=document.createElement('button');rot.className='g';rot.textContent='Issue a new password';
 rot.style.marginRight='8px';rot.onclick=rotateAgent;
 var rev=document.createElement('button');rev.className='g';rev.textContent='Revoke this account';
 rev.onclick=revokeAgent;
 row.appendChild(rot);row.appendChild(rev);box.appendChild(row)}

function showSecret(title,handle,pw){
 var box=document.getElementById('agentbox');box.className='agent';
 box.innerHTML='<p class="h"><b>'+title+'</b> Copy it now — it is never shown again.</p>'+
  '<p class="h">Username: <code>'+esc(handle)+'</code></p>'+
  '<p class="h">Password:</p><code>'+esc(pw)+'</code>'+
  '<p class="h">Use them together as HTTP Basic auth against '+
  '<code>https://idp.intrane.fr/authorize</code>.</p>';
 var d=document.createElement('button');d.className='g';d.textContent='Done, I saved it';
 d.onclick=function(){load()};box.appendChild(d)}

function rotateAgent(){
 if(!confirm('Issue a new password? Anything using the current one stops working.'))return;
 api('/v1/agent/rotate',{method:'POST'}).then(function(x){
  if(x.s!==200){say('rmsg',(x.j.error&&x.j.error.message)||'Could not issue a new password.','no');return}
  showSecret('New password issued.',x.j.handle,x.j.password)})}

function revokeAgent(){
 if(!confirm('Delete this robot account? Anything signing in with it stops working.'))return;
 api('/v1/agent',{method:'DELETE'}).then(function(x){
  if(x.s!==200){say('rmsg',(x.j.error&&x.j.error.message)||'Could not revoke it.','no');return}
  say('rmsg','Revoked.','ok');load()})}

document.querySelectorAll('.tab').forEach(function(b){b.onclick=function(){
 document.querySelectorAll('.tab').forEach(function(x){x.classList.toggle('on',x===b)});
 var want='p-'+b.getAttribute('data-tab');
 document.querySelectorAll('.panel').forEach(function(pn){pn.classList.toggle('hidden',pn.id!==want)})}});
document.getElementById('go').onclick=unlock;
var rc=document.getElementById('recov');
if(rc){rc.onclick=function(e){e.preventDefault();
 say('umsg','Sending…');
 fetch('/v1/labs/recover',{method:'POST',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({lab:LAB})})
 .then(function(r){return r.json()})
 .then(function(){say('umsg','If this board has an email on file, a recovery link is on its way. It lasts 30 minutes.','ok')})
 .catch(function(){say('umsg','Network trouble — try again.','no')})}}
document.getElementById('key').addEventListener('keydown',function(e){if(e.key==='Enter')unlock()});
function makeAgent(){
 say('rmsg','Creating…');
 api('/v1/agent',{method:'POST'}).then(function(x){
  if(x.s!==200){say('rmsg',(x.j.error&&x.j.error.message)||'Could not create it.','no');return}
  say('rmsg','');showSecret('Robot account created.',x.j.handle,x.j.password)})}
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
	// Must be "[]" and never "null": json.Marshal of a nil slice yields null,
	// and PROVIDERS.length on null throws, which would abort the whole page
	// script — including the unlock handler.
	provs := "[]"
	if ssoEnabled() {
		list := ssoProviders()
		if list == nil {
			list = []string{}
		}
		if b, err := json.Marshal(list); err == nil {
			provs = string(b)
		}
	}
	page = strings.ReplaceAll(page, "__PROVIDERS__", provs)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex")
	_, _ = w.Write([]byte(page))
}
