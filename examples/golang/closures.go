package examples

// MakeAdder returns a closure that adds x to its argument.
func MakeAdder(x int) func(int) int {
	return func(y int) int {
		return x + y
	}
}

// ApplyTwice applies f to v two times using a nested helper closure.
func ApplyTwice(v int, f func(int) int) int {
	step := func(n int) int {
		return f(n)
	}
	return step(step(v))
}
