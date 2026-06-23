"""A larger module (>= 20 symbols) to test list_signatures on a realistic file."""


def step01(x):
    """Add one."""
    return x + 1


def step02(x):
    return x + 2


def step03(x):
    return x + 3


def step04(x):
    return x + 4


def step05(x):
    return x + 5


def step06(x):
    return x + 6


def step07(x):
    return x + 7


def step08(x):
    return x + 8


def step09(x):
    return x + 9


def step10(x):
    return x + 10


def step11(x):
    return x + 11


def step12(x):
    return x + 12


def step13(x):
    return x + 13


def step14(x):
    return x + 14


def step15(x):
    return x + 15


def step16(x):
    return x + 16


def step17(x):
    return x + 17


def step18(x):
    return x + 18


class Accumulator:
    """Stateful accumulator over the step functions."""

    def __init__(self):
        self.total = 0

    def add(self, value):
        """Add value to the running total."""
        self.total += value
        return self.total
