// makeCounter returns a closure that increments a captured count.
function makeCounter(start: number): () => number {
  let count = start;
  function next(): number {
    count += 1;
    return count;
  }
  return next;
}

/** identity returns its argument unchanged. */
const identity = (x: number): number => x;

// pair builds a two-element tuple from its arguments.
const pair = (a: number, b: number): [number, number] => [a, b];
