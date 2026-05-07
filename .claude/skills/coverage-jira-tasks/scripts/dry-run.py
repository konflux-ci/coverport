#!/usr/bin/env python3
"""Dry-run: read audit CSV, classify repos, generate Jira task files."""

import argparse
import csv
import os
import re
import sys


def has_ci(row):
    ci = row.get("CI System", "").strip().lower()
    return ci and ci != "none detected"


def has_language(row):
    lang = row.get("Language", "").strip()
    return bool(lang) and lang.lower() not in ("makefile", "dockerfile")


def star_count(row):
    try:
        return int(row.get("Stars", "0").strip())
    except ValueError:
        return 0


def is_false_positive_tests(row):
    """Detect repos where Has Unit Tests=yes but tests are actually placeholder/broken."""
    details = row.get("Test Details", "").strip().lower()
    return 'echo "error: no test specified"' in details or "no test specified" in details


def priority_from_stars(stars, base_priority):
    """Boost priority for high-star repos."""
    if stars >= 1000:
        return "Critical"
    if stars >= 100:
        return max_priority(base_priority, "Major")
    if stars >= 30:
        return max_priority(base_priority, "Normal")
    return base_priority


PRIORITY_ORDER = ["Minor", "Normal", "Major", "Critical"]


def max_priority(a, b):
    return PRIORITY_ORDER[max(PRIORITY_ORDER.index(a), PRIORITY_ORDER.index(b))]


def classify_repo(row):
    """Return list of (task_type, priority) or None to skip."""
    category = row.get("Category", "").strip()
    fork = row.get("Fork", "").strip().lower()
    archived = row.get("Archived", "").strip().lower()
    has_tests = row.get("Has Unit Tests", "").strip().lower()
    has_e2e = row.get("Has E2E Tests", "").strip().lower()
    has_codecov = row.get("Has Codecov", "").strip().lower()
    codecov_details = row.get("Codecov Details", "").strip().lower()
    stars = star_count(row)

    if category != "application":
        return None, "skip-non-app"
    if fork == "yes":
        return None, "skip-fork"
    if archived == "yes":
        return None, "skip-archived"

    # Skip repos with no language AND no CI — config/docs repos
    if not has_language(row) and not has_ci(row):
        return None, "skip-no-code"

    tasks = []

    # Codecov already present
    if has_codecov == "yes":
        if "flags" not in codecov_details:
            p = priority_from_stars(stars, "Critical")
            tasks.append(("fix-codecov", p))
        else:
            p = priority_from_stars(stars, "Major")
            tasks.append(("verify-codecov", p))
    elif has_codecov == "config-only":
        p = priority_from_stars(stars, "Critical")
        tasks.append(("fix-codecov", p))

    # Unit tests present but no Codecov
    elif has_tests in ("yes", "likely"):
        if is_false_positive_tests(row):
            p = priority_from_stars(stars, "Minor")
            tasks.append(("needs-tests", p))
        elif not has_ci(row):
            p = priority_from_stars(stars, "Normal")
            tasks.append(("needs-ci", p))
        else:
            p = priority_from_stars(stars, "Normal")
            tasks.append(("onboard-unit", p))

    # Unknown tests — differentiate between "has CI" and "has nothing"
    elif has_tests == "unknown":
        if has_ci(row) and has_language(row):
            p = priority_from_stars(stars, "Minor")
            tasks.append(("needs-tests", p))
        elif has_language(row):
            p = priority_from_stars(stars, "Minor")
            tasks.append(("investigate", p))
        # else: no language, has CI but no language → skip (caught above mostly)

    # E2E tests present
    if has_e2e == "yes":
        lang = row.get("Language", "").strip()
        if lang in ("Go", "Python", "TypeScript", "JavaScript"):
            p = priority_from_stars(stars, "Minor")
            tasks.append(("onboard-e2e", p))

    if tasks:
        return tasks, None
    return None, "skip-no-action"


