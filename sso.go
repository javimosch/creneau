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
		if _, ok := labSession(&http.Request{
			Header: http.Header{"Authorization": []string{"Bearer " + link}},
			URL:    r.URL,
		}, c); !ok {
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
