// A deterministic mix of many symbols to exercise the signature lister on a
// realistic file. No randomness, no I/O, no timestamps.

/** Point is a 2D coordinate. */
type Point = { x: number; y: number };

/** Named is anything with a name. */
interface Named {
  /** label returns the display name. */
  label(): string;
}

export function clamp(value: number, lo: number, hi: number): number {
  if (value < lo) return lo;
  if (value > hi) return hi;
  return value;
}

function sign(n: number): number {
  if (n > 0) return 1;
  if (n < 0) return -1;
  return 0;
}

function abs(n: number): number {
  return n < 0 ? -n : n;
}

function max(a: number, b: number): number {
  return a > b ? a : b;
}

function min(a: number, b: number): number {
  return a < b ? a : b;
}

const double = (n: number): number => n * 2;

const half = (n: number): number => n / 2;

const negate = (n: number): number => -n;

/** Vector is a simple 2D vector with arithmetic. */
class Vector implements Named {
  constructor(
    public x: number,
    public y: number,
  ) {}

  label(): string {
    return `(${this.x}, ${this.y})`;
  }

  add(other: Vector): Vector {
    return new Vector(this.x + other.x, this.y + other.y);
  }

  scale(k: number): Vector {
    return new Vector(this.x * k, this.y * k);
  }

  length(): number {
    return Math.sqrt(this.x * this.x + this.y * this.y);
  }

  static zero(): Vector {
    return new Vector(0, 0);
  }
}

function dot(a: Vector, b: Vector): number {
  return a.x * b.x + a.y * b.y;
}

function distance(a: Point, b: Point): number {
  const dx = a.x - b.x;
  const dy = a.y - b.y;
  return Math.sqrt(dx * dx + dy * dy);
}

const sum = (xs: number[]): number => xs.reduce((a, b) => a + b, 0);

const product = (xs: number[]): number => xs.reduce((a, b) => a * b, 1);

function average(xs: number[]): number {
  return xs.length === 0 ? 0 : sum(xs) / xs.length;
}
