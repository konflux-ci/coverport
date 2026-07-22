"""Tests for the Python pytest-cov e2e fixture."""

from app import greet


def test_greet_empty():
    assert greet("") == "Hello, World!"


def test_greet_name():
    assert greet("test") == "Hello, test!"


def test_greet_coverport():
    assert greet("coverport") == "Hello from the CoverPort test fixture!"
