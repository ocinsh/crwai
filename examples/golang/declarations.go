// Package examples holds deterministic Go sources used to exercise the golang
// language implementation.
//
// This file carries only declarations that have no body: the kinds a listing
// reported as nothing at all until const, var and type became addressable.
package examples

import "errors"

// MaxRetries caps the retry loop.
const MaxRetries = 3

const (
	// KindLine is a line comment.
	KindLine = "line"
	// KindBlock is a block comment.
	KindBlock = "block"
)

// ErrEmpty is a standalone sentinel.
var ErrEmpty = errors.New("empty")

var (
	// ErrTooBig is one entry of a grouped declaration.
	ErrTooBig = errors.New("too big")
	// ErrTooSmall is another, and must be addressable on its own.
	ErrTooSmall = errors.New("too small")
)

// Table is bound to a multi-line literal, so its listed line is only the first.
var Table = map[string]int{
	"one": 1,
	"two": 2,
}

// Handler is a function type.
type Handler func(int) error

// Weight is a named scalar type.
type Weight float64

// Pair is a struct and must keep reading as one.
type Pair struct {
	Left, Right int
}

// Sum adds the pair, so the file also holds one callable.
func (p Pair) Sum() int { return p.Left + p.Right }
