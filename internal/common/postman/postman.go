// Package postman is a COMMON-FILE tool: it reads a Postman collection export
// (a `*.postman_collection.json` file, Collection Format v2.1) at the granularity
// of its individual requests (endpoints). Like the markdown tool it lives
// alongside the programming-language layer but is deliberately NOT a
// core.Language:
//
//   - It is not routed by file extension through the language registry; the tool
//     is selected explicitly (a CLI subcommand / a named MCP tool).
//   - It parses the export with encoding/json into the Postman schema, NOT with
//     tree-sitter: the meaningful structure is the collection's request tree
//     (folders -> requests), not JSON syntax nodes, so tree-sitter would buy
//     nothing here.
//   - It is READ-ONLY. Editing a Postman export is out of scope, so this package
//     intentionally exposes no writer capability (the absence is the contract).
//
// The two capabilities mirror a pair of shell tools this replaces: a "list" that
// prints one endpoint per line for discovery, and a "doc" that prints
// METHOD / URL / BODY / DOC for the requests matching a query.
package postman

import (
	"encoding/json"
	"strings"
)

// Request is the light, body-free descriptor of one endpoint in the collection
// tree — the form returned by listing (the Postman analogue of core.Signature).
type Request struct {
	// Name is the request's display name in the collection.
	Name string
	// Method is the HTTP verb (GET, POST, PUT, ...).
	Method string
	// URL is the raw request URL, INCLUDING any unresolved Postman placeholders
	// (e.g. "https://{{Morningstar as a Service (EMEA)}}/token/oauth"). In a v2.1
	// export the `url` node is an object { raw, host, path, protocol }; this is its
	// `raw` field. (A plain-string `url` is also tolerated for older exports.)
	URL string
	// Folder is the slash-joined folder path locating the request inside the
	// collection tree (e.g. "Scenario Analysis/Metrics"). It disambiguates
	// requests that share a Name or URL across folders (commonly: the same
	// endpoint duplicated per region).
	Folder string
}

// RequestDetail is the full read of a request — METHOD / URL / BODY / DOC. A
// single query can match several requests (e.g. one endpoint duplicated per
// region), so the reader returns these in a slice. The Postman analogue of
// get_function.
type RequestDetail struct {
	// Request is the embedded light form (Name / Method / URL / Folder).
	Request
	// Body is `request.body.raw` returned verbatim (typically JSON), or "" when the
	// request has no body. Only the `raw` body mode is read; other modes
	// (formdata, urlencoded, graphql, file) yield "". In the reference export every
	// body is `raw`, so this covers the corpus.
	Body string
	// Doc is the request's documentation: `request.description` (Markdown text)
	// when present, falling back to the body of the first saved example in
	// `response[]` when there is no description, and "" when neither exists.
	Doc string
}

// Collection is a parsed Postman export, valid only for the duration of one call.
// The tool is stateless: every call re-reads the file from disk and re-decodes
// it.
type Collection struct {
	// requests is the flattened request tree in document order (populated by
	// Parse); folder paths are precomputed into Request.Folder.
	requests []RequestDetail
}

// --- raw schema (only the fields this tool reads) ---

type rawCollection struct {
	Item []rawItem `json:"item"`
}

// rawItem is either a folder (has Item children) or a leaf request (has Request).
type rawItem struct {
	Name     string        `json:"name"`
	Item     []rawItem     `json:"item"`
	Request  *rawRequest   `json:"request"`
	Response []rawResponse `json:"response"`
}

type rawRequest struct {
	Method      string          `json:"method"`
	URL         json.RawMessage `json:"url"`         // string or object {raw,...}
	Description json.RawMessage `json:"description"` // string or object {content,...}
	Body        *rawBody        `json:"body"`
}

type rawBody struct {
	Mode string `json:"mode"`
	Raw  string `json:"raw"`
}

type rawResponse struct {
	Name string `json:"name"`
	Body string `json:"body"`
}

