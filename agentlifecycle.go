package main

// Rotate and revoke the lab's agent identity.
//
// Minting shows the password once and stores only the handle, which is right —
// but it left an organizer who closed that dialog with an identity they could
// never use and never remove. These two calls are the way out.

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// idpAdminCall performs an admin-token call against machin-idp and returns the
// decoded body. Shared by rotate and revoke so their gating cannot drift.
func idpAdminCall(method, path string, body any) (map[string]any, int, error) {
	adm := os.Getenv("IDP_ADMIN_TOKEN")
	idp := os.Getenv("IDP_URL")
	if idp == "" {
		idp = "https://idp.intrane.fr"
	}
	if adm == "" {
		return nil, http.StatusNotImplemented, errors.New("agent identities are not enabled on this instance")
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(method, strings.TrimRight(idp, "/")+path, strings.NewReader(string(b)))
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	req.Header.Set("Authorization", "Bearer "+adm)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, http.StatusBadGateway, errors.New("the identity provider is unreachable")
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if resp.StatusCode >= 300 {
		return out, http.StatusBadGateway,
			errors.New("the identity provider refused: " + strings.TrimSpace(string(raw)))
	}
	return out, http.StatusOK, nil
}

// POST /{lab}/v1/agent/rotate — issue a new password for the lab's agent.
func rotateAgentHandler(w http.ResponseWriter, r *http.Request) {
	c := newBkn()
	l, ok := labSession(r, c)
	if !ok {
		writeErr(w, http.StatusForbidden, "forbidden", "a valid session or admin token is required")
		return
	}
	labDoc, err := c.get(ns, "labs", l.ID)
	if err != nil {
		writeBknErr(w, err)
		return
	}
	handle := asStr(labDoc["agent_handle"])
	if handle == "" {
		writeErr(w, http.StatusNotFound, "not_found",
			"this lab has no agent identity yet; create one first")
		return
	}
	out, status, cerr := idpAdminCall(http.MethodPost, "/v1/agents/rotate",
		map[string]string{"handle": handle})
	if cerr != nil {
		writeErr(w, status, "upstream", cerr.Error())
		return
	}
	pw := asStr(out["password"])
	if pw == "" {
		writeErr(w, http.StatusBadGateway, "upstream", "unexpected reply from the identity provider")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "handle": handle, "password": pw,
		"note": "the previous password stopped working. This one is shown once.",
	})
}

// DELETE /{lab}/v1/agent — remove the lab's agent identity.
func revokeAgentHandler(w http.ResponseWriter, r *http.Request) {
	c := newBkn()
	l, ok := labSession(r, c)
	if !ok {
		writeErr(w, http.StatusForbidden, "forbidden", "a valid session or admin token is required")
		return
	}
	labDoc, err := c.get(ns, "labs", l.ID)
	if err != nil {
		writeBknErr(w, err)
		return
	}
	handle := asStr(labDoc["agent_handle"])
	if handle == "" {
		writeErr(w, http.StatusNotFound, "not_found", "this lab has no agent identity")
		return
	}
	// Delete upstream first: clearing our record while the identity still exists
	// would strand an agent that can log in but is no longer listed anywhere.
	if _, status, cerr := idpAdminCall(http.MethodDelete, "/v1/agents",
		map[string]string{"handle": handle}); cerr != nil {
		writeErr(w, status, "upstream", cerr.Error())
		return
	}
	kept := make([]any, 0)
	for _, o := range ownersOf(labDoc) {
		if strings.EqualFold(o.Email, handle) {
			continue // the agent stops being an owner along with its identity
		}
		kept = append(kept, doc{"provider": o.Provider, "sub": o.Sub, "email": o.Email})
	}
	if _, err := c.patchIf(ns, "labs", l.ID,
		doc{"agent_handle": "", "owners": kept}, nil); err != nil {
		writeBknErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "revoked": handle,
		"note": "the identity is gone and no longer an owner of this lab",
	})
}
