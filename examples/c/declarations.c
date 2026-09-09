/* Bodyless top-level declarations used to exercise the C listing. */

/* MAX caps the retry loop. */
#define MAX 3

/* counter is a file-scope global. */
static int counter = 0;

/* Id names a scalar type. */
typedef int Id;

/* Point is an aggregate typedef and must keep reading as a struct. */
typedef struct {
  int x;
  int y;
} Point;

/* sum adds two numbers, so the file also holds one callable. */
int sum(int a, int b) { return a + b; }
