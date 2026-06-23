//! A larger, realistic-looking module used to exercise list_signatures on a file
//! with many symbols (functions, methods, a struct, and a trait).

/// Returns the constant zero.
fn c01() -> i32 {
    0
}

/// Returns the constant one.
fn c02() -> i32 {
    1
}

/// Doubles the input.
fn c03(x: i32) -> i32 {
    x * 2
}

/// Halves the input using integer division.
fn c04(x: i32) -> i32 {
    x / 2
}

/// Negates the input.
fn c05(x: i32) -> i32 {
    -x
}

/// Returns the absolute value.
fn c06(x: i32) -> i32 {
    if x < 0 {
        -x
    } else {
        x
    }
}

/// Adds two numbers.
fn c07(a: i32, b: i32) -> i32 {
    a + b
}

/// Subtracts b from a.
fn c08(a: i32, b: i32) -> i32 {
    a - b
}

/// Multiplies two numbers.
fn c09(a: i32, b: i32) -> i32 {
    a * b
}

/// Returns the larger of two numbers.
fn c10(a: i32, b: i32) -> i32 {
    if a > b {
        a
    } else {
        b
    }
}

/// Returns the smaller of two numbers.
fn c11(a: i32, b: i32) -> i32 {
    if a < b {
        a
    } else {
        b
    }
}

/// Squares the input.
fn c12(x: i32) -> i32 {
    x * x
}

/// Returns true when the input is even.
fn c13(x: i32) -> bool {
    x % 2 == 0
}

/// Returns true when the input is odd.
fn c14(x: i32) -> bool {
    x % 2 != 0
}

/// Clamps a value into the inclusive range [lo, hi].
fn c15(x: i32, lo: i32, hi: i32) -> i32 {
    if x < lo {
        lo
    } else if x > hi {
        hi
    } else {
        x
    }
}

/// Sums a slice of integers.
fn c16(xs: &[i32]) -> i32 {
    let mut total = 0;
    for x in xs {
        total += x;
    }
    total
}

/// A tiny accumulator.
struct Calc {
    total: i32,
}

impl Calc {
    /// Creates a calculator starting at zero.
    fn new() -> Calc {
        Calc { total: 0 }
    }

    /// Adds a value to the running total.
    fn add(&mut self, x: i32) {
        self.total += x;
    }

    /// Resets the running total to zero.
    fn reset(&mut self) {
        self.total = 0;
    }

    /// Returns the running total.
    fn value(&self) -> i32 {
        self.total
    }
}

/// A unary integer operation.
trait Op {
    /// Applies the operation to x.
    fn apply(&self, x: i32) -> i32;

    /// Returns the human-readable name of the operation.
    fn name(&self) -> &str;
}
