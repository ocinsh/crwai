// big.cpp — a realistically sized file with many symbols (>= 20) to exercise
// list_signatures on a non-trivial input. Deterministic and self-contained.

namespace calc {

// add returns a + b.
int add(int a, int b) { return a + b; }

// sub returns a - b.
int sub(int a, int b) { return a - b; }

// mul returns a * b.
int mul(int a, int b) { return a * b; }

// divi returns a / b (integer division; b assumed non-zero).
int divi(int a, int b) { return a / b; }

// mod returns a % b.
int mod(int a, int b) { return a % b; }

// neg returns -a.
int neg(int a) { return -a; }

// absv returns the absolute value of a.
int absv(int a) { return a < 0 ? -a : a; }

// maxi returns the larger of a and b.
int maxi(int a, int b) { return a > b ? a : b; }

// mini returns the smaller of a and b.
int mini(int a, int b) { return a < b ? a : b; }

// square returns a * a.
int square(int a) { return a * a; }

// cube returns a * a * a.
int cube(int a) { return a * a * a; }

// sign returns -1, 0 or 1 according to a.
int sign(int a) { return a < 0 ? -1 : (a > 0 ? 1 : 0); }

} // namespace calc

// Counter is a monotonic integer counter.
struct Counter {
    int value;

    // inc increments the counter and returns the new value.
    int inc() { return ++value; }

    // dec decrements the counter and returns the new value.
    int dec() { return --value; }

    // reset sets the counter back to zero.
    void reset() { value = 0; }

    // get returns the current value.
    int get() const { return value; }
};

// Stack is a tiny fixed-capacity integer stack.
class Stack {
public:
    // push stores v on top of the stack.
    void push(int v) { data_[size_++] = v; }

    // pop removes and returns the top element.
    int pop() { return data_[--size_]; }

    // empty reports whether the stack has no elements.
    bool empty() const { return size_ == 0; }

    // size returns the number of stored elements.
    int size() const { return size_; }

private:
    int data_[16];
    int size_ = 0;
};

// run is a free function tying the helpers together.
int run() { return calc::add(1, 2) + calc::mul(3, 4); }
