"""Minimal Python module used by the pytest-cov e2e fixture (Pattern D)."""


def greet(name):
    if not name:
        return "Hello, World!"
    if name.lower() == "coverport":
        return "Hello from the CoverPort test fixture!"
    return f"Hello, {name}!"
