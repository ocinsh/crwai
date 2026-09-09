// Bodyless top-level bindings used to exercise the JavaScript listing.

// MAX caps the retry loop.
export const MAX = 3;

// counter is mutable, so it binds as a variable rather than a constant.
let counter = 0;

// legacy uses the older binding form.
var legacy = "declarations";

// TABLE is bound to a multi-line literal.
const TABLE = {
  one: 1,
  two: 2,
};

// sum adds two numbers, so the file also holds one callable.
export function sum(a, b) {
  return a + b;
}
