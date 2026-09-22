package lang_test

import (
	"strings"
	"testing"

	"github.com/ocinsh/crwai"
)

func TestSeeFilePreservesDocumentationAndHidesBodies(t *testing.T) {
	cases := []struct {
		path, documentation, signature, body string
	}{
		{"../../examples/golang/methods.go", "// Inc increments the counter", "func (c *Counter) Inc() int", "c.n++"},
		{"../../examples/python/shapes.py", `"""Return the area of a rectangle."""`, "def area(width, height):", "return width * height"},
		{"../../examples/java/Crlf.java", "class Crlf", "class Crlf", "return 42"},
		{"../../examples/javascript/closures.js", "// identity returns", "const identity = (x) =>", "return x;"},
		{"../../examples/typescript/closures.ts", "//", "const", "return x"},
		{"../../examples/typescript/widget.tsx", "Greeting renders a friendly hello", "function Greeting", "Hello, {props.name}!"},
		{"../../examples/c/shapes.c", "struct Point", "struct Point", "return a + b"},
		{"../../examples/cpp/shapes.cpp", "class", "class", "return a + b"},
		{"../../examples/rust/edge.rs", "/// Demonstrates a nested function", "fn outer() -> i32", "double(inner())"},
		{"../../examples/dart/declarations.dart", "/// sum adds two numbers", "int sum(int a, int b)", "a + b"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			view, err := crwai.New().SeeFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			if len(view.Signatures) == 0 || !strings.Contains(view.Content, tc.documentation) || !strings.Contains(view.Content, tc.signature) {
				t.Fatalf("missing index, documentation, or signature in preview of %s", tc.path)
			}
			if strings.Contains(view.Content, tc.body) || !strings.Contains(view.Content, "use get_function") {
				t.Fatalf("body not hidden in preview of %s", tc.path)
			}
		})
	}
}
