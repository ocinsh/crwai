/// Shape can compute its own area.
trait Shape {
    /// Returns the area of the shape.
    fn area(&self) -> f64;
}

/// A circle with a radius.
struct Circle {
    radius: f64,
}

impl Shape for Circle {
    /// Area of the circle: pi * r^2.
    fn area(&self) -> f64 {
        3.14159 * self.radius * self.radius
    }
}
