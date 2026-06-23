/// A simple stack of integers.
struct Stack {
    items: Vec<i32>,
}

impl Stack {
    /// Creates an empty stack.
    fn new() -> Stack {
        Stack { items: Vec::new() }
    }

    /// Pushes a value onto the stack.
    fn push(&mut self, value: i32) {
        self.items.push(value);
    }

    /// Returns the number of items on the stack.
    fn size(&self) -> usize {
        self.items.len()
    }
}

/// Free function named `size`, distinct from Stack::size.
fn size() -> usize {
    0
}
