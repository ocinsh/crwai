/// Returns a sushi emoji. The function name is a Unicode identifier.
fn 寿司() -> &'static str {
    "🍣"
}

/// Demonstrates a nested function and a closure.
fn outer() -> i32 {
    fn inner() -> i32 {
        21
    }
    let double = |x: i32| x * 2;
    double(inner())
}
