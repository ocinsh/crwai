// Shape is a base class for 2D shapes.
class Shape {
  // area returns the shape's area.
  area() {
    return 0;
  }

  describe() {
    return "a shape";
  }
}

class Circle {
  constructor(r) {
    this.r = r;
  }

  // area returns the circle's area.
  area() {
    return 3.14159 * this.r * this.r;
  }
}

// area is a free function that shares its name with the methods above; it is
// disambiguated by an empty container.
function area(w, h) {
  return w * h;
}

module.exports = { Shape, Circle, area };