// Parse decodes a Postman v2.1 export into a Collection, flattening the
// folder/request tree into an ordered list and precomputing each request's
// folder path, URL, method, body, and documentation.
func Parse(src []byte) (*Collection, error) {
	var rc rawCollection
	if err := json.Unmarshal(src, &rc); err != nil {
		return nil, err
	}
	c := &Collection{}
	var walk func(items []rawItem, folder string)
	walk = func(items []rawItem, folder string) {
		for _, it := range items {
			if it.Request != nil {
				c.requests = append(c.requests, RequestDetail{
					Request: Request{
						Name:   it.Name,
						Method: it.Request.Method,
						URL:    parseURL(it.Request.URL),
						Folder: folder,
					},
					Body: bodyRaw(it.Request.Body),
					Doc:  doc(it.Request.Description, it.Response),
				})
			}
			if len(it.Item) > 0 {
				child := it.Name
				if folder != "" {
					child = folder + "/" + it.Name
				}
				walk(it.Item, child)
			}
		}
	}
	walk(rc.Item, "")
	return c, nil
}

// parseURL extracts the raw URL from a `url` node that may be a plain string or
// an object { raw, ... }.
func parseURL(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var o struct {
		Raw string `json:"raw"`
	}
	if json.Unmarshal(raw, &o) == nil {
		return o.Raw
	}
	return ""
}

// bodyRaw returns the raw body text when the body mode is "raw", else "".
func bodyRaw(b *rawBody) string {
	if b == nil || b.Mode != "raw" {
		return ""
	}
	return b.Raw
}

// doc resolves a request's documentation: its description (string or object), or
// the first saved example response body as a fallback, or "".
func doc(desc json.RawMessage, resp []rawResponse) string {
	if d := parseDescription(desc); d != "" {
		return d
	}
	if len(resp) > 0 {
		return resp[0].Body
	}
	return ""
}

// parseDescription extracts the text from a `description` node that may be a
// plain string or an object { content, ... }.
func parseDescription(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var o struct {
		Content string `json:"content"`
	}
	if json.Unmarshal(raw, &o) == nil {
		return o.Content
	}
	return ""
}

// RequestLister lists the collection's requests — the cheap map an agent reads
// first to discover which endpoints exist. The Postman analogue of
// core.SignatureLister / `list_signatures`.
type RequestLister interface {
	// Requests returns every request in the collection in tree order. When filter
	// is non-empty it keeps only requests whose URL contains filter
	// (case-insensitive); an empty filter returns all requests.
	Requests(filter string) []Request
}

// RequestReader returns the full detail of the requests matching a query. The
// Postman analogue of core.FunctionReader / `get_function`.
type RequestReader interface {
	// Doc returns every request whose URL contains query (case-insensitive),
	// preserving tree order. It returns an empty slice when nothing matches
	// (an empty result is not an error).
	Doc(query string) []RequestDetail
}

// Compile-time assertions: a parsed Collection satisfies both read capabilities.
// It intentionally implements NO writer interface (the tool is read-only).
var (
	_ RequestLister = (*Collection)(nil)
	_ RequestReader = (*Collection)(nil)
)

// Requests lists the collection's endpoints, optionally filtered by URL substring.
func (c *Collection) Requests(filter string) []Request {
	f := strings.ToLower(filter)
	out := make([]Request, 0, len(c.requests))
	for _, r := range c.requests {
		if f == "" || strings.Contains(strings.ToLower(r.URL), f) {
			out = append(out, r.Request)
		}
	}
	return out
}

// Doc returns the full detail of every request whose URL matches query.
func (c *Collection) Doc(query string) []RequestDetail {
	q := strings.ToLower(query)
	out := make([]RequestDetail, 0)
	for _, r := range c.requests {
		if q == "" || strings.Contains(strings.ToLower(r.URL), q) {
			out = append(out, r)
		}
	}
	return out
}
