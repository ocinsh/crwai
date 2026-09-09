"""Module-level bindings used to exercise the Python declaration listing.

Python has no constant: every binding here is reported as a variable, because
inferring a constant from an upper-case name would be a guess dressed as a fact.
"""

# MAX_RETRIES caps the retry loop.
MAX_RETRIES = 3

# TIMEOUT carries an annotation, which the listed line preserves.
TIMEOUT: float = 1.5

# TABLE is bound to a multi-line literal.
TABLE = {
    "one": 1,
    "two": 2,
}


def described():
    """Return a description, so the file also holds one callable."""
    return "declarations"
