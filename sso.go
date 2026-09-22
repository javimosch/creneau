package main

// SSO for organizers, brokered by portier.
//
// This is deliberately ADDITIVE. The session layer already exists, so SSO is just
// one more way to mint a session: verified identity -> owner check -> session.
// The admin token keeps working for agents, curl and recovery — and it must, because
// portier is metered (free auths then a peage wallet), and an empty wallet must not
// lock a lab out of its own board.
//
// SSO authenticates ORGANIZERS ONLY. Members book from a link with no account, and
// putting a login in front of that would break the product's central promise.

import (
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const authStateTTL = 10 * time.Minute

func ssoEnabled() bool {
	return os.Getenv("PORTIER_APP_ID") != "" && os.Getenv("PORTIER_APP_SECRET") != ""
}

func portierURL() string {
	if u := os.Getenv("PORTIER_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "https://portier.intrane.fr"
}

// ssoProviders is the list offered on the admin page, in order.
func ssoProviders() []string {
	if p := os.Getenv("PORTIER_PROVIDERS"); p != "" {
		out := []string{}
		for _, s := range strings.Split(p, ",") {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	// No default. An unset list means no sign-in buttons — falling back to
	// "demo" would leave a FAKE identity provider reachable in production, where
	// anyone could authenticate as demo@portier.
	return nil
}

func selfURL() string {
	if u := os.Getenv("CRENEAU_PUBLIC_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "https://board.creneau.intrane.fr"
}

type owner struct {
	Provider string `json:"provider"`
	Sub      string `json:"sub"`
	Email    string `json:"email"`
}

func ownersOf(d doc) []owner {
	out := []owner{}
	raw, ok := d["owners"].([]any)
	if !ok {
		return out
	}
	for _, o := range raw {
		od, ok := o.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, owner{asStr(od["provider"]), asStr(od["sub"]), asStr(od["email"])})
	}
	return out
}

// authStartHandler sends the organizer to portier. The lab id travels in a
// server-side nonce rather than in the state blob, so a tampered state cannot
// point the callback at a different lab — the nonce IS the only thing that maps
// back, it is single use, and it expires.
func authStartHandler(w http.ResponseWriter, r *http.Request) {
	if !ssoEnabled() {
		writeErr(w, http.StatusNotImplemented, "unconfigured", "SSO is not configured on this instance")
		return
	}
	labID := r.PathValue("lab")
	c := newBkn()
	if _, err := loadLab(c, labID); err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "no such lab "+labID)
		return
	}
	provs := ssoProviders()
	if len(provs) == 0 {
		writeErr(w, http.StatusNotImplemented, "unconfigured",
			"no sign-in providers are configured on this instance")
		return
	}
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		provider = provs[0]
	}
	// Only offer what the operator listed, so a guessed provider name cannot
	// reach one that was registered but deliberately not exposed.
	allowed := false
	for _, p := range provs {
		if p == provider {
			allowed = true
			break
		}
	}
	if !allowed {
		writeErr(w, http.StatusBadRequest, "invalid_value", "unknown provider "+provider)
		return
	}
	nonce, err := manageToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "could not start sign-in")
		return
	}
	rec := doc{
		"lab": labID, "provider": provider,
		"expires": time.Now().UTC().Add(authStateTTL).Format(stamp),
	}
	// "link" mode attaches a new identity to a lab the caller already administers,
	// proven by an existing session rather than by pasting the key again.
	if link := strings.TrimSpace(r.URL.Query().Get("link")); link != "" {
		if !sessionValidFor(c, link, labID) {
			writeErr(w, http.StatusForbidden, "forbidden", "that session is not valid")
			return
		}
		rec["link_session"] = hashToken(link)
	}
	if _, err := c.put(ns, "authstate", hashToken(nonce), rec); err != nil {
		writeBknErr(w, err)
		return
	}
	dest := portierURL() + "/auth/" + os.Getenv("PORTIER_APP_ID") + "/" + url.PathEscape(provider) +
		"?redirect_uri=" + url.QueryEscape(selfURL()+"/auth/done") +
		"&state=" + url.QueryEscape(nonce)
	http.Redirect(w, r, dest, http.StatusFound)
}

