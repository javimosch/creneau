package main

// The board a human uses. It is a static client: the browser calls the same two
// public routes an agent calls — GET /v1/slots and POST /v1/book — and renders
// them. Nothing is server-rendered; this handler only hands over the client.
// Same origin as the API, so it needs no CORS.

import (
	"encoding/json"
	"html"
	"net/http"
	"strings"
)

const boardHTML = `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<link rel="icon" href="data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 viewBox=%220 0 100 100%22%3E%3Ctext y=%22.9em%22 font-size=%2290%22%3E%F0%9F%9B%A0%EF%B8%8F%3C/text%3E%3C/svg%3E">
<title>__LAB__ — machine board</title>
<style>
:root{--bg:#fbfbfc;--surf:#fff;--s2:#f2f4f7;--s3:#e6eaed;--ink:#0f1318;--mut:#59626d;
--bd:rgba(10,18,28,.12);--ac:#c2410c;--acb:#f97316;--acs:rgba(249,115,22,.11);--ok:#0f9d63;--no:#c8443b;
--mono:ui-monospace,"SF Mono",Menlo,Consolas,monospace;
--sans:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif}
@media(prefers-color-scheme:dark){:root{--bg:#0a0c0f;--surf:#13171c;--s2:#191e25;--s3:#222831;
--ink:#e8eaed;--mut:#8a919b;--bd:rgba(255,255,255,.085);--ac:#fdba74;--acb:#fb923c;
--acs:rgba(251,146,60,.12);--ok:#34d399;--no:#f87171}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--ink);font:15.5px/1.6 var(--sans)}
.wrap{max-width:1060px;margin:0 auto;padding:22px 18px 60px}
header{display:flex;align-items:baseline;justify-content:space-between;gap:14px;flex-wrap:wrap;margin-bottom:6px}
h1{font-size:21px;margin:0;letter-spacing:-.02em}
.sub{color:var(--mut);font-size:13.5px;margin:2px 0 18px}
.adm{display:inline-block;margin-top:6px;font-size:13.5px;text-decoration:none;
 border:1px solid var(--bd);border-radius:8px;padding:6px 11px;color:inherit}
.empty{padding:22px;text-align:center;color:var(--mut);font-size:14px;line-height:1.6}
.empty a{color:inherit}
.days{display:flex;gap:7px;flex-wrap:wrap;margin-bottom:16px}
.day{font:600 12.5px var(--sans);padding:7px 12px;border-radius:8px;border:1px solid var(--bd);
background:var(--surf);color:var(--ink);cursor:pointer}
.day[aria-pressed=true]{background:var(--acs);border-color:var(--acb);color:var(--ac)}
.frame{border:1px solid var(--bd);border-radius:13px;background:var(--surf);overflow:hidden}
.scr{overflow-x:auto}
table{border-collapse:separate;border-spacing:0;width:100%;min-width:620px;table-layout:fixed}
thead th:first-child{width:186px}
.mc{width:186px;overflow:hidden;text-overflow:ellipsis}
th,td{padding:0;text-align:left;vertical-align:middle}
.hh{font:600 10.5px var(--mono);color:var(--mut);text-align:center;padding:12px 0 8px}
.mc{padding:8px 14px;height:52px;white-space:nowrap;border-top:1px solid var(--bd);
position:sticky;left:0;background:var(--surf);z-index:2;min-width:150px;font-weight:650;font-size:13.5px}
.cw{border-top:1px solid var(--bd);padding:6px 3px;height:52px}
.c{width:100%;height:38px;border-radius:7px;border:1px solid var(--bd);background:transparent;
cursor:pointer;font:600 10.5px var(--mono);color:var(--mut);transition:.12s}
.c:hover:not(:disabled){border-color:var(--acb);background:var(--acs);color:var(--ac)}
.c:disabled{background:var(--s3);border-color:transparent;cursor:not-allowed}
.c.mine{background:var(--acb);border-color:var(--acb);color:#fff}
.legend{display:flex;gap:15px;flex-wrap:wrap;padding:11px 15px;border-top:1px solid var(--bd);
background:var(--s2);font-size:12px;color:var(--mut)}
.legend i{display:inline-block;width:10px;height:10px;border-radius:3px;vertical-align:-1px;
margin-right:5px;border:1px solid var(--bd)}
.legend .t{background:var(--s3);border-color:transparent}
dialog{border:1px solid var(--bd);border-radius:13px;background:var(--surf);color:var(--ink);
padding:0;max-width:390px;width:calc(100% - 32px)}
dialog::backdrop{background:rgba(0,0,0,.45)}
.dlg{padding:20px}
.dlg h2{margin:0 0 4px;font-size:16.5px}
.dlg p{margin:0 0 14px;color:var(--mut);font-size:13.5px}
.dlg input{width:100%;padding:11px 13px;border-radius:9px;border:1px solid var(--bd);
background:var(--bg);color:var(--ink);font:14.5px var(--sans);margin-bottom:10px}
.row{display:flex;gap:9px;justify-content:flex-end}
button.p{padding:10px 17px;border-radius:9px;border:0;background:var(--acb);color:#fff;
font:600 14px var(--sans);cursor:pointer}
button.g{padding:10px 15px;border-radius:9px;border:1px solid var(--bd);background:transparent;
color:var(--ink);font:500 14px var(--sans);cursor:pointer}
.msg{font-size:13px;min-height:18px;margin-top:8px}
.msg.ok{color:var(--ok)}.msg.no{color:var(--no)}
.mine{border:1px solid var(--bd);border-radius:11px;background:var(--surf);padding:14px 16px;margin-bottom:16px}
.mine h3{margin:0 0 9px;font-size:13px;letter-spacing:.06em;text-transform:uppercase;color:var(--mut)}
.mb{display:flex;align-items:center;gap:12px;justify-content:space-between;
padding:8px 0;border-top:1px solid var(--bd);font-size:14px;flex-wrap:wrap}
.mb:first-of-type{border-top:0}
.mb .w{color:var(--mut);font-size:13px}
.mb button{padding:6px 12px;border-radius:7px;border:1px solid var(--bd);background:transparent;
color:var(--no);font:600 12.5px var(--sans);cursor:pointer}
.mb button:hover{border-color:var(--no)}
.mb.done{opacity:.55}
.api{margin-top:16px;font:12px var(--mono);color:var(--mut)}
.api a{color:var(--ac)}
</style></head><body><div class="wrap">
<header><h1>__LAB__ — machine board</h1>
<a class="adm" href="/__LABID__/admin">Organizer? Manage this board &rarr;</a></header>
<p class="sub" id="tzline"></p>
<div id="mine"></div>
<div class="days" id="days"></div>
<div class="frame"><div class="scr"><table><thead id="th"></thead><tbody id="tb"></tbody></table></div>
<div class="legend"><span><i></i>free — click to book</span><span><i class="t"></i>taken</span>
<span id="stat"></span></div></div>
<p class="api">Same two calls an agent makes:
<span id="apih"></span> · <a href="/guide">GET /guide</a></p>
</div>
<dialog id="dlg"><div class="dlg">
<h2 id="dt">Book</h2><p id="dp"></p>
<input id="who" type="email" placeholder="your@email" autocomplete="email">
<div class="row"><button class="g" id="cx">Cancel</button><button class="p" id="ok">Book it</button></div>
<div class="msg" id="dm"></div>
</div></dialog>
<script>
var LAB='__LABID__', TZ='__TZ__'||'UTC';
var MACH=__MACHINES__, HOURS=[], day=0, DAYS=[], pick=null;

// Everything is stored and served in UTC, but a fablab thinks in its own wall
// clock: a 09:00-18:00 Paris day was being shown to guests as 07:00-16:00, and
// the slot the workshop calls 14:00 read as 12:00. Availability already honours
// the lab's zone server-side; this makes what people SEE agree with it.
function tzParts(d){
 try{
  var f=new Intl.DateTimeFormat('en-CA',{timeZone:TZ,year:'numeric',month:'2-digit',
   day:'2-digit',hour:'2-digit',minute:'2-digit',hour12:false}).formatToParts(d);
  var g={};f.forEach(function(p){g[p.type]=p.value});
  var hh=g.hour==='24'?'00':g.hour;
  return{ymd:g.year+'-'+g.month+'-'+g.day,hm:hh+':'+g.minute}
 }catch(e){ // unknown zone: fall back to UTC rather than render nothing
  var i=d.toISOString();return{ymd:i.slice(0,10),hm:i.slice(11,16)}}}
function hmOf(iso){return tzParts(new Date(iso)).hm}
function ymdOf(iso){return tzParts(new Date(iso)).ymd}

// The seven day buttons are the lab's days, not the viewer's — otherwise a
// guest abroad sees a window shifted off the workshop's calendar.
for(var i=0;i<7;i++){
 var d=new Date(Date.now()+i*86400000);
 var ymd=tzParts(d).ymd;
 DAYS.push({date:ymd,
  label:new Date(ymd+'T12:00:00Z').toLocaleDateString(undefined,
   {weekday:'short',day:'numeric',month:'short',timeZone:'UTC'})})}
var slots={};
function renderDays(){var e=document.getElementById('days');e.innerHTML='';
DAYS.forEach(function(d,i){var b=document.createElement('button');b.className='day';b.textContent=d.label;
b.setAttribute('aria-pressed',i===day?'true':'false');b.onclick=function(){day=i;renderDays();load()};e.appendChild(b)})}
function load(){document.getElementById('stat').textContent='loading…';
var date=DAYS[day].date, done=0; slots={};
// A brand-new lab has no machines, so this forEach body never runs and render()
// was never reached — the board sat on "loading…" forever with no console error.
if(!MACH.length){document.getElementById('stat').textContent='';
 document.getElementById('tb').innerHTML='';
 document.getElementById('th').innerHTML='';
 var f=document.querySelector('.frame');
 f.innerHTML='<div class="empty"><b>No machines yet.</b><br>'+
  'Add your first machine on the <a href="/'+LAB+'/admin">admin page</a>, '+
  'then members can book it here.</div>';
 return}
MACH.forEach(function(m){
 fetch('/'+LAB+'/v1/slots?machine='+encodeURIComponent(m.id)+'&from='+date+'&to='+date)
 .then(function(r){return r.json()}).then(function(j){slots[m.id]=(j.slots||[]).map(function(s){return s.start})})
 .catch(function(){slots[m.id]=[]})
 .finally(function(){if(++done===MACH.length){buildHours();render()}})})}
var gran={};
function buildHours(){var set={};gran={};
Object.keys(slots).forEach(function(k){gran[k]={};
 slots[k].forEach(function(s){var hm=hmOf(s);set[hm]=1;gran[k][hm.slice(3,5)]=1})});
HOURS=Object.keys(set).sort()}
function render(){
 var th=document.getElementById('th');th.innerHTML='';
 var tr=document.createElement('tr');tr.appendChild(document.createElement('th'));
 HOURS.forEach(function(h){var e=document.createElement('th');e.className='hh';e.textContent=h;tr.appendChild(e)});
 th.appendChild(tr);
 var tb=document.getElementById('tb');tb.innerHTML='';
 MACH.forEach(function(m){var r=document.createElement('tr');
  var td=document.createElement('td');td.className='mc';td.textContent=m.name;r.appendChild(td);
  HOURS.forEach(function(h){var c=document.createElement('td');c.className='cw';
   var full=(slots[m.id]||[]).filter(function(s){return hmOf(s)===h})[0];
   // A machine's grid is its own: a 60-min laser has no :30 slot, so that cell is
   // NOT "taken", it is not a slot. Showing "taken" there would be a false claim —
   // /v1/slots returns only free slots, so we can only honestly distinguish
   // "bookable" from "not bookable".
   var onGrid=(gran[m.id]||{})[h.slice(3)];
   if(!full&&!onGrid){c.appendChild(document.createTextNode(''));r.appendChild(c);return}
   var b=document.createElement('button');b.className='c';
   if(!full){b.disabled=true;b.textContent='taken';b.title='already booked'}
   else{b.setAttribute('aria-label','Book '+m.name+' at '+h);
    b.onclick=function(){pick={m:m,at:full,h:h};open_()}}
   c.appendChild(b);r.appendChild(c)});
  tb.appendChild(r)});
 document.getElementById('stat').textContent=HOURS.length?'':'nothing bookable on this day';
 document.getElementById('tzline').textContent='Times shown in '+TZ+' · '+
  MACH.length+(MACH.length===1?' machine':' machines');
}
var LS='creneau.bookings.'+LAB;
function mine(){try{return JSON.parse(localStorage.getItem(LS)||'[]')}catch(e){return[]}}
function remember(b,m,label){try{var a=mine();
 a.push({id:b.id,token:b.manage_token,machine:m,when:label,start:b.start});
 localStorage.setItem(LS,JSON.stringify(a))}catch(e){}}
function forget(id){try{localStorage.setItem(LS,JSON.stringify(mine().filter(function(x){return x.id!==id})))}catch(e){}}
// "Your bookings" lives in localStorage, so it used to keep showing a booking
// the organizer had already cancelled — this browser was never told. Re-check
// each remembered booking against the server (the manage token we hold is
// exactly the proof needed to read it) and drop the ones that are gone.
function syncMine(){
 var a=mine();if(!a.length)return;
 var left=a.length;
 a.forEach(function(x){
  fetch('/'+LAB+'/v1/booking/'+encodeURIComponent(x.id)+'?t='+encodeURIComponent(x.token||''))
  .then(function(r){return r.json().then(function(j){return{s:r.status,j:j}})})
  .then(function(r){
   var b=r.j&&r.j.booking;
   if(r.s===404||(b&&b.status&&b.status!=='confirmed'))forget(x.id)})
  .catch(function(){})   // offline: keep what we have rather than wrongly forget
  .finally(function(){if(--left===0)renderMine()})})}

function renderMine(){
 var host=document.getElementById('mine'),a=mine();
 if(!a.length){host.innerHTML='';return}
 var h='<div class="mine"><h3>Your bookings</h3>';
 a.forEach(function(x){
  h+='<div class="mb" data-id="'+x.id+'"><span><b>'+x.machine+'</b> <span class="w">'+x.when+'</span></span>'+
     '<button data-cancel="'+x.id+'">Cancel</button></div>'});
 host.innerHTML=h+'</div>';
 host.querySelectorAll('[data-cancel]').forEach(function(btn){
  btn.onclick=function(){
   var id=btn.getAttribute('data-cancel'),rec=mine().filter(function(x){return x.id===id})[0];
   if(!rec){forget(id);renderMine();return}
   if(!confirm('Cancel your booking?\n\n'+rec.machine+' — '+rec.when+'\n\nThe slot frees up for someone else straight away.'))return;
   btn.disabled=true;btn.textContent='cancelling…';
   fetch('/'+LAB+'/v1/cancel',{method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({id:rec.id,token:rec.token})})
   .then(function(r){return r.json().then(function(j){return{s:r.status,j:j}})})
   .then(function(x){
     if(x.s===200||x.s===409){forget(id);renderMine();load()}
     else{btn.disabled=false;btn.textContent='Cancel';
      alert((x.j.error&&x.j.error.message)||'could not cancel')}})
   .catch(function(){btn.disabled=false;btn.textContent='Cancel'})}})}

var dlg=document.getElementById('dlg');
function open_(){document.getElementById('dt').textContent='Book '+pick.m.name;
document.getElementById('dp').textContent=DAYS[day].label+' at '+pick.h;
document.getElementById('dm').textContent='';document.getElementById('dm').className='msg';dlg.showModal()}
document.getElementById('cx').onclick=function(){dlg.close()};
document.getElementById('ok').onclick=function(){
 var who=document.getElementById('who').value.trim(),m=document.getElementById('dm');
 if(!who){m.className='msg no';m.textContent='an email is required';return}
 m.className='msg';m.textContent='booking…';
 fetch('/'+LAB+'/v1/book',{method:'POST',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({machine:pick.m.id,at:pick.at,who:who})})
 .then(function(r){return r.json().then(function(j){return{s:r.status,j:j}})})
 .then(function(x){if(x.s===200){m.className='msg ok';m.textContent='Booked. See you then.';
   remember(x.j.booking||{},pick.m.name,DAYS[day].label+' at '+pick.h);renderMine();
   setTimeout(function(){dlg.close();load()},900)}
  else{m.className='msg no';m.textContent=(x.j.error&&x.j.error.message)||'could not book';if(x.s===409){load()}}})
 .catch(function(){m.className='msg no';m.textContent='network trouble'})};
document.getElementById('apih').innerHTML=
 '<a href="/'+LAB+'/v1/slots?machine='+(MACH[0]||{}).id+'">GET /'+LAB+'/v1/slots</a> · POST /'+LAB+'/v1/book';
renderDays();renderMine();syncMine();load();
</script></body></html>`

