//! Bodyless top-level items used to exercise the Rust declaration listing.

/// MAX caps the retry loop.
pub const MAX: usize = 3;

/// NAME is a static, which binds once for the whole program.
pub static NAME: &str = "declarations";

/// Id is a type alias.
pub type Id = u64;

/// Op is an enum, which reads as a struct exactly as it does in C and Java.
pub enum Op {
    Add,
    Sub,
}

/// Pair is a struct and must keep reading as one.
pub struct Pair {
    pub left: i32,
    pub right: i32,
}

/// sum adds the pair, so the file also holds one callable.
pub fn sum(p: &Pair) -> i32 {
    p.left + p.right
}