// authDoneHandler is the single registered redirect_uri. It resolves the nonce to
// a lab, exchanges the code for a verified identity, and only then decides.
func authDoneHandler(w http.ResponseWriter, r *http.Request) {
	if !ssoEnabled() {
		writeErr(w, http.StatusNotImplemented, "unconfigured", "SSO is not configured on this instance")
		return
	}
	q := r.URL.Query()
	code, nonce := q.Get("code"), q.Get("state")
	if code == "" || nonce == "" {
		ssoPage(w, "", "Sign-in did not complete", "The provider did not return a code.")
		return
	}
	c := newBkn()
	key := hashToken(nonce)
	st, err := c.get(ns, "authstate", key)
	if err != nil {
		ssoPage(w, "", "That sign-in link has expired", "Start again from the admin page.")
		return
	}
	_ = c.del(ns, "authstate", key) // single use, whatever happens next
	exp, perr := time.Parse(stamp, asStr(st["expires"]))
	if perr != nil || time.Now().UTC().After(exp) {
		ssoPage(w, "", "That sign-in link has expired", "Start again from the admin page.")
		return
	}
	labID := asStr(st["lab"])

	id, ierr := portierExchange(code)
	if ierr != nil {
		ssoPage(w, labID, "Could not verify that account", ierr.Error())
		return
	}
	labDoc, lerr := c.get(ns, "labs", labID)
	if lerr != nil {
		ssoPage(w, "", "That lab no longer exists", "")
		return
	}

	linked := false
	for _, o := range ownersOf(labDoc) {
		if o.Provider == id.Provider && o.Sub == id.Sub {
			linked = true
			break
		}
	}
	// Two ways to become an owner: prove you already administer the lab (link
	// mode, via a live session), or be the address the lab signed up with.
	if !linked {
		if ls := asStr(st["link_session"]); ls != "" {
			if sd, serr := c.get(ns, "sessions", ls); serr == nil && asStr(sd["lab"]) == labID {
				linked = true
			}
		}
	}
	if !linked && id.Email != "" && strings.EqualFold(id.Email, asStr(labDoc["email"])) {
		linked = true
	}
	if !linked {
		ssoPage(w, labID, "That account is not linked to this lab",
			html.EscapeString(id.Email)+" is not an owner. Sign in with the admin token, then use “Link this account”.")
		return
	}
	if err := addOwner(c, labID, labDoc, id); err != nil {
		ssoPage(w, labID, "Could not record that account", err.Error())
		return
	}
	tok, terr := manageToken()
	if terr != nil {
		ssoPage(w, labID, "Could not start a session", "")
		return
	}
	expAt := time.Now().UTC().Add(sessionTTL)
	if _, err := c.put(ns, "sessions", hashToken(tok), doc{
		"lab": labID, "expires": expAt.Format(stamp), "created_at": time.Now().UTC().Format(stamp),
		"via": id.Provider + ":" + id.Email,
	}); err != nil {
		ssoPage(w, labID, "Could not start a session", err.Error())
		return
	}
	// Hand the session to the admin page the same way it stores one itself.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>Signing you in…</title>
<body style="font:15px -apple-system,sans-serif;padding:40px">Signing you in…
<script>try{localStorage.setItem('creneau.session.' + ` + jsStr(labID) + `,
 JSON.stringify({t:` + jsStr(tok) + `,exp:` + jsStr(expAt.Format(stamp)) + `}))}catch(e){}
location.replace('/' + ` + jsStr(labID) + ` + '/admin');</script></body>`))
}

func addOwner(c *bkn, labID string, labDoc doc, id identity) error {
	ows := ownersOf(labDoc)
	for _, o := range ows {
		if o.Provider == id.Provider && o.Sub == id.Sub {
			return nil
		}
	}
	ows = append(ows, owner{id.Provider, id.Sub, id.Email})
	arr := make([]any, 0, len(ows))
	for _, o := range ows {
		arr = append(arr, doc{"provider": o.Provider, "sub": o.Sub, "email": o.Email})
	}
	_, err := c.patchIf(ns, "labs", labID, doc{"owners": arr}, nil)
	return err
}

type identity struct{ Sub, Email, Name, Provider string }

func portierExchange(code string) (identity, error) {
	body, _ := json.Marshal(map[string]string{"code": code})
	req, err := http.NewRequest("POST", portierURL()+"/v1/token", strings.NewReader(string(body)))
	if err != nil {
		return identity{}, err
	}
	req.Header.Set("Authorization", "Bearer "+os.Getenv("PORTIER_APP_SECRET"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return identity{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode >= 300 {
		return identity{}, errString("portier refused the code")
	}
	var out struct {
		Identity identity `json:"identity"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return identity{}, err
	}
	if out.Identity.Sub == "" {
		return identity{}, errString("portier returned no subject")
	}
	return out.Identity, nil
}

