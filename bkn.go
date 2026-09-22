package main

// A thin client for the one interface an out-of-process application has to
// bkn: its HTTP API. bkn's Go packages are all `internal/`, so importing them
// is not an option, and the primitives this domain needs most — `lock`,
// `putIfAbsent`, a two-bound range — have no route. That is why the atomic
// steps run as scripts and this file mostly moves plain documents.
//
// See docs/ledger.md rows 1-4.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type bkn struct {
	base  string
	token string
	hc    *http.Client
}

func newBkn() *bkn {
	base := os.Getenv("BKN_URL")
	if base == "" {
		base = "http://127.0.0.1:7799"
	}
	return &bkn{
		base:  strings.TrimRight(base, "/"),
		token: os.Getenv("BKN_ADMIN_TOKEN"),
		hc:    &http.Client{Timeout: 20 * time.Second},
	}
}

// bknError carries bkn's own status through so a caller can tell "slot taken"
// (409) from "bkn is down" (connection refused) without string matching.
type bknError struct {
	Status int
	Type   string
	Msg    string
}

func (e *bknError) Error() string {
	if e.Type != "" {
		return fmt.Sprintf("bkn %d %s: %s", e.Status, e.Type, e.Msg)
	}
	return fmt.Sprintf("bkn %d: %s", e.Status, e.Msg)
}

func (c *bkn) do(method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return &bknError{Status: 0, Type: "unreachable", Msg: err.Error()}
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 400 {
		var e struct {
			Error struct{ Type, Message string } `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		return &bknError{Status: resp.StatusCode, Type: e.Error.Type, Msg: e.Error.Message}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

type doc = map[string]any

// --- documents ------------------------------------------------------------

func (c *bkn) declare(ns, coll string, normalize map[string]string) error {
	body := doc{}
	if len(normalize) > 0 {
		body["normalize"] = normalize
	}
	return c.do(http.MethodPut, "/v1/store/"+ns+"/"+coll, body, nil)
}

func (c *bkn) put(ns, coll, id string, d doc) (doc, error) {
	var out struct {
		Record doc `json:"record"`
	}
	p := "/v1/store/" + ns + "/" + coll
	if id != "" {
		p += "?id=" + url.QueryEscape(id)
	}
	if err := c.do(http.MethodPost, p, d, &out); err != nil {
		return nil, err
	}
	return out.Record, nil
}

func (c *bkn) get(ns, coll, id string) (doc, error) {
	var out struct {
		Record doc `json:"record"`
	}
	if err := c.do(http.MethodGet, "/v1/store/"+ns+"/"+coll+"/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return out.Record, nil
}

func (c *bkn) list(ns, coll string, q url.Values) ([]doc, error) {
	var out struct {
		Records []doc `json:"records"`
	}
	p := "/v1/store/" + ns + "/" + coll
	if len(q) > 0 {
		p += "?" + q.Encode()
	}
	if err := c.do(http.MethodGet, p, nil, &out); err != nil {
		return nil, err
	}
	return out.Records, nil
}

// patchIf is a compare-and-set: the write lands only if every condition still
// holds on the document being written. This one IS reachable over HTTP.
func (c *bkn) patchIf(ns, coll, id string, fields doc, conds map[string]string) (doc, error) {
	q := url.Values{}
	for f, v := range conds {
		q.Add("if", f+"="+v)
	}
	p := "/v1/store/" + ns + "/" + coll + "/" + url.PathEscape(id)
	if len(q) > 0 {
		p += "?" + q.Encode()
	}
	var out struct {
		Record doc `json:"record"`
	}
	if err := c.do(http.MethodPatch, p, fields, &out); err != nil {
		return nil, err
	}
	return out.Record, nil
}

// --- scripts --------------------------------------------------------------

// run invokes a named bkn script. Everything this domain needs that the HTTP
// API does not expose goes through here.
func (c *bkn) run(name string, input any) (doc, error) {
	var out struct {
		Value doc `json:"value"`
		Run   doc `json:"run"`
	}
	// The posted body IS the script's input — bkn decodes it straight into
	// main(input) rather than unwrapping an envelope.
	if err := c.do(http.MethodPost, "/v1/script/"+url.PathEscape(name)+"/run",
		input, &out); err != nil {
		return nil, err
	}
	if out.Value == nil {
		return doc{}, nil
	}
	return out.Value, nil
}

// scriptResult reads the {ok,status,error} envelope creneau's scripts return,
// so a script-level refusal (409 slot taken) surfaces as an error rather than
// as a success with a confusing body.
func scriptResult(v doc, err error) (doc, error) {
	if err != nil {
		return nil, err
	}
	if ok, _ := v["ok"].(bool); !ok {
		status := 0
		if f, isNum := v["status"].(float64); isNum {
			status = int(f)
		}
		msg, _ := v["error"].(string)
		return v, &bknError{Status: status, Type: "script_refused", Msg: msg}
	}
	return v, nil
}

// del removes a document. Used for session revocation, where deleting the row
// IS the revocation — no denylist, no waiting out an expiry.
func (c *bkn) del(ns, coll, id string) error {
	return c.do(http.MethodDelete, "/v1/store/"+ns+"/"+coll+"/"+url.PathEscape(id), nil, nil)
}
