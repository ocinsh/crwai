package examples

// Number constrains the generic helpers below to numeric types.
type Number interface {
	~int | ~int64 | ~float64
}

// Stack is a generic LIFO stack.
type Stack[T any] struct {
	items []T
}

// MapSlice applies f to every element of in and returns the results.
func MapSlice[T any, U any](in []T, f func(T) U) []U {
	out := make([]U, 0, len(in))
	for _, v := range in {
		out = append(out, f(v))
	}
	return out
}

// Sum adds every element of nums using the Number constraint.
func Sum[T Number](nums []T) T {
	var total T
	for _, n := range nums {
		total += n
	}
	return total
}

// Push appends x to the stack. Its receiver is a generic type (Stack[T]); the
// Container resolves to "Stack" with the type argument stripped.
func (s *Stack[T]) Push(x T) {
	s.items = append(s.items, x)
}

// Len reports the number of items on the stack.
func (s *Stack[T]) Len() int {
	return len(s.items)
}
