/**
 * Shape is anything with a measurable area.
 * @returns the area when area() is called
 */
interface Shape {
  /** area returns the shape's area. */
  area(): number;
  name: string;
}

/** Circle is a round shape. */
class Circle implements Shape {
  name: string;

  constructor(private r: number) {
    this.name = "circle";
  }

  /** area returns the circle's area. */
  area(): number {
    return 3.14159 * this.r * this.r;
  }

  /** unit builds the unit circle. */
  static unit(): Circle {
    return new Circle(1);
  }
}

/** Square is a four-sided shape. */
class Square implements Shape {
  name: string;

  constructor(private side: number) {
    this.name = "square";
  }

  area(): number {
    return this.side * this.side;
  }
}

// area is a free function sharing its name with the methods above.
function area(w: number, h: number): number {
  return w * h;
}
