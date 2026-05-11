#!/usr/bin/env python3
"""Create Jira tasks from generated task files and link to epic.

Expects the output structure from dry-run.py:
  <input-dir>/
    <repo-name>/
      task.md              (parent task → created as Task under epic)
      subtask-<type>.md    (subtasks → created as Subtask under parent)
    _devlake-setup.md      (DevLake task → created as Task under epic)
    _devlake-dashboard.md  (DevLake task → created as Task under epic)
"""

import argparse
import base64
import json
import os
import re
import ssl
import sys
import time
import urllib.error
import urllib.request

DEFAULT_JIRA_URL = "https://redhat.atlassian.net"
CREATED_LOG = ".created-issues.json"


def get_jira_url():
    return os.environ.get("JIRA_URL", DEFAULT_JIRA_URL).rstrip("/")


def get_auth():
    username = os.environ.get("JIRA_USERNAME", "")
    token = os.environ.get("JIRA_API_TOKEN", "")
    if not username or not token:
        print("ERROR: Set JIRA_USERNAME and JIRA_API_TOKEN env vars")
        print("  export JIRA_USERNAME='user@redhat.com'")
        print("  export JIRA_API_TOKEN='your-token'")
        print("  Generate at: https://id.atlassian.com/manage-profile/security/api-tokens")
        sys.exit(1)
    return base64.b64encode(f"{username}:{token}".encode()).decode()


def parse_task_file(filepath):
    """Parse frontmatter and body from task markdown file."""
    with open(filepath) as f:
        content = f.read()

    frontmatter = {}
    body = content

    fm_match = re.match(r"^---\s*\n(.*?)\n---\s*\n", content, re.DOTALL)
    if fm_match:
        for line in fm_match.group(1).strip().split("\n"):
            if ":" in line:
                key, val = line.split(":", 1)
                frontmatter[key.strip()] = val.strip().strip('"').strip("'")
        body = content[fm_match.end():]

    return frontmatter, body.strip()


def markdown_to_jira(md):
    """Convert markdown to Jira wiki markup."""
    lines = md.split("\n")
    result = []
    in_code_block = False
    code_lang = ""

    for line in lines:
        if line.strip().startswith("```"):
            if not in_code_block:
                in_code_block = True
                code_lang = line.strip()[3:].strip()
                lang_attr = f":language={code_lang}" if code_lang else ""
                result.append("{code" + lang_attr + "}")
            else:
                in_code_block = False
                result.append("{code}")
            continue

        if in_code_block:
            result.append(line)
            continue

        # Headers
        m = re.match(r"^(#{1,6})\s+(.*)", line)
        if m:
            level = len(m.group(1))
            result.append(f"h{level}. {m.group(2)}")
            continue

        # Table separator rows — skip
        if re.match(r"^\s*\|[-:\s|]+\|\s*$", line):
            continue

        # Table rows
        if line.strip().startswith("|") and line.strip().endswith("|"):
            cells = [c.strip() for c in line.strip().strip("|").split("|")]
            idx = lines.index(line, lines.index(line))
            is_header = False
            for future in lines[idx+1:]:
                if future.strip() == "":
                    break
                if re.match(r"^\s*\|[-:\s|]+\|\s*$", future):
                    is_header = True
                    break
                break
            if is_header:
                result.append("||" + "||".join(cells) + "||")
            else:
                result.append("|" + "|".join(cells) + "|")
            continue

        # Blockquotes
        if line.startswith("> "):
            result.append("{quote}" + line[2:] + "{quote}")
            continue

        # Checkboxes
        line = re.sub(r"^(\s*)- \[ \] ", r"\1* ", line)
        line = re.sub(r"^(\s*)- \[x\] ", r"\1* (/) ", line)

        # Unordered lists
        line = re.sub(r"^(\s*)- ", r"\1* ", line)

        # Bold
        line = re.sub(r"\*\*(.+?)\*\*", r"*\1*", line)

        # Inline code
        line = re.sub(r"`([^`]+)`", r"{{\1}}", line)

        # Links
        line = re.sub(r"\[([^\]]+)\]\(([^)]+)\)", r"[\1|\2]", line)

        result.append(line)

    return "\n".join(result)


def _ssl_context():
    try:
        import certifi
        return ssl.create_default_context(cafile=certifi.where())
    except ImportError:
        return ssl.create_default_context()


