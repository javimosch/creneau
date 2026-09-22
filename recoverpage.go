package main

// The human half of account recovery.
//
// The recovery mail used to carry only a curl command. That is right for an
// agent and a dead end for a person, so this renders a page the link in that
// mail can point at.
//
// It deliberately does NOT confirm on GET. The code is single-use, and mail
// clients, link scanners and corporate security gateways all prefetch links —
// any of them would silently burn the code before the organizer clicked it.
// The page therefore only *shows* the code; the POST happens on click.

import (
	"html"
	"net/http"
	"strings"
)

func recoverPageHandler(w http.ResponseWriter, r *http.Request) {
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
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	page := strings.NewReplacer(
		"__LABID__", labID, // validLabID() already restricts this to a slug
		"__LABNAME__", html.EscapeString(l.Name),
		"__CODE__", html.EscapeString(code),
	).Replace(recoverHTML)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(page))
}

const recoverHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex">
<title>__LABNAME__ — recover access</title>
<style>
:root{--bg:#fbfbfc;--ink:#0b0e13;--mut:#616b78;--bd:#e3e6ea;--card:#fff;--accent:#f26b1d}
@media(prefers-color-scheme:dark){:root{--bg:#0a0c10;--ink:#eceef1;--mut:#8892a0;--bd:#222833;--card:#11151b}}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--ink);font:15px/1.55 system-ui,-apple-system,Segoe UI,Roboto,sans-serif}
.wrap{max-width:560px;margin:8vh auto;padding:0 16px}
h1{font-size:20px;margin:0 0 4px}.sub{color:var(--mut);font-size:13.5px;margin:0 0 18px}
.card{background:var(--card);border:1px solid var(--bd);border-radius:12px;padding:20px}
label{display:block;font-size:13px;color:var(--mut);margin-bottom:6px}
input{width:100%;padding:10px 12px;border:1px solid var(--bd);border-radius:8px;
 background:var(--bg);color:var(--ink);font-family:ui-monospace,monospace;font-size:15px}
button{margin-top:14px;padding:10px 18px;border:0;border-radius:8px;background:var(--accent);
 color:#fff;font-weight:600;font-size:14.5px;cursor:pointer}
button[disabled]{opacity:.55;cursor:default}
.msg{margin-top:12px;font-size:14px}.no{color:#c0392b}.ok{color:#1e8449}
code{display:block;font-family:ui-monospace,monospace;font-size:13.5px;word-break:break-all;
 background:var(--bg);border:1px solid var(--bd);border-radius:8px;padding:10px;margin-top:8px}
.fine{color:var(--mut);font-size:12.5px;margin-top:14px}
</style></head><body><div class="wrap">
<h1>__LABNAME__</h1>
<p class="sub">Recover organizer access</p>
<div class="card" id="card">
  <label for="code">Recovery code from your email</label>
  <input id="code" value="__CODE__" placeholder="XXXX-XXXX" autocomplete="one-time-code">
  <button id="go">Recover access</button>
  <div class="msg" id="msg"></div>
  <p class="fine">This rotates your admin token: the previous one stops working.
    Nothing changes until you press the button.</p>
</div>
</div>
<script>
var LAB='__LABID__', SK='creneau.session.'+LAB;
var go=document.getElementById('go'), msg=document.getElementById('msg');
function say(t,c){msg.className='msg '+(c||'');msg.textContent=t}
go.onclick=function(){
 var code=(document.getElementById('code').value||'').trim().toUpperCase();
 if(!code){say('Paste the code from your email.','no');return}
 go.disabled=true;say('Checking…');
 fetch('/v1/labs/recover/confirm',{method:'POST',
  headers:{'Content-Type':'application/json'},
  body:JSON.stringify({lab:LAB,code:code})})
 .then(function(r){return r.json().then(function(j){return{s:r.status,j:j}})})
 .then(function(x){
  if(x.s!==200){go.disabled=false;
   say((x.j.error&&x.j.error.message)||'That code is not valid.','no');return}
  // Trade the fresh admin token straight for a session, so the organizer lands
  // signed in rather than holding a secret they have to paste somewhere.
  var tok=x.j.admin_token;
  fetch('/'+LAB+'/v1/session',{method:'POST',
   headers:{'Content-Type':'application/json'},
   body:JSON.stringify({admin_token:tok})})
  .then(function(r){return r.json()})
  .then(function(s){
   if(s&&s.session){
    try{localStorage.setItem(SK,JSON.stringify({t:s.session,exp:s.expires}))}catch(e){}
    location.replace('/'+LAB+'/admin');return}
   throw new Error('no session')})
  .catch(function(){
   // Session failed but the token is real — show it rather than lose it.
   document.getElementById('card').innerHTML=
    '<b>Access recovered.</b><p class="fine">Save this admin token — it is not shown again.</p>'+
    '<code>'+tok.replace(/[<&]/g,'')+'</code>'+
    '<p class="fine"><a href="/'+LAB+'/admin">Open the admin page</a></p>'})})
 .catch(function(){go.disabled=false;say('Network trouble — try again.','no')})}
</script></body></html>`
