package examples

// This file is intentionally large (well over 20 top-level symbols) to exercise
// list_signatures on a realistic file. The symbols are trivial and deterministic.

// BigConst01 doubles its argument.
func BigConst01(n int) int { return n * 2 }

// BigConst02 triples its argument.
func BigConst02(n int) int { return n * 3 }

// BigConst03 negates its argument.
func BigConst03(n int) int { return -n }

// BigConst04 returns the absolute value.
func BigConst04(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// BigConst05 returns true for even numbers.
func BigConst05(n int) bool { return n%2 == 0 }

// BigConst06 returns true for odd numbers.
func BigConst06(n int) bool { return n%2 != 0 }

// BigConst07 returns the larger of a and b.
func BigConst07(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// BigConst08 returns the smaller of a and b.
func BigConst08(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// BigConst09 clamps n into [lo, hi].
func BigConst09(n, lo, hi int) int {
	return BigConst07(lo, BigConst08(n, hi))
}

// BigConst10 squares its argument.
func BigConst10(n int) int { return n * n }

// BigConst11 cubes its argument.
func BigConst11(n int) int { return n * n * n }

// BigConst12 increments its argument.
func BigConst12(n int) int { return n + 1 }

// BigConst13 decrements its argument.
func BigConst13(n int) int { return n - 1 }

// BigConst14 returns the sign of n.
func BigConst14(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	default:
		return 0
	}
}

// BigConst15 sums a slice.
func BigConst15(xs []int) int {
	total := 0
	for _, x := range xs {
		total += x
	}
	return total
}

// BigConst16 returns the length of a slice.
func BigConst16(xs []int) int { return len(xs) }

// BigConst17 reverses a slice in place and returns it.
func BigConst17(xs []int) []int {
	for i, j := 0, len(xs)-1; i < j; i, j = i+1, j-1 {
		xs[i], xs[j] = xs[j], xs[i]
	}
	return xs
}

// BigConst18 repeats n, count times, into a slice.
func BigConst18(n, count int) []int {
	out := make([]int, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, n)
	}
	return out
}

// BigConst19 returns the max of a slice (0 for empty).
func BigConst19(xs []int) int {
	if len(xs) == 0 {
		return 0
	}
	m := xs[0]
	for _, x := range xs[1:] {
		m = BigConst07(m, x)
	}
	return m
}

// BigConst20 returns the min of a slice (0 for empty).
func BigConst20(xs []int) int {
	if len(xs) == 0 {
		return 0
	}
	m := xs[0]
	for _, x := range xs[1:] {
		m = BigConst08(m, x)
	}
	return m
}

// BigConst21 reports whether xs contains v.
func BigConst21(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// BigConst22 returns a copy of xs.
func BigConst22(xs []int) []int {
	out := make([]int, len(xs))
	copy(out, xs)
	return out
}
