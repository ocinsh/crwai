package examples

// Point is a 2D coordinate with documented fields.
type Point struct {
	// X is the horizontal coordinate.
	X int
	// Y is the vertical coordinate.
	Y int
}

// Shape is anything with an area and a perimeter. It carries a multi-line doc
// comment used to verify interface doc-extraction:
//
// Implementations must be deterministic.
type Shape interface {
	// Area returns the enclosed area.
	Area() float64
	// Perimeter returns the boundary length.
	Perimeter() float64
}

// Stringer mirrors fmt.Stringer for the interface-reading tests.
type Stringer interface {
	String() string
}
