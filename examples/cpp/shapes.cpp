// shapes.cpp — geometric shapes used to exercise the C++ language capabilities.
// Deterministic: no I/O, no randomness, no timestamps.

#include <string>

// PI is a compile-time constant used by the area helpers.
const double PI = 3.14159265358979323846;

// area computes the area of a circle from its radius. This FREE function shares
// its name with Circle::area to exercise SymbolID disambiguation by container.
double area(double radius) {
    return PI * radius * radius;
}

/// Circle is a simple shape with a radius.
/// It demonstrates a documented class with member functions.
class Circle {
public:
    // Circle constructs a circle of the given radius.
    Circle(double r) : radius_(r) {}

    // area returns the area of this circle. Same name as the free function
    // area(double) above, but reached via container "Circle".
    double area() const {
        return PI * radius_ * radius_;
    }

    // circumference returns the perimeter of the circle.
    double circumference() const {
        return 2.0 * PI * radius_;
    }

private:
    double radius_;
};

/* Rectangle is a width-by-height shape.
   This block comment documents the struct. */
struct Rectangle {
    double width;
    double height;

    // area returns width times height.
    double area() const {
        return width * height;
    }
};

// describe returns a human-readable label for a shape name. It has a multi-line
// doc comment so doc-extraction can be checked against an exact expected string.
std::string describe(const std::string& kind) {
    return "shape: " + kind;
}
