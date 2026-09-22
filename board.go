package main

// The board a human uses. It is a static client: the browser calls the same two
// public routes an agent calls — GET /v1/slots and POST /v1/book — and renders
// them. Nothing is server-rendered; this handler only hands over the client.
// Same origin as the API, so it needs no CORS.

import (
	"net/http"
	"strings"
)

const boardHTML = `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
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
.api{margin-top:16px;font:12px var(--mono);color:var(--mut)}
.api a{color:var(--ac)}
</style></head><body><div class="wrap">
<header><h1>__LAB__ — machine board</h1></header>
<p class="sub" id="tzline"></p>
<div class="days" id="days"></div>
<div class="frame"><div class="scr"><table><thead id="th"></thead><tbody id="tb"></tbody></table></div>
<div class="legend"><span><i></i>free — click to book</span><span><i class="t"></i>taken</span>
<span id="stat"></span></div></div>
<p class="api">Same two calls an agent makes:
<a href="/v1/slots?event=laser&amp;from=2026-10-06&amp;to=2026-10-06">GET /v1/slots</a> ·
POST /v1/book · <a href="/guide">GET /guide</a></p>
</div>
<dialog id="dlg"><div class="dlg">
<h2 id="dt">Book</h2><p id="dp"></p>
<input id="who" type="email" placeholder="your@email" autocomplete="email">
<div class="row"><button class="g" id="cx">Cancel</button><button class="p" id="ok">Book it</button></div>
<div class="msg" id="dm"></div>
</div></dialog>
<script>
var MACH=__MACHINES__, HOURS=[], day=0, DAYS=[], pick=null;
function iso(d){return d.toISOString().slice(0,10)}
for(var i=0;i<7;i++){var d=new Date();d.setDate(d.getDate()+i);DAYS.push({date:iso(d),
label:d.toLocaleDateString(undefined,{weekday:'short',day:'numeric',month:'short'})})}
var slots={};
function renderDays(){var e=document.getElementById('days');e.innerHTML='';
DAYS.forEach(function(d,i){var b=document.createElement('button');b.className='day';b.textContent=d.label;
b.setAttribute('aria-pressed',i===day?'true':'false');b.onclick=function(){day=i;renderDays();load()};e.appendChild(b)})}
function load(){document.getElementById('stat').textContent='loading…';
var date=DAYS[day].date, done=0; slots={};
MACH.forEach(function(m){
 fetch('/v1/slots?event='+encodeURIComponent(m.id)+'&from='+date+'&to='+date)
 .then(function(r){return r.json()}).then(function(j){slots[m.id]=(j.slots||[]).map(function(s){return s.start})})
 .catch(function(){slots[m.id]=[]})
 .finally(function(){if(++done===MACH.length){buildHours();render()}})})}
var gran={};
function buildHours(){var set={};gran={};
Object.keys(slots).forEach(function(k){gran[k]={};
 slots[k].forEach(function(s){set[s.slice(11,16)]=1;gran[k][s.slice(14,16)]=1})});
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
   var full=(slots[m.id]||[]).filter(function(s){return s.slice(11,16)===h})[0];
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
 document.getElementById('tzline').textContent='Times shown in UTC as returned by the API · '+MACH.length+' machines';
}
var dlg=document.getElementById('dlg');
function open_(){document.getElementById('dt').textContent='Book '+pick.m.name;
document.getElementById('dp').textContent=DAYS[day].label+' at '+pick.h;
document.getElementById('dm').textContent='';document.getElementById('dm').className='msg';dlg.showModal()}
document.getElementById('cx').onclick=function(){dlg.close()};
document.getElementById('ok').onclick=function(){
 var who=document.getElementById('who').value.trim(),m=document.getElementById('dm');
 if(!who){m.className='msg no';m.textContent='an email is required';return}
 m.className='msg';m.textContent='booking…';
 fetch('/v1/book',{method:'POST',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({event:pick.m.id,at:pick.at,who:who})})
 .then(function(r){return r.json().then(function(j){return{s:r.status,j:j}})})
 .then(function(x){if(x.s===200){m.className='msg ok';m.textContent='Booked. See you then.';
   setTimeout(function(){dlg.close();load()},900)}
  else{m.className='msg no';m.textContent=(x.j.error&&x.j.error.message)||'could not book';if(x.s===409){load()}}})
 .catch(function(){m.className='msg no';m.textContent='network trouble'})};
renderDays();load();
</script></body></html>`

// machinesJSON is the board's machine list. Kept as config rather than derived,
// because the display name and the order are a lab's choice, not the engine's.
func boardPage(lab, machinesJSON string) string {
	s := strings.ReplaceAll(boardHTML, "__LAB__", lab)
	return strings.ReplaceAll(s, "__MACHINES__", machinesJSON)
}

func boardHandler(lab, machinesJSON string) http.HandlerFunc {
	page := boardPage(lab, machinesJSON)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			writeErr(w, http.StatusNotFound, "no_such_route", "not found")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=60")
		_, _ = w.Write([]byte(page))
	}
}
