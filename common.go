package crwai

import (
	"os"

	"github.com/ocinsh/crwai/internal/common/markdown"
	"github.com/ocinsh/crwai/internal/common/postman"
)

// This file is the public surface of the COMMON-FILE tools: utilities that read
// documents which are not code, at a granularity that is useful on its own. They
// are deliberately kept apart from Reader / Writer / Service because they are not
// bound by the Language contract:
//
//   - they are never routed by file extension through the language registry, so a
//     caller selects the tool explicitly (a named CLI subcommand, a named MCP
//     tool) rather than having it inferred;
//   - they do not use tree-sitter, and carry no CGO;
//   - a target is addressed by the file's own identity — a heading path, a request
//     URL — instead of by a code symbol.
//
// What they share with the code tools is the discipline: stateless calls that
// re-read the file every time, a cheap listing before any full read, text placed
// exactly as the caller supplied it, and the same all-or-nothing atomic write path.

// The common-file data types, aliased from their internal packages so a consumer
// of this facade never has to import internal/.
type (
	// Heading is one node of a Markdown outline: level, text, the slash-joined
	// heading path that identifies it, and its line number.
	Heading = markdown.Heading

	// Section is a Markdown heading together with the content beneath it, down to
	// the next heading of equal or shallower level.
	Section = markdown.Section

	// SectionEdit is a requested rewrite of one Markdown section, addressed by
	// heading path. The caller supplies the whole replacement block.
	SectionEdit = markdown.SectionEdit

	// Request is the light descriptor of one endpoint in a Postman collection.
	Request = postman.Request

	// RequestDetail is the full read of one Postman request: method, URL, body,
	// and documentation.
	RequestDetail = postman.RequestDetail
)

// DocReader is the read surface of the common-file tools. Each method re-reads and
// re-parses the file, exactly like the code readers, and each keeps the same
// cheap-map-then-fetch shape: outline a document before reading one section, list
// the endpoints before reading one request.
type DocReader interface {
	// Outline returns every heading of a Markdown file in document order — the
	// cheap map, with no section content loaded.
	Outline(path string) ([]Heading, error)

	// Section returns one Markdown section, addressed by its heading path (e.g.
	// "Usage/Flags"). An unknown path returns ErrSymbolNotFound.
	Section(path, heading string) (Section, error)

	// Requests lists the endpoints of a Postman collection export, keeping only
	// those whose URL contains filter (case-insensitive; an empty filter keeps
	// all of them).
	Requests(path, filter string) ([]Request, error)

	// Request returns the full detail of every endpoint whose URL contains query.
	// It is a slice because one endpoint is commonly duplicated across folders,
	// for instance once per region. No match is an empty result, not an error.
	Request(path, query string) ([]RequestDetail, error)
}

// DocWriter is the write surface of the common-file tools. Only Markdown is
// writable: a Postman export is read-only by design, so no method here targets one.
type DocWriter interface {
	// WriteSections replaces one or more Markdown sections with caller-supplied
	// text in a single atomic batch, through the same all-or-nothing pipeline the
	// code writer uses. An empty batch returns ErrNoEdits.
	WriteSections(path string, edits ...SectionEdit) (WriteResult, error)
}

// DocService is the full common-file surface. *Engine implements it alongside
// Service, so one engine drives both families, but the two interfaces stay
// separate because the contracts are unrelated.
type DocService interface {
	DocReader
	DocWriter
}

// Outline returns the heading outline of the Markdown file at path.
func (e *Engine) Outline(path string) ([]Heading, error) {
	doc, err := e.markdown(path)
	if err != nil {
		return nil, err
	}
	return doc.Outline(), nil
}

// Section returns the Markdown section addressed by the heading path.
func (e *Engine) Section(path, heading string) (Section, error) {
	doc, err := e.markdown(path)
	if err != nil {
		return Section{}, err
	}
	return doc.Section(heading)
}

// WriteSections applies a batch of Markdown section edits atomically.
func (e *Engine) WriteSections(path string, edits ...SectionEdit) (WriteResult, error) {
	if err := e.allow(path); err != nil {
		return WriteResult{Path: path}, err
	}
	return markdown.WriteSections(path, edits)
}

// Requests lists the endpoints of the Postman collection at path.
func (e *Engine) Requests(path, filter string) ([]Request, error) {
	col, err := e.postman(path)
	if err != nil {
		return nil, err
	}
	return col.Requests(filter), nil
}

// Request returns the full detail of the Postman endpoints matching query.
func (e *Engine) Request(path, query string) ([]RequestDetail, error) {
	col, err := e.postman(path)
	if err != nil {
		return nil, err
	}
	return col.Doc(query), nil
}

// markdown reads and parses a Markdown file for the duration of one call, after
// checking it against the configured root.
func (e *Engine) markdown(path string) (*markdown.Doc, error) {
	b, err := e.readDoc(path)
	if err != nil {
		return nil, err
	}
	return markdown.Parse(b)
}

// postman reads and decodes a Postman collection export for the duration of one
// call, after checking it against the configured root.
func (e *Engine) postman(path string) (*postman.Collection, error) {
	b, err := e.readDoc(path)
	if err != nil {
		return nil, err
	}
	return postman.Parse(b)
}

// readDoc is the shared front half of every common-file read: enforce the root,
// then read the bytes. The tools are selected explicitly, so there is no language
// resolution step here.
func (e *Engine) readDoc(path string) ([]byte, error) {
	if err := e.allow(path); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}
