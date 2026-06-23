/** Geometry groups planar helpers. */
namespace Geometry {
  /** perimeter returns the perimeter of a rectangle. */
  export function perimeter(w: number, h: number): number {
    return 2 * (w + h);
  }

  function diagonalSquared(w: number, h: number): number {
    return w * w + h * h;
  }
}

module Strings {
  export const shout = (s: string): string => s.toUpperCase();
}

// perimeter is a free function sharing its name with Geometry.perimeter.
function perimeter(side: number): number {
  return 4 * side;
}