type errString string

func (e errString) Error() string { return string(e) }

func jsStr(s string) string { b, _ := json.Marshal(s); return string(b) }

func ssoPage(w http.ResponseWriter, labID, title, detail string) {
	back := "/"
	if labID != "" {
		back = "/" + labID + "/admin"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>` + html.EscapeString(title) + `</title>
<body style="font:15px/1.6 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;max-width:520px;margin:60px auto;padding:0 20px">
<h1 style="font-size:19px">` + html.EscapeString(title) + `</h1>
<p style="color:#5a6472">` + detail + `</p>
<p><a href="` + back + `">Back to the admin page</a></p></body>`))
}

// --- per-lab agent identity ------------------------------------------------

// mintAgentHandler gives a lab one agent identity at idp.intrane.fr, so the
// lab's own automation can administer it — and, because the identity is an IdP
// principal rather than a creneau token, sign in to any other app brokered
// through portier. That cross-app reach is the whole point.
//
// creneau holds a credential that can mint identities, so the blast radius is
// bounded deliberately:
//   - the handle is DERIVED from the lab id, never taken from the caller, so a
//     compromised session cannot choose `someone@intrane.fr`;
//   - one agent per lab, so a compromised session cannot mint without limit;
//   - the IdP itself refuses anything but kind=agent, and an agent cannot use
//     the browser sign-in form, so none of this can produce a human login.
func mintAgentHandler(w http.ResponseWriter, r *http.Request) {
	c := newBkn()
	l, ok := labSession(r, c)
	if !ok {
		writeErr(w, http.StatusForbidden, "forbidden", "a valid session or admin token is required")
		return
	}
	adm := os.Getenv("IDP_ADMIN_TOKEN")
	idp := os.Getenv("IDP_URL")
	if idp == "" {
		idp = "https://idp.intrane.fr"
	}
	if adm == "" {
		writeErr(w, http.StatusNotImplemented, "unconfigured",
			"agent identities are not enabled on this instance")
		return
	}
	labDoc, err := c.get(ns, "labs", l.ID)
	if err != nil {
		writeBknErr(w, err)
		return
	}
	if h := asStr(labDoc["agent_handle"]); h != "" {
		writeErr(w, http.StatusConflict, "conflict",
			"this lab already has the agent "+h+" — its password cannot be shown again")
		return
	}
	handle := "lab-" + l.ID + "@agents.intrane.fr"

	body, _ := json.Marshal(map[string]string{"handle": handle, "name": l.Name + " agent"})
	req, rerr := http.NewRequest("POST", strings.TrimRight(idp, "/")+"/v1/agents", strings.NewReader(string(body)))
	if rerr != nil {
		writeErr(w, http.StatusInternalServerError, "internal", rerr.Error())
		return
	}
	req.Header.Set("Authorization", "Bearer "+adm)
	req.Header.Set("Content-Type", "application/json")
	resp, herr := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if herr != nil {
		writeErr(w, http.StatusBadGateway, "upstream", "the identity provider is unreachable")
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode >= 300 {
		writeErr(w, http.StatusBadGateway, "upstream",
			"the identity provider refused: "+strings.TrimSpace(string(raw)))
		return
	}
	var out struct{ Sub, Handle, Password string }
	if err := json.Unmarshal(raw, &out); err != nil || out.Password == "" {
		writeErr(w, http.StatusBadGateway, "upstream", "unexpected reply from the identity provider")
		return
	}
	// Record the handle and make the agent an owner, so it can sign in through
	// portier immediately. The password is never stored — it is shown once here.
	if _, err := c.patchIf(ns, "labs", l.ID, doc{"agent_handle": out.Handle}, nil); err != nil {
		writeBknErr(w, err)
		return
	}
	labDoc["agent_handle"] = out.Handle
	if err := addOwner(c, l.ID, labDoc, identity{
		Sub: out.Sub, Email: out.Handle, Provider: "intrane",
	}); err != nil {
		writeBknErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "handle": out.Handle, "password": out.Password, "sub": out.Sub,
		"note": "shown once and not recoverable. This identity works at every app on " + idp +
			", not only creneau.",
		"headless_login": "curl -u '" + out.Handle + ":<password>' '" + idp +
			"/authorize?response_type=code&client_id=<cid>&redirect_uri=<uri>&scope=openid%20email&state=x'",
	})
}

// --- invite the organizer when a lab signs up ------------------------------

// inviteOrganizer asks machin-idp for an invitation for the address the lab
// signed up with, and mails it. Best effort by design: a lab must exist even if
// the IdP is unreachable, so this never fails lab creation — it reports what
// happened instead.
//
// The identity the invitation creates carries that same address, so the existing
// owner check (email match) grants it on first sign-in. No separate linking step.
func inviteOrganizer(labID, labName, email string) string {
	adm := os.Getenv("IDP_ADMIN_TOKEN")
	if adm == "" || email == "" {
		return "skipped"
	}
	// Don't mint an invitation nobody can ever receive: it would sit in the IdP
	// unusable, and the send would bounce against our sending reputation.
	if !deliverable(email) {
		return "skipped-undeliverable-address"
	}
	idp := os.Getenv("IDP_URL")
	if idp == "" {
		idp = "https://idp.intrane.fr"
	}
	body, _ := json.Marshal(map[string]any{
		"handle": email, "name": labName + " organizer", "days": 14,
		"invited_by": "creneau/" + labID,
	})
	req, err := http.NewRequest("POST", strings.TrimRight(idp, "/")+"/v1/invites", strings.NewReader(string(body)))
	if err != nil {
		return "error"
	}
	req.Header.Set("Authorization", "Bearer "+adm)
	req.Header.Set("Content-Type", "application/json")
	resp, herr := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if herr != nil {
		return "unreachable"
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	// Already registered is not a failure: they have an identity, so they can
	// simply sign in. Saying "exists" is more useful than saying "error".
	if resp.StatusCode == http.StatusConflict {
		return "already-registered"
	}
	if resp.StatusCode >= 300 {
		return "refused"
	}
	var out struct {
		InviteURL string `json:"invite_url"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.InviteURL == "" {
		return "error"
	}
	if err := sendInviteMail(email, labID, labName, out.InviteURL); err != nil {
		return "created-not-mailed"
	}
	return "sent"
}

func sendInviteMail(to, labID, labName, inviteURL string) error {
	key := os.Getenv("RESEND_API_KEY")
	if key == "" {
		return errString("RESEND_API_KEY not set")
	}
	if !deliverable(to) {
		return errString("address is not deliverable: " + to)
	}
	from := os.Getenv("CRENEAU_MAIL_FROM")
	if from == "" {
		from = "creneau <javi@intrane.fr>"
	}
	board := selfURL() + "/" + labID
	body, _ := json.Marshal(map[string]any{
		"from": from, "to": []string{to},
		"subject": labName + " — your board is ready",
		"text": "Your machine board is live:\n\n  " + board + "\n\n" +
			"Members book from that link. They need no account.\n\n" +
			"To administer it — add machines, block maintenance days, see who booked —\n" +
			"set a password for your intrane sign-in here:\n\n  " + inviteURL + "\n\n" +
			"That link works once and expires in 14 days. Nobody else sets your password.\n" +
			"Afterwards, sign in at " + selfURL() + "/" + labID + "/admin\n\n" +
			"You can also administer the board with the admin token shown when you\n" +
			"signed up, which keeps working whether or not you use the sign-in above.\n",
	})
	req, err := http.NewRequest("POST", "https://api.resend.com/emails", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
		return errString("resend " + strings.TrimSpace(string(raw)))
	}
	return nil
}
