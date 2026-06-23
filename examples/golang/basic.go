// Package examples holds deterministic Go sources used to exercise the golang
// language implementation. They contain no randomness, no network calls, and no
// timestamps, and they parse cleanly.
package examples

import "strings"

// Add returns the sum of two integers.
func Add(a int, b int) int {
	return a + b
}

// Greet builds a greeting for name. It has a multi-line doc comment so the
// doc-extraction logic can be verified exactly:
//
// The second paragraph is part of the doc too, including this list:
//   - one
//   - two
func Greet(name string) string {
	return "Hello, " + strings.TrimSpace(name) + "!"
}

func noDoc() bool {
	return true
}

// Variadic sums any number of integers.
func Variadic(nums ...int) int {
	total := 0
	for _, n := range nums {
		total += n
	}
	return total
}

// MultiReturn returns a value and an error.
func MultiReturn(s string) (string, error) {
	return strings.ToUpper(s), nil
}