def create_issue(auth, project, summary, description, priority, parent_key, jira_url,
                 labels=None, issue_type="Task"):
    """Create a Jira issue. Returns (key, error)."""
    ctx = _ssl_context()

    payload = {
        "fields": {
            "project": {"key": project},
            "summary": summary,
            "description": description,
            "issuetype": {"name": issue_type},
            "priority": {"name": priority},
        }
    }

    if labels:
        payload["fields"]["labels"] = labels

    if parent_key:
        payload["fields"]["parent"] = {"key": parent_key}

    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        f"{jira_url}/rest/api/2/issue", data=data, method="POST"
    )
    req.add_header("Authorization", f"Basic {auth}")
    req.add_header("Content-Type", "application/json")

    for attempt in range(3):
        try:
            with urllib.request.urlopen(req, context=ctx) as resp:
                result = json.loads(resp.read().decode())
                return result.get("key"), None
        except urllib.error.HTTPError as e:
            body = e.read().decode("utf-8", errors="replace")
            if e.code >= 500 and attempt < 2:
                time.sleep(2 ** attempt)
                continue
            return None, f"{e.code}: {body[:500]}"
    return None, "max retries exceeded"


def detect_subtask_type(auth, project, jira_url):
    """Auto-detect subtask issue type name for the project."""
    ctx = _ssl_context()
    url = f"{jira_url}/rest/api/2/project/{project}/statuses"
    req = urllib.request.Request(url)
    req.add_header("Authorization", f"Basic {auth}")
    req.add_header("Content-Type", "application/json")

    try:
        with urllib.request.urlopen(req, context=ctx) as resp:
            data = json.loads(resp.read().decode())
            for issue_type in data:
                name = issue_type.get("name", "")
                if name.lower().replace("-", "").replace(" ", "") == "subtask":
                    return name
    except Exception:
        pass
    return None


def load_created_log(input_dir):
    """Load log of previously created issues for idempotent retries."""
    log_path = os.path.join(input_dir, CREATED_LOG)
    if os.path.exists(log_path):
        with open(log_path) as f:
            return json.load(f)
    return {}


def save_created_log(input_dir, log):
    """Save log of created issues."""
    log_path = os.path.join(input_dir, CREATED_LOG)
    with open(log_path, "w") as f:
        json.dump(log, f, indent=2)


