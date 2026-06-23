package examples

import "strings"

// Counter is a simple integer counter used to exercise methods with receivers.
type Counter struct {
	n int
}

// Greeter greets with a configurable prefix.
type Greeter struct {
	prefix string
}

// Inc increments the counter and returns the new value.
func (c *Counter) Inc() int {
	c.n++
	return c.n
}

// Value returns the current counter value (value receiver).
func (c Counter) Value() int {
	return c.n
}

// Reset zeroes the counter. Its name collides with Greeter.Reset to exercise
// container-based disambiguation.
func (c *Counter) Reset() {
	c.n = 0
}

// Reset clears the greeter prefix. Same method name, different container.
func (g *Greeter) Reset() {
	g.prefix = ""
}

// Greet returns a greeting. This method shares its name with the top-level Greet
// function in basic.go; only the Container ("Greeter") tells them apart.
func (g *Greeter) Greet(name string) string {
	return strings.TrimSpace(g.prefix) + " " + name
}
