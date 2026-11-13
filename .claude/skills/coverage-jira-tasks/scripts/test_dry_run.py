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

    # --- CI System column fallback (issue #178 / PR #144) ---
    # When the URL column is empty, _codecov_provider also inspects the
    # CI System column, so GitLab rows without a URL still route to 'gl'.

    def test_ci_system_gitlab_with_empty_url_returns_gl(self):
        """CI System fallback: empty URL + GitLab CI -> gl (PR #144 path)."""
        self.assertEqual(
            dry_run._codecov_provider({"URL": "", "CI System": "GitLab CI"}), "gl"
        )

    def test_ci_system_github_actions_with_empty_url_returns_gh(self):
        self.assertEqual(
            dry_run._codecov_provider({"URL": "", "CI System": "GitHub Actions"}), "gh"
        )

    def test_missing_ci_system_column_with_empty_url_returns_gh(self):
        self.assertEqual(dry_run._codecov_provider({"URL": ""}), "gh")
        self.assertEqual(dry_run._codecov_provider({}), "gh")

    def test_populated_gitlab_url_returns_gl(self):
        self.assertEqual(
            dry_run._codecov_provider({"URL": "https://gitlab.com/group/repo"}), "gl"
        )

    def test_populated_non_gitlab_url_returns_gh(self):
        self.assertEqual(
            dry_run._codecov_provider({"URL": "https://github.com/org/repo"}), "gh"
        )

    def test_url_takes_precedence_over_ci_system(self):
        """A gitlab URL returns 'gl' regardless of CI System.

        The CI System check only runs when the URL does not already identify
        GitLab; a non-gitlab URL with a GitLab CI System still falls back to
        'gl' (that fallback is the whole point of PR #144).
        """
        self.assertEqual(
            dry_run._codecov_provider(
                {"URL": "https://gitlab.com/group/repo", "CI System": "GitHub Actions"}
            ),
            "gl",
        )
        self.assertEqual(
            dry_run._codecov_provider(
                {"URL": "https://github.com/org/repo", "CI System": "GitLab CI"}
            ),
            "gl",
        )
        self.assertEqual(
            dry_run._codecov_provider(
                {"URL": "https://github.com/org/repo", "CI System": "GitHub Actions"}
            ),
            "gh",
        )


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

    def test_gitlab_ci_system_fallback_triggers_warning(self):
        # PR #144 fixed the dead-code bug from issue #111: _codecov_provider()
        # now also checks the CI System column, so the GitLab fallback warning
        # in _repo_url correctly fires for GitLab CI rows with empty URLs.
        row = {"URL": "", "Repository": "repo", "CI System": "GitLab CI"}
        buf = io.StringIO()
        with redirect_stdout(buf):
            result = dry_run._repo_url(row, "myorg")
        # Still falls back to GitHub URL (no GitLab URL to construct without a URL column)
        self.assertEqual(result, "https://github.com/myorg/repo")
        # But now the warning fires to alert the user
        self.assertIn("WARNING", buf.getvalue())
        self.assertIn("falling back to GitHub URL", buf.getvalue())


if __name__ == "__main__":
    unittest.main()
