package postman

import (
	"os"
	"path/filepath"
	"testing"
)

const examplesDir = "../../../examples/postman"

func load(t *testing.T, name string) *Collection {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(examplesDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	c, err := Parse(b)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return c
}

// TestRequestsAndFilter checks the flattened listing and the case-insensitive URL
// filter (the `list.sh` analogue).
func TestRequestsAndFilter(t *testing.T) {
	c := load(t, "sample.postman_collection.json")

	if got := len(c.Requests("")); got != 3 {
		t.Fatalf("Requests(\"\") = %d, want 3", got)
	}
	if got := len(c.Requests("screeners")); got != 2 {
		t.Errorf("Requests(\"screeners\") = %d, want 2 (one per region)", got)
	}
	if got := len(c.Requests("TOKEN")); got != 1 {
		t.Errorf("Requests(\"TOKEN\") = %d, want 1 (case-insensitive)", got)
	}

	// Folder path locates a request inside the region/folder tree.
	var token Request
	for _, r := range c.Requests("token") {
		token = r
	}
	if token.Folder != "EMEA/Authentication" {
		t.Errorf("Token folder = %q, want EMEA/Authentication", token.Folder)
	}
	if token.Method != "POST" {
		t.Errorf("Token method = %q, want POST", token.Method)
	}
	if token.URL != "https://{{base-emea}}/token/oauth" {
		t.Errorf("Token URL = %q (placeholder must be preserved)", token.URL)
	}
}

// TestDocMultiRegion verifies a query matching the same endpoint across regions
// returns one detail per region (the `doc.sh` analogue).
func TestDocMultiRegion(t *testing.T) {
	c := load(t, "sample.postman_collection.json")

	hits := c.Doc("screeners/equities")
	if len(hits) != 2 {
		t.Fatalf("Doc(screeners/equities) = %d, want 2", len(hits))
	}
	folders := map[string]bool{}
	for _, h := range hits {
		folders[h.Folder] = true
		if h.Body != "{\"dataPoints\":[]}" {
			t.Errorf("body not extracted verbatim: %q", h.Body)
		}
	}
	for _, want := range []string{"EMEA/Screeners", "Americas/Screeners"} {
		if !folders[want] {
			t.Errorf("missing region folder %q in %v", want, folders)
		}
	}
}

// TestDocFallback verifies the documentation precedence: description first, then
// the first saved example response body, then "".
func TestDocFallback(t *testing.T) {
	c := load(t, "sample.postman_collection.json")

	// Has a description.
	if hits := c.Doc("token/oauth"); len(hits) != 1 || hits[0].Doc != "Get an OAuth token." {
		t.Errorf("Doc(token) = %+v, want description text", hits)
	}

	// No description, but a saved example response -> falls back to its body.
	for _, h := range c.Doc("screeners/equities") {
		switch h.Folder {
		case "EMEA/Screeners":
			if h.Doc != "{\"rows\":[]}" {
				t.Errorf("EMEA equities Doc = %q, want response-example fallback", h.Doc)
			}
		case "Americas/Screeners":
			if h.Doc != "" {
				t.Errorf("Americas equities Doc = %q, want empty (no description, no example)", h.Doc)
			}
		}
	}
}

// TestRealExport validates the parser against the real reference export when it
// is present (it is not committed; ~50MB). Skipped otherwise.
func TestRealExport(t *testing.T) {
	path := "../../../dist/pm_example/msd.json"
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("real export not present: %v", err)
	}
	c, err := Parse(b)
	if err != nil {
		t.Fatalf("parse real export: %v", err)
	}
	if got := len(c.Requests("")); got != 4198 {
		t.Errorf("real export request count = %d, want 4198", got)
	}
	if len(c.Doc("token/oauth")) == 0 {
		t.Error("expected at least one token/oauth request in the real export")
	}
}
