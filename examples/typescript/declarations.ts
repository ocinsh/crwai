// Bodyless top-level declarations used to exercise the TypeScript listing.

// MAX caps the retry loop.
export const MAX: number = 3;

// counter is mutable, so it binds as a variable rather than a constant.
export let counter = 0;

// Id is a type alias over a scalar, which names a type without describing a shape.
export type Id = string;

// Pair is an object-typed alias, which describes a shape and reads as a struct.
export type Pair = { left: number; right: number };

// sum adds the pair, so the file also holds one callable.
export function sum(p: Pair): number {
  return p.left + p.right;
}