TASK_TYPE_LABELS = {
    "fix-codecov": "Fix Codecov configuration",
    "verify-codecov": "Verify Codecov setup",
    "onboard-unit": "Onboard unit test coverage to Codecov",
    "investigate": "Investigate tests and onboard to Codecov",
    "needs-tests": "Add tests then onboard to Codecov",
    "needs-ci": "Set up CI then onboard to Codecov",
    "onboard-e2e": "Onboard e2e test coverage via Coverport",
}

TASK_TYPE_JIRA_LABELS = {
    "fix-codecov": "fix-codecov",
    "verify-codecov": "verify-codecov",
    "onboard-unit": "unit-tests",
    "investigate": "investigate",
    "needs-tests": "needs-tests",
    "needs-ci": "needs-ci",
    "onboard-e2e": "e2e-tests",
}

TASK_TYPE_SUMMARIES = {
    "fix-codecov": "Fix Codecov configuration",
    "verify-codecov": "Verify Codecov setup",
    "onboard-unit": "Onboard unit test coverage to Codecov",
    "investigate": "Investigate and onboard to Codecov",
    "needs-tests": "Add unit tests and onboard to Codecov",
    "needs-ci": "Set up CI pipeline and onboard to Codecov",
    "onboard-e2e": "Onboard e2e test coverage via Coverport",
}


