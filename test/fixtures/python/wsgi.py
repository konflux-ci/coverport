"""WSGI Flask entry for the Python Kind/HTTP e2e fixture.

Pattern D (pytest-cov) imports greet from app.py only; this module is for
Gunicorn + coverage_server.py container collection.
"""

from flask import Flask, request

from app import greet

app = Flask(__name__)


@app.get("/hello")
def hello():
    name = request.args.get("name", "")
    return greet(name) + "\n", 200, {"Content-Type": "text/plain"}
