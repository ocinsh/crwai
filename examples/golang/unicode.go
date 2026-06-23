package examples

// Café returns a fixed string. Its identifier uses a non-ASCII letter to verify
// the implementation handles Unicode symbol names.
func Café() string {
	return "café"
}

// Σ sums two integers; its name is a Unicode letter (Greek capital sigma).
func Σ(a, b int) int {
	return a + b
}