def generate_steps(row, task_type):
    lang = row.get("Language", "").strip()
    ci = row.get("CI System", "").strip()
    repo = row.get("Repository", "").strip()
    org = row.get("_org", "")
    codecov_url = f"https://app.codecov.io/gh/{org}/{repo}"

    if task_type == "fix-codecov":
        return f"""1. Review current Codecov configuration in CI workflows
2. Ensure coverage upload uses `flags: unit-tests`
3. Ensure workflow runs on push to `main` (not just PRs)
4. Verify `CODECOV_TOKEN` secret is set (or OIDC for public repos)
5. Enable flag analytics in [{codecov_url}]({codecov_url}) → Flags tab → click "Enable flag analytics"
"""

    if task_type == "verify-codecov":
        return f"""1. Check [{codecov_url}]({codecov_url}) shows coverage data on main page
2. Verify `unit-tests` flag appears under Flags tab
3. Confirm coverage percentage is non-zero
4. If anything missing, follow fix steps above
"""

    if task_type == "onboard-e2e":
        return f"""1. Follow the [E2E Code Coverage Guide](https://konflux.pages.redhat.com/docs/users/testing/e2e-code-coverage.html)
2. Add Coverport instrumentation to the application
3. Modify Dockerfile for instrumented builds
4. Update CI pipeline for coverage collection
5. Upload to Codecov with `e2e-tests` flag
"""

    if task_type == "investigate":
        return f"""1. Check if repository has unit tests (look for `_test.go`, `*_test.py`, `*.test.ts`, `*.spec.ts` files)
2. Check if a CI pipeline exists but was not detected by the audit
3. If tests exist, follow onboarding steps for the repo's language
4. If no tests exist but the repo has meaningful code, consider adding basic unit tests
5. If the repo is config-only or has no testable code, close this task as won't-do
"""

    if task_type == "needs-tests":
        lang_hint = ""
        if lang == "Go":
            lang_hint = "\n   - Go: create `*_test.go` files, use `go test ./...`"
        elif lang == "Python":
            lang_hint = "\n   - Python: create `tests/` directory, use `pytest`"
        elif lang in ("TypeScript", "JavaScript"):
            lang_hint = "\n   - JS/TS: add `jest` or `vitest`, create `*.test.ts` files"
        return f"""1. Identify key packages/modules that should have test coverage
2. Add unit test framework and initial test files:{lang_hint}
3. Add coverage generation to CI pipeline
4. Add Codecov upload step with `flags: unit-tests`
5. Enable flag analytics in [{codecov_url}]({codecov_url}) → Flags tab
"""

    if task_type == "needs-ci":
        return f"""1. Set up CI pipeline (GitHub Actions recommended):
   - Create `.github/workflows/ci.yml`
   - Add test step with coverage generation
2. Add `CODECOV_TOKEN` secret to repo
3. Add Codecov upload step with `flags: unit-tests`
4. Enable flag analytics in [{codecov_url}]({codecov_url}) → Flags tab
"""

    # onboard-unit — language-specific
    if "GitHub Actions" in ci:
        if lang == "Go":
            return f"""1. Add `CODECOV_TOKEN` secret to repo (or use OIDC for public repos)
2. Update test step to generate coverage:
   ```
   go test ./... -coverprofile=coverage.out -covermode=atomic
   ```
3. Add Codecov upload step:
   ```yaml
   - uses: codecov/codecov-action@v5
     with:
       token: ${{{{ secrets.CODECOV_TOKEN }}}}
       flags: unit-tests
       files: ./coverage.out
       fail_ci_if_error: false
   ```
4. Ensure workflow runs on push to `main`
5. Enable flag analytics in [{codecov_url}]({codecov_url}) → Flags tab
"""
        elif lang == "Python":
            return f"""1. Add `CODECOV_TOKEN` secret to repo
2. Update test step to generate coverage:
   ```
   pip install coverage
   coverage run -m pytest
   coverage xml -o coverage.xml
   ```
3. Add Codecov upload step:
   ```yaml
   - uses: codecov/codecov-action@v5
     with:
       token: ${{{{ secrets.CODECOV_TOKEN }}}}
       flags: unit-tests
       files: ./coverage.xml
       fail_ci_if_error: false
   ```
4. Ensure workflow runs on push to `main`
5. Enable flag analytics in [{codecov_url}]({codecov_url}) → Flags tab
"""
        elif lang in ("TypeScript", "JavaScript"):
            return f"""1. Add `CODECOV_TOKEN` secret to repo
2. Update test step to generate coverage:
   ```
   npx jest --coverage   # or: npx vitest --coverage
   ```
3. Add Codecov upload step:
   ```yaml
   - uses: codecov/codecov-action@v5
     with:
       token: ${{{{ secrets.CODECOV_TOKEN }}}}
       flags: unit-tests
       files: ./coverage/lcov.info
       fail_ci_if_error: false
   ```
4. Ensure workflow runs on push to `main`
5. Enable flag analytics in [{codecov_url}]({codecov_url}) → Flags tab
"""
        else:
            return f"""1. Add `CODECOV_TOKEN` secret to repo
2. Add coverage generation to test step (language-specific)
3. Add Codecov upload step with `flags: unit-tests`
4. Ensure workflow runs on push to `main`
5. Enable flag analytics in [{codecov_url}]({codecov_url}) → Flags tab
"""
    elif "GitLab CI" in ci:
        return f"""1. Add `CODECOV_TOKEN` CI variable in GitLab
2. Add coverage generation to test job
3. Add upload step using Codecov CLI:
   ```
   curl -Os https://cli.codecov.io/latest/linux/codecov
   chmod +x codecov
   ./codecov upload-process --token $CODECOV_TOKEN --flag unit-tests --file <coverage-file>
   ```
4. Ensure pipeline runs on push to `main`
5. Enable flag analytics in [{codecov_url}]({codecov_url}) → Flags tab
"""
    else:
        return f"""1. Add `CODECOV_TOKEN` to CI secrets
2. Add coverage generation to test step
3. Upload to Codecov with `--flag unit-tests`
4. Ensure tests run on push to `main`
5. Enable flag analytics in [{codecov_url}]({codecov_url}) → Flags tab
"""


