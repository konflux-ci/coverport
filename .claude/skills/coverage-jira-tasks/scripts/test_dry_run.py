"""Tests for _repo_url GitLab detection in dry-run.py."""

import importlib.util
import os
import sys
from io import StringIO
from unittest.mock import patch

import pytest

# Import dry-run.py despite the hyphenated filename
_script_path = os.path.join(os.path.dirname(__file__), "dry-run.py")
_spec = importlib.util.spec_from_file_location("dry_run", _script_path)
dry_run = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(dry_run)

_repo_url = dry_run._repo_url


class TestRepoUrlGitLabDetection:
    """Verify GitLab fallback warning in _repo_url uses CI System column."""

    def test_gitlab_warning_fires_when_ci_system_is_gitlab_and_url_empty(self, capsys):
        """GitLab warning fires for CI System='GitLab CI' with empty URL."""
        row = {"Repository": "my-repo", "URL": "", "CI System": "GitLab CI"}
        result = _repo_url(row, "my-org")

        assert result == "https://github.com/my-org/my-repo"
        captured = capsys.readouterr()
        assert "WARNING" in captured.out
        assert "my-repo" in captured.out
        assert "falling back to GitHub URL" in captured.out

    def test_no_warning_for_github_repos(self, capsys):
        """Warning does NOT fire for GitHub repos (CI System='GitHub Actions')."""
        row = {"Repository": "my-repo", "URL": "", "CI System": "GitHub Actions"}
        result = _repo_url(row, "my-org")

        assert result == "https://github.com/my-org/my-repo"
        captured = capsys.readouterr()
        assert captured.out == ""

    def test_no_warning_when_url_is_populated(self, capsys):
        """No warning when URL is populated, even for GitLab repos."""
        row = {
            "Repository": "my-repo",
            "URL": "https://gitlab.com/my-org/my-repo",
            "CI System": "GitLab CI",
        }
        result = _repo_url(row, "my-org")

        assert result == "https://gitlab.com/my-org/my-repo"
        captured = capsys.readouterr()
        assert captured.out == ""

    def test_no_warning_when_ci_system_empty(self, capsys):
        """No warning when CI System column is empty."""
        row = {"Repository": "my-repo", "URL": "", "CI System": ""}
        result = _repo_url(row, "my-org")

        assert result == "https://github.com/my-org/my-repo"
        captured = capsys.readouterr()
        assert captured.out == ""

    def test_no_warning_when_ci_system_missing(self, capsys):
        """No warning when CI System column is absent from the row."""
        row = {"Repository": "my-repo", "URL": ""}
        result = _repo_url(row, "my-org")

        assert result == "https://github.com/my-org/my-repo"
        captured = capsys.readouterr()
        assert captured.out == ""
