package main

// Losing the admin token used to orphan a lab: nothing could mint another, and a
// fablab that cannot administer its own board has no product. Recovery mirrors the
// pattern already used elsewhere in the estate (vigie): ask with the lab id or the
// signup email, receive a short code at the recorded address, exchange the code for
// a fresh token — which invalidates the old one.

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const recoveryTTL = 30 * time.Minute

var recoverLimiter = newRateLimiter(5, time.Hour)

// recoveryCode is short enough to retype from an email and still 40 bits of
// entropy, which is ample against a 30-minute, single-use, rate-limited window.
func recoveryCode() (string, error) {
	const alpha = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no I/O/0/1
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, 8)
	for i, v := range b {
		out[i] = alpha[int(v)%len(alpha)]
	}
	return string(out[:4]) + "-" + string(out[4:]), nil
}

// findLabForRecovery accepts either the lab id or the signup email.
func findLabForRecovery(c *bkn, labID, email string) (doc, bool) {
	if labID != "" {
		if d, err := c.get(ns, "labs", labID); err == nil {
			return d, true
		}
		return nil, false
	}
	if email == "" {
		return nil, false
	}
	recs, err := c.list(ns, "labs", nil)
	if err != nil {
		return nil, false
	}
	for _, d := range recs {
		if strings.EqualFold(asStr(d["email"]), email) {
			return d, true
		}
	}
	return nil, false
}

// recoverStartHandler always answers 200. Telling a caller whether a lab or an
// email exists would turn this into an enumeration oracle, and the honest reply
// ("if it exists, a code is on its way") costs nothing.
func recoverStartHandler(w http.ResponseWriter, r *http.Request) {
	if !recoverLimiter.allow(clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "rate_limited", "too many attempts; try again later")
		return
	}
	var in struct{ Lab, Email string }
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body", "body must be a JSON object")
		return
	}
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	reply := map[string]any{
		"ok": true,
		"message": "if that lab exists and has an email on file, a recovery code is on its way; " +
			"it is valid for 30 minutes",
		"next": "POST /v1/labs/recover/confirm {\"lab\":\"<id>\",\"code\":\"XXXX-XXXX\"}",
	}
	if in.Lab == "" && in.Email == "" {
		writeErr(w, http.StatusBadRequest, "missing_argument", "lab or email is required")
		return
	}

	c := newBkn()
	d, found := findLabForRecovery(c, in.Lab, in.Email)
	if !found {
		writeJSON(w, http.StatusOK, reply)
		return
	}
	to := asStr(d["email"])
	if to == "" {
		writeJSON(w, http.StatusOK, reply)
		return
	}
	code, err := recoveryCode()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "could not mint a code")
		return
	}
	labID := asStr(d["id"])
	if _, err := c.patchIf(ns, "labs", labID, doc{
		"recovery_code_hash": hashToken(code),
		"recovery_expires":   time.Now().UTC().Add(recoveryTTL).Format(stamp),
	}, nil); err != nil {
		writeBknErr(w, err)
		return
	}

	// Deliver it, or say plainly that it could not be delivered. A recovery flow
	// that silently drops the code is worse than one that admits it has no mailer.
	if err := sendRecoveryMail(to, labID, code); err != nil {
		// The code is stored HASHED, so it cannot be read back from the record —
		// an earlier version of this message claimed otherwise and was simply
		// wrong. Without a mailer the only way it survives is the journal, so put
		// it there deliberately and say exactly that. It is single-use and expires
		// in 30 minutes, which is the trade that makes this acceptable.
		fmt.Fprintf(os.Stderr, "[recover] lab=%s to=%s code=%s (no mailer: deliver by hand)\n",
			labID, to, code)
		reply["delivery"] = "unconfigured"
		reply["message"] = "a recovery code was generated but this instance has no mailer " +
			"configured, so it could not be emailed — ask the operator, who can read it from " +
			"the service journal (journalctl -u creneau)"
	}
	writeJSON(w, http.StatusOK, reply)
}

// recoverConfirmHandler exchanges a valid code for a fresh admin token. The old
// token stops working, because the hash it was checked against is overwritten.
func recoverConfirmHandler(w http.ResponseWriter, r *http.Request) {
	if !recoverLimiter.allow(clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "rate_limited", "too many attempts; try again later")
		return
	}
	var in struct{ Lab, Code string }
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body", "body must be a JSON object")
		return
	}
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	if in.Lab == "" || in.Code == "" {
		writeErr(w, http.StatusBadRequest, "missing_argument", "lab and code are required")
		return
	}
	c := newBkn()
	d, err := c.get(ns, "labs", in.Lab)
	if err != nil {
		writeErr(w, http.StatusForbidden, "forbidden", "that code is not valid")
		return
	}
	want, exp := asStr(d["recovery_code_hash"]), asStr(d["recovery_expires"])
	if want == "" || hashToken(in.Code) != want {
		writeErr(w, http.StatusForbidden, "forbidden", "that code is not valid")
		return
	}
	t, perr := time.Parse(stamp, exp)
	if perr != nil || time.Now().UTC().After(t) {
		writeErr(w, http.StatusForbidden, "forbidden", "that code has expired; request another")
		return
	}
	tok, terr := manageToken()
	if terr != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "could not mint a token")
		return
	}
	// Single use: the code is cleared in the same write that rotates the token.
	if _, err := c.patchIf(ns, "labs", in.Lab, doc{
		"admin_token_hash":   hashToken(tok),
		"recovery_code_hash": "",
		"recovery_expires":   "",
	}, nil); err != nil {
		writeBknErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "lab": in.Lab, "admin_token": tok,
		"note": "the previous admin token no longer works; save this one",
	})
}

// sendRecoveryMail uses Resend when a key is present. creneau deliberately does
// not hold a mail credential of its own: point the unit at one with
// EnvironmentFile and this starts working, with no code change.
func sendRecoveryMail(to, labID, code string) error {
	key := os.Getenv("RESEND_API_KEY")
	if key == "" {
		return fmt.Errorf("RESEND_API_KEY not set")
	}
	from := os.Getenv("CRENEAU_MAIL_FROM")
	if from == "" {
		from = "creneau <javi@intrane.fr>"
	}
	body, _ := json.Marshal(map[string]any{
		"from": from, "to": []string{to},
		"subject": "Your creneau recovery code: " + code,
		// A person gets a link, an agent gets the curl. The link only opens a page;
		// it does not spend the code, so a mail scanner prefetching it is harmless.
		"text": "Someone asked to recover the admin token for the creneau board \"" + labID + "\".\n\n" +
			"Open this to get back in:\n\n  " + selfURL() + "/" + labID + "/recover?code=" + code + "\n\n" +
			"Recovery code: " + code + "\nIt is valid for 30 minutes and can be used once.\n\n" +
			"Prefer the API? Same thing:\n\n" +
			"  curl -X POST " + selfURL() + "/v1/labs/recover/confirm \\\n" +
			"    -H 'content-type: application/json' \\\n" +
			"    -d '{\"lab\":\"" + labID + "\",\"code\":\"" + code + "\"}'\n\n" +
			"If this was not you, ignore it — nothing has changed yet.\n",
	})
	req, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewReader(body))
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
		return fmt.Errorf("resend %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}