def generate_task_file(row, task_type, priority, org):
    repo = row.get("Repository", "").strip()
    lang = row.get("Language", "") or "unknown"
    has_tests = row.get("Has Unit Tests", "")
    has_e2e = row.get("Has E2E Tests", "")
    has_codecov = row.get("Has Codecov", "")
    ci = row.get("CI System", "") or "unknown"
    description = row.get("Description", "").strip()
    stars = star_count(row)
    contributors = []
    for i in range(1, 4):
        c = row.get(f"Contributor {i}", "").strip()
        if c:
            contributors.append(c)
    test_details = row.get("Test Details", "").strip()

    summary_label = TASK_TYPE_SUMMARIES.get(task_type, task_type)
    summary = f"{repo}: {summary_label}"
    type_label = TASK_TYPE_JIRA_LABELS.get(task_type, task_type)
    labels = f"codecov-onboarding, {type_label}"

    row["_org"] = org
    steps = generate_steps(row, task_type)

    codecov_status = has_codecov
    if has_codecov == "yes":
        codecov_status = f"yes ({row.get('Codecov Details', '')})"

    desc_line = f"\n> {description}\n" if description else ""
    stars_line = f" ({stars} ⭐)" if stars > 0 else ""
    contributors_line = ""
    if contributors:
        contributors_line = f"\n**Key contacts:** {', '.join(contributors)}\n"
    test_details_line = ""
    if test_details:
        test_details_line = f"| Test details | {test_details} |\n"

    content = f"""---
summary: "{summary}"
priority: "{priority}"
type: "Task"
labels: "{labels}"
---

### Objective

{summary_label} for [{repo}](https://github.com/{org}/{repo}){stars_line}.
{desc_line}
### Current State

| Item | Status |
|------|--------|
| Unit tests | {has_tests} |
| E2E tests | {has_e2e} |
| Codecov | {codecov_status} |
| CI System | {ci} |
| Language | {lang} |
{test_details_line}
{contributors_line}
### Steps

{steps}

### AI-Assisted Implementation

Most code changes can be automated using the **codecov-onboarding** AI skill:

**Quick Start:** https://github.com/konflux-ci/coverport/blob/main/.claude/skills/codecov-onboarding/README.md

**Manual steps required:**
- Get upload token from Codecov settings
- Add `CODECOV_TOKEN` as CI secret
- Enable flag analytics in Codecov UI

### Verification

- [ ] CI workflow uploads coverage successfully
- [ ] Codecov dashboard shows coverage data
- [ ] Coverage flag visible in Flags tab
- [ ] Coverage percentage is non-zero
"""
    return summary, content


def slugify(text):
    return re.sub(r"[^a-z0-9]+", "-", text.lower()).strip("-")


