/// Wraps a value of any type.
struct Wrapper<T> {
    value: T,
}

/// Returns the larger of two comparable values.
fn max_of<T: PartialOrd>(a: T, b: T) -> T {
    if a > b {
        a
    } else {
        b
    }
}

/// Identity function constrained by a where-clause.
fn identity<T>(x: T) -> T
where
    T: Clone,
{
    x
}
