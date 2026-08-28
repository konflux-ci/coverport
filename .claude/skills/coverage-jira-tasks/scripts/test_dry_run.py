#!/usr/bin/env python3
"""Unit tests for the URL-construction helpers in dry-run.py.

These helpers (`_codecov_provider`, `_codecov_repo_path`, `_codecov_url`,
`_repo_url`) are pure functions over CSV row dicts, so they are cheap to test
and are exactly where past regressions have landed (see issue #110 / PR #95).

Run:
    cd .claude/skills/coverage-jira-tasks/scripts
    python -m unittest test_dry_run        # stdlib, no external deps
    # or, if pytest is available:
    python -m pytest test_dry_run.py

Uses only the standard library to preserve the repo's zero-external-deps
convention for skill scripts.
"""

import importlib.util
import io
import os
import unittest
from contextlib import redirect_stdout

# dry-run.py isn't an importable module name (the hyphen is illegal in an
# identifier), so load it from its file path.
_SCRIPT = os.path.join(os.path.dirname(__file__), "dry-run.py")
_spec = importlib.util.spec_from_file_location("dry_run", _SCRIPT)
dry_run = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(dry_run)


class TestCodecovProvider(unittest.TestCase):
    def test_github_url_returns_gh(self):
        self.assertEqual(dry_run._codecov_provider({"URL": "https://github.com/org/repo"}), "gh")

    def test_gitlab_url_returns_gl(self):
        self.assertEqual(dry_run._codecov_provider({"URL": "https://gitlab.com/group/repo"}), "gl")

    def test_empty_url_defaults_to_gh(self):
        self.assertEqual(dry_run._codecov_provider({"URL": ""}), "gh")
        self.assertEqual(dry_run._codecov_provider({}), "gh")

    def test_detection_is_case_insensitive(self):
        self.assertEqual(dry_run._codecov_provider({"URL": "https://GitLab.com/g/r"}), "gl")


class TestCodecovRepoPath(unittest.TestCase):
    def test_github_url(self):
        self.assertEqual(
            dry_run._codecov_repo_path({"URL": "https://github.com/org/repo"}), "org/repo"
        )

    def test_gitlab_nested_groups(self):
        self.assertEqual(
            dry_run._codecov_repo_path({"URL": "https://gitlab.com/group/subgroup/project"}),
            "group/subgroup/project",
        )

    def test_git_suffix_is_stripped(self):
        self.assertEqual(
            dry_run._codecov_repo_path({"URL": "https://github.com/org/repo.git"}), "org/repo"
        )

    def test_fallback_to_org_and_repo_when_no_url(self):
        row = {"URL": "", "_org": "myorg", "Repository": "myrepo"}
        self.assertEqual(dry_run._codecov_repo_path(row), "myorg/myrepo")


class TestCodecovUrl(unittest.TestCase):
    def test_default_instance_github(self):
        row = {"URL": "https://github.com/org/repo"}
        self.assertEqual(dry_run._codecov_url(row), "https://app.codecov.io/gh/org/repo")

    def test_default_instance_gitlab(self):
        row = {"URL": "https://gitlab.com/group/repo"}
        self.assertEqual(dry_run._codecov_url(row), "https://app.codecov.io/gl/group/repo")

    def test_custom_codecov_base_url(self):
        row = {"URL": "https://github.com/org/repo", "_codecov_base_url": "https://codecov.example.com"}
        self.assertEqual(dry_run._codecov_url(row), "https://codecov.example.com/gh/org/repo")

    def test_org_segment_is_present(self):
        # Regression guard for the PR #95 bug where the org/owner segment was
        # dropped, producing 404 Codecov links. The org must survive into the
        # final URL.
        row = {"URL": "https://github.com/acme-org/widget"}
        self.assertIn("acme-org", dry_run._codecov_url(row))
        self.assertEqual(dry_run._codecov_url(row), "https://app.codecov.io/gh/acme-org/widget")


class TestRepoUrl(unittest.TestCase):
    def test_csv_url_takes_precedence(self):
        row = {"URL": "https://gitlab.com/group/repo", "Repository": "repo"}
        self.assertEqual(dry_run._repo_url(row, "org"), "https://gitlab.com/group/repo")

    def test_fallback_constructs_github_url(self):
        row = {"URL": "", "Repository": "repo"}
        self.assertEqual(dry_run._repo_url(row, "myorg"), "https://github.com/myorg/repo")

    def test_gitlab_fallback_warning_is_currently_unreachable(self):
        # Documents the dead-code bug tracked in issue #111: the GitLab warning
        # branch in _repo_url is guarded by _codecov_provider(row) == "gl", but
        # provider detection only inspects the URL column — which is guaranteed
        # empty at the point the warning would fire. So a GitLab-CI repo with no
        # URL silently falls back to a GitHub link and NO warning is printed.
        row = {"URL": "", "Repository": "repo", "CI System": "GitLab CI"}
        buf = io.StringIO()
        with redirect_stdout(buf):
            result = dry_run._repo_url(row, "myorg")
        self.assertEqual(result, "https://github.com/myorg/repo")
        self.assertEqual(buf.getvalue(), "", "warning unexpectedly fired; update this test if #111 is fixed")


if __name__ == "__main__":
    unittest.main()
