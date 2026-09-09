// Bodyless top-level declarations used to exercise the C++ listing.

// kMax caps the retry loop.
const int kMax = 3;

// counter is a file-scope global.
static int counter = 0;

// Id names a type through the modern alias form.
using Id = unsigned long;

// Legacy names a type through the older typedef form.
typedef int Legacy;

// sum adds two numbers, so the file also holds one callable.
int sum(int a, int b) { return a + b; }
