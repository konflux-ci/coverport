#!/usr/bin/env python3
"""Create Jira tasks from generated task files and link to epic."""

import argparse
import base64
import glob
import json
import os
import re
import ssl
import sys
import time
import urllib.error
import urllib.request

DEFAULT_JIRA_URL = "https://redhat.atlassian.net"


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
        # Code block fences
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

        # Headers: ### → h3.
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
            # Peek ahead: if next non-blank line is a separator, this is a header row
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

        # Unordered lists: - item → * item
        line = re.sub(r"^(\s*)- ", r"\1* ", line)

        # Bold: **text** → *text*
        line = re.sub(r"\*\*(.+?)\*\*", r"*\1*", line)

        # Inline code: `text` → {{text}}
        line = re.sub(r"`([^`]+)`", r"{{\1}}", line)

        # Links: [text](url) → [text|url]
        line = re.sub(r"\[([^\]]+)\]\(([^)]+)\)", r"[\1|\2]", line)

        result.append(line)

    return "\n".join(result)


def _ssl_context():
    try:
        import certifi
        return ssl.create_default_context(cafile=certifi.where())
    except ImportError:
        return ssl.create_default_context()


def create_issue(auth, project, summary, description, priority, parent_key, jira_url, labels=None):
    ctx = _ssl_context()

    payload = {
        "fields": {
            "project": {"key": project},
            "summary": summary,
            "description": description,
            "issuetype": {"name": "Task"},
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

    try:
        with urllib.request.urlopen(req, context=ctx) as resp:
            result = json.loads(resp.read().decode())
            return result.get("key"), None
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8", errors="replace")
        return None, f"{e.code}: {body[:300]}"


def main():
    parser = argparse.ArgumentParser(description="Create Jira tasks from generated task files")
    parser.add_argument("--input-dir", required=True, help="Directory with task .md files")
    parser.add_argument("--epic", required=True, help="Epic key to link tasks to (e.g., COVERPORT-11)")
    parser.add_argument("--project", default=None, help="Jira project key (default: derived from epic)")
    parser.add_argument("--jira-url", default=None, help="Jira base URL (default: JIRA_URL env or https://redhat.atlassian.net)")
    args = parser.parse_args()

    project = args.project or args.epic.rsplit("-", 1)[0]
    jira_url = args.jira_url or get_jira_url()
    auth = get_auth()

    task_files = sorted(glob.glob(os.path.join(args.input_dir, "*.md")))
    if not task_files:
        print(f"No .md files found in {args.input_dir}")
        sys.exit(1)

    print(f"Creating {len(task_files)} tasks in {project}, linked to {args.epic}")
    print(f"Jira URL: {jira_url}\n")

    created = []
    failed = []

    for i, filepath in enumerate(task_files, 1):
        filename = os.path.basename(filepath)
        frontmatter, body = parse_task_file(filepath)

        summary = frontmatter.get("summary", filename.replace(".md", ""))
        priority = frontmatter.get("priority", "Normal")
        labels_str = frontmatter.get("labels", "codecov-onboarding")
        labels = [l.strip() for l in labels_str.split(",") if l.strip()]

        print(f"[{i}/{len(task_files)}] {summary}")

        jira_body = markdown_to_jira(body)
        key, error = create_issue(auth, project, summary, jira_body, priority, args.epic, jira_url, labels)
        if key:
            print(f"  → {key} ({jira_url}/browse/{key})")
            created.append((key, summary))
        else:
            print(f"  FAILED: {error}")
            failed.append((filename, error))

        time.sleep(0.5)

    print(f"\n{'='*60}")
    print(f"Created: {len(created)}/{len(task_files)}")

    if created:
        print(f"\nCreated issues:")
        for key, summary in created:
            print(f"  {key}: {summary}")

    if failed:
        print(f"\nFailed:")
        for filename, error in failed:
            print(f"  {filename}: {error}")


if __name__ == "__main__":
    main()
