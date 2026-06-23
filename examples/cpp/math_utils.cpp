// math_utils.cpp — templates, a nested namespace, and an out-of-line method
// definition, used to exercise generics and namespace::class containers.

namespace mind {
namespace math {

// clamp restricts v to the inclusive range [lo, hi].
// This is a multi-line doc comment spanning
// three source lines on purpose, so doc extraction
// can be asserted exactly.
template <typename T>
T clamp(T v, T lo, T hi) {
    if (v < lo) return lo;
    if (v > hi) return hi;
    return v;
}

// Accumulator sums values incrementally.
class Accumulator {
public:
    // add folds value into the running total.
    void add(double value);

    // total returns the accumulated sum.
    double total() const { return sum_; }

private:
    double sum_ = 0.0;
};

} // namespace math
} // namespace mind

// add is an OUT-OF-LINE definition of mind::math::Accumulator::add. Its container
// is "mind::math::Accumulator" and its name is "add".
void mind::math::Accumulator::add(double value) {
    sum_ += value;
}

// identity returns its argument unchanged (a second template, free function).
template <typename T>
T identity(T value) {
    return value;
}
