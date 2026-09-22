package lang

import (
	"sort"
	"strings"

	"github.com/ocinsh/crwai/internal/core"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type bodySpan struct {
	start, end uint
	text       string
}

// Preview returns the original file with callable bodies replaced by retrieval
// hints. Documentation and all declarations outside those bodies stay verbatim.
func Preview(src core.Source, language string) string {
	var spans []bodySpan
	var walk func(*sitter.Node)
	walk = func(node *sitter.Node) {
		if node == nil {
			return
		}
		if language == "dart" && node.Kind() == "function_body" {
			spans = append(spans, previewSpan(node, src.Bytes()))
			return
		}
		if callableNode(node.Kind()) {
			if body := node.ChildByFieldName("body"); body != nil {
				if language == "python" {
					spans = append(spans, pythonBodySpan(body))
				} else {
					spans = append(spans, previewSpan(body, src.Bytes()))
				}
				for i := uint(0); i < node.NamedChildCount(); i++ {
					child := node.NamedChild(i)
					if child.StartByte() != body.StartByte() || child.EndByte() != body.EndByte() {
						walk(child)
					}
				}
				return
			}
		}
		for i := uint(0); i < node.NamedChildCount(); i++ {
			walk(node.NamedChild(i))
		}
	}
	walk(src.Root())
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	var out strings.Builder
	previous := uint(0)
	for _, span := range spans {
		if span.start < previous || span.end > uint(len(src.Bytes())) {
			continue
		}
		out.Write(src.Bytes()[previous:span.start])
		out.WriteString(span.text)
		previous = span.end
	}
	out.Write(src.Bytes()[previous:])
	return out.String()
}

func callableNode(kind string) bool {
	return strings.Contains(kind, "function") || strings.Contains(kind, "method") || strings.Contains(kind, "constructor") || strings.Contains(kind, "lambda")
}

func previewSpan(body *sitter.Node, source []byte) bodySpan {
	text := "/* use get_function */"
	if bytes := source[body.StartByte():body.EndByte()]; len(bytes) >= 2 && bytes[0] == '{' && bytes[len(bytes)-1] == '}' {
		text = "{ /* use get_function */ }"
	}
	return bodySpan{body.StartByte(), body.EndByte(), text}
}

func pythonBodySpan(body *sitter.Node) bodySpan {
	start := body.StartByte()
	text := "# use get_function"
	if body.NamedChildCount() > 0 {
		first := body.NamedChild(0)
		if first.Kind() == "expression_statement" && first.NamedChildCount() > 0 && first.NamedChild(0).Kind() == "string" {
			start = first.EndByte()
			text = "\n" + strings.Repeat(" ", int(first.StartPosition().Column)) + text
		}
	}
	return bodySpan{start, body.EndByte(), text}
}