// machinesJSON is the board's machine list. Kept as config rather than derived,
// because the display name and the order are a lab's choice, not the engine's.
func boardPage(l lab) string {
	ms := make([]map[string]any, 0, len(l.Machines))
	for _, m := range l.Machines {
		ms = append(ms, map[string]any{"id": m.ID, "name": m.Name})
	}
	j, _ := json.Marshal(ms)
	// __LAB__ is the display NAME and __LABID__ is the url slug. They are one
	// character apart and a name works fine in a heading, so putting __LAB__ in
	// an href fails only for labs whose name differs from their id — which is
	// every real lab. Use __LABID__ in every URL.
	s := strings.ReplaceAll(boardHTML, "__LAB__", html.EscapeString(l.Name))
	s = strings.ReplaceAll(s, "__LABID__", l.ID)
	tz := l.TZ
	if tz == "" {
		tz = "UTC"
	}
	s = strings.ReplaceAll(s, "__TZ__", html.EscapeString(tz))
	return strings.ReplaceAll(s, "__MACHINES__", string(j))
}

// labBoardHandler serves one lab's board. The machine list comes from the lab
// record, not from env, which is what makes a second lab a data change rather
// than another deployment.
func labBoardHandler(w http.ResponseWriter, r *http.Request) {
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=30")
	_, _ = w.Write([]byte(boardPage(l)))
}

// labIndexHandler answers the bare host. It lists nothing by default: a lab's
// board is not a directory entry, and creneau is not a marketplace.
func labIndexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "creneau",
		"message": "each lab has its own board at /<lab>",
		"routes": []string{
			"GET /<lab>", "GET /<lab>/v1/slots?machine=<id>&from=<date>&to=<date>",
			"POST /<lab>/v1/book", "POST /<lab>/v1/cancel", "GET /guide",
		},
	})
}