def main():
    parser = argparse.ArgumentParser(description="Create Jira tasks from generated task files")
    parser.add_argument("--input-dir", required=True, help="Directory with task files (output of dry-run.py)")
    parser.add_argument("--epic", required=True, help="Epic key to link tasks to (e.g., COVERPORT-11)")
    parser.add_argument("--project", default=None, help="Jira project key (default: derived from epic)")
    parser.add_argument("--jira-url", default=None,
                        help="Jira base URL (default: JIRA_URL env or https://redhat.atlassian.net)")
    parser.add_argument("--subtask-type", default=None,
                        help="Issue type name for subtasks (auto-detected if not set). "
                             "Some projects use 'Subtask', others 'Sub-task'.")
    args = parser.parse_args()

    project = args.project or args.epic.rsplit("-", 1)[0]
    jira_url = args.jira_url or get_jira_url()
    auth = get_auth()

    # Auto-detect subtask type if not specified
    subtask_type = args.subtask_type
    if not subtask_type:
        print(f"Auto-detecting subtask issue type for project {project}...")
        subtask_type = detect_subtask_type(auth, project, jira_url)
        if subtask_type:
            print(f"  Found: '{subtask_type}'")
        else:
            subtask_type = "Subtask"
            print(f"  Could not detect, defaulting to '{subtask_type}'")

    # Load created-issues log for idempotent retries
    created_log = load_created_log(args.input_dir)

    # Discover repo directories and root-level DevLake tasks
    repo_dirs = []
    devlake_files = []

    for entry in sorted(os.listdir(args.input_dir)):
        full_path = os.path.join(args.input_dir, entry)
        if os.path.isdir(full_path):
            task_file = os.path.join(full_path, "task.md")
            if os.path.exists(task_file):
                repo_dirs.append(full_path)
        elif entry.startswith("_devlake") and entry.endswith(".md"):
            devlake_files.append(full_path)

    if not repo_dirs and not devlake_files:
        print(f"No task directories or DevLake files found in {args.input_dir}")
        sys.exit(1)

    # Count subtasks
    total_subtasks = 0
    for repo_dir in repo_dirs:
        for f in os.listdir(repo_dir):
            if f.startswith("subtask-") and f.endswith(".md"):
                total_subtasks += 1

    total_issues = len(repo_dirs) + total_subtasks + len(devlake_files)
    print(f"Creating {total_issues} issues in {project}, linked to {args.epic}")
    print(f"  Parent tasks: {len(repo_dirs)}")
    print(f"  Subtasks: {total_subtasks}")
    print(f"  DevLake tasks: {len(devlake_files)}")
    print(f"  Jira URL: {jira_url}\n")

    created = []
    failed = []
    counter = 0

    # Create repo tasks (parent + subtasks)
    for repo_dir in repo_dirs:
        repo_name = os.path.basename(repo_dir)
        task_file = os.path.join(repo_dir, "task.md")
        frontmatter, body = parse_task_file(task_file)

        summary = frontmatter.get("summary", f"{repo_name}: Code coverage onboarding")
        priority = frontmatter.get("priority", "Normal")
        labels_str = frontmatter.get("labels", "codecov-onboarding")
        labels = [l.strip() for l in labels_str.split(",") if l.strip()]

        counter += 1
        log_key = f"task:{repo_name}"

        if log_key in created_log:
            parent_key = created_log[log_key]
            print(f"[{counter}/{total_issues}] TASK: {summary}")
            print(f"  → {parent_key} (already created, skipping)")
            created.append((parent_key, summary))
        else:
            print(f"[{counter}/{total_issues}] TASK: {summary}")

            jira_body = markdown_to_jira(body)
            parent_key, error = create_issue(
                auth, project, summary, jira_body, priority, args.epic, jira_url,
                labels=labels, issue_type="Task"
            )

            if parent_key:
                print(f"  → {parent_key} ({jira_url}/browse/{parent_key})")
                created.append((parent_key, summary))
                created_log[log_key] = parent_key
                save_created_log(args.input_dir, created_log)
            else:
                print(f"  FAILED: {error}")
                failed.append((task_file, error))
                time.sleep(0.5)
                continue  # Skip subtasks if parent failed

        time.sleep(0.5)

        # Create subtasks under parent
        subtask_files = sorted(
            f for f in os.listdir(repo_dir)
            if f.startswith("subtask-") and f.endswith(".md")
        )

        for subtask_file in subtask_files:
            subtask_path = os.path.join(repo_dir, subtask_file)
            sub_log_key = f"subtask:{repo_name}:{subtask_file}"

            if sub_log_key in created_log:
                sub_key = created_log[sub_log_key]
                counter += 1
                fm, _ = parse_task_file(subtask_path)
                sub_summary = fm.get("summary", subtask_file.replace(".md", ""))
                print(f"[{counter}/{total_issues}]   SUBTASK: {sub_summary}")
                print(f"  → {sub_key} (already created, skipping)")
                created.append((sub_key, f"  ↳ {sub_summary}"))
                continue

            fm, sub_body = parse_task_file(subtask_path)

            sub_summary = fm.get("summary", subtask_file.replace(".md", ""))
            sub_priority = fm.get("priority", "Normal")
            sub_labels_str = fm.get("labels", "codecov-onboarding")
            sub_labels = [l.strip() for l in sub_labels_str.split(",") if l.strip()]

            counter += 1
            print(f"[{counter}/{total_issues}]   SUBTASK: {sub_summary}")

            sub_jira_body = markdown_to_jira(sub_body)
            sub_key, sub_error = create_issue(
                auth, project, sub_summary, sub_jira_body, sub_priority, parent_key, jira_url,
                labels=sub_labels, issue_type=subtask_type
            )

            if sub_key:
                print(f"  → {sub_key} ({jira_url}/browse/{sub_key})")
                created.append((sub_key, f"  ↳ {sub_summary}"))
                created_log[sub_log_key] = sub_key
                save_created_log(args.input_dir, created_log)
            else:
                print(f"  FAILED: {sub_error}")
                failed.append((subtask_path, sub_error))

            time.sleep(0.5)

    # Create DevLake follow-up tasks (directly under epic)
    for devlake_file in devlake_files:
        fm, body = parse_task_file(devlake_file)

        summary = fm.get("summary", os.path.basename(devlake_file).replace(".md", ""))
        priority = fm.get("priority", "Normal")
        labels_str = fm.get("labels", "codecov-onboarding, devlake")
        labels = [l.strip() for l in labels_str.split(",") if l.strip()]

        counter += 1
        devlake_log_key = f"devlake:{os.path.basename(devlake_file)}"

        if devlake_log_key in created_log:
            key = created_log[devlake_log_key]
            print(f"[{counter}/{total_issues}] DEVLAKE: {summary}")
            print(f"  → {key} (already created, skipping)")
            created.append((key, summary))
            continue

        print(f"[{counter}/{total_issues}] DEVLAKE: {summary}")

        jira_body = markdown_to_jira(body)
        key, error = create_issue(
            auth, project, summary, jira_body, priority, args.epic, jira_url,
            labels=labels, issue_type="Task"
        )

        if key:
            print(f"  → {key} ({jira_url}/browse/{key})")
            created.append((key, summary))
            created_log[devlake_log_key] = key
            save_created_log(args.input_dir, created_log)
        else:
            print(f"  FAILED: {error}")
            failed.append((devlake_file, error))

        time.sleep(0.5)

    # Summary
    print(f"\n{'='*60}")
    print(f"Created: {len(created)}/{total_issues}")

    if created:
        print(f"\nCreated issues:")
        for key, summary in created:
            print(f"  {key}: {summary}")

    if failed:
        print(f"\nFailed ({len(failed)}):")
        for filepath, error in failed:
            print(f"  {os.path.basename(filepath)}: {error}")


if __name__ == "__main__":
    main()