def main():
    parser = argparse.ArgumentParser(description="Dry-run: generate Jira task files from audit CSV")
    parser.add_argument("--csv", required=True, help="Path to audit CSV file")
    parser.add_argument("--org", required=True, help="GitHub/GitLab org name")
    parser.add_argument("--output-dir", default="./jira-tasks-draft/", help="Output directory for task files")
    parser.add_argument("--types", default=None,
                        help="Comma-separated task types to include (e.g., 'onboard-unit,fix-codecov'). Default: all")
    parser.add_argument("--repos", default=None,
                        help="Comma-separated repo names to include (e.g., 'quay-operator,mirror-registry'). Default: all")
    args = parser.parse_args()

    allowed_types = None
    if args.types:
        allowed_types = set(t.strip() for t in args.types.split(","))
        valid_types = set(TASK_TYPE_LABELS.keys())
        invalid = allowed_types - valid_types
        if invalid:
            print(f"ERROR: Unknown task types: {', '.join(invalid)}")
            print(f"Valid types: {', '.join(sorted(valid_types))}")
            sys.exit(1)

    if not os.path.exists(args.csv):
        print(f"ERROR: CSV file not found: {args.csv}")
        sys.exit(1)

    os.makedirs(args.output_dir, exist_ok=True)

    with open(args.csv, newline="") as f:
        reader = csv.DictReader(f)
        rows = list(reader)

    has_onboard_col = any("Onboard" in (r.keys() if hasattr(r, 'keys') else []) for r in rows[:1])
    if has_onboard_col and not args.repos:
        before = len(rows)
        rows = [r for r in rows if r.get("Onboard", "").strip().upper() == "TRUE"]
        print(f"Read {before} repos from {args.csv}, filtered to {len(rows)} by Onboard=TRUE column\n")
        if len(rows) == 0:
            print("WARNING: No repos have Onboard=TRUE. Check the CSV or use --repos to override.\n")

    allowed_repos = None
    if args.repos:
        allowed_repos = set(r.strip() for r in args.repos.split(","))

    if allowed_repos:
        before = len(rows)
        rows = [r for r in rows if r.get("Repository", "").strip() in allowed_repos]
        print(f"Read {before} repos from {args.csv}, filtered to {len(rows)} by --repos\n")
        missing = allowed_repos - {r.get("Repository", "").strip() for r in rows}
        if missing:
            print(f"WARNING: repos not found in CSV: {', '.join(sorted(missing))}\n")
    elif not has_onboard_col:
        print(f"Read {len(rows)} repos from {args.csv}\n")

    stats = {"total": 0, "tasks": {}}
    skip_stats = {}
    task_files = []
    wave_buckets = {"critical": [], "high": [], "medium": [], "low": []}

    for row in rows:
        result, skip_reason = classify_repo(row)
        if not result:
            skip_stats[skip_reason] = skip_stats.get(skip_reason, 0) + 1
            continue

        for task_type, priority in result:
            if allowed_types and task_type not in allowed_types:
                continue

            summary, content = generate_task_file(row, task_type, priority, args.org)
            repo = row.get("Repository", "").strip()
            filename = f"{slugify(repo)}-{task_type}.md"
            filepath = os.path.join(args.output_dir, filename)

            with open(filepath, "w") as f:
                f.write(content)

            task_files.append((filename, summary, priority, task_type))
            stats["total"] += 1
            stats["tasks"][task_type] = stats["tasks"].get(task_type, 0) + 1

            # Bucket for wave summary
            if priority == "Critical":
                wave_buckets["critical"].append(summary)
            elif priority == "Major":
                wave_buckets["high"].append(summary)
            elif priority == "Normal":
                wave_buckets["medium"].append(summary)
            else:
                wave_buckets["low"].append(summary)

    total_skipped = sum(skip_stats.values())
    print(f"Generated {stats['total']} task files in {args.output_dir}")
    print(f"Skipped {total_skipped} repos:")
    for reason, count in sorted(skip_stats.items()):
        print(f"  {reason}: {count}")

    print(f"\nTask breakdown:")
    for tt, count in sorted(stats["tasks"].items()):
        label = TASK_TYPE_LABELS.get(tt, tt)
        print(f"  {label}: {count}")

    if allowed_types:
        print(f"\n(Filtered to types: {', '.join(sorted(allowed_types))})")

    print(f"\n{'='*60}")
    print("Recommended execution waves:")
    for wave_name, items in [("Wave 1 - Critical", wave_buckets["critical"]),
                              ("Wave 2 - High priority", wave_buckets["high"]),
                              ("Wave 3 - Medium priority", wave_buckets["medium"]),
                              ("Wave 4 - Low priority", wave_buckets["low"])]:
        if items:
            print(f"\n  {wave_name} ({len(items)} tasks):")
            for s in items:
                print(f"    - {s}")

    print(f"\n{'='*60}")
    print(f"Files created:")
    for filename, summary, priority, _ in task_files:
        print(f"  [{priority}] {filename}")
        print(f"         {summary}")

    print(f"\nReview files in {args.output_dir}/, then run create-tasks.py to push to Jira.")


if __name__ == "__main__":
    main()
