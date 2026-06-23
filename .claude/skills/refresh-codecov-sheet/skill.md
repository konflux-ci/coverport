---
name: refresh-codecov-sheet
description: Refresh the Codecov rollout tracking sheet with current PR statuses and review info. Use when asked to "refresh the sheet", "update PR statuses", "check codecov PRs", or "sync the tracker".
---

# Refresh Codecov Rollout Tracker

## Overview

Scans the Codecov rollout tracking Google Sheet for rows with PR URLs, checks each PR's current status and review state via `gh`, and updates columns J (PR Status) and K (Notes) in-place.

## Sheet Reference

- **Sheet ID**: `1Xd_lamxi-JWl8I5ndrVfnmJu2bqw3zBIBop3W5JqBeI`
- **Column I**: PR URL (e.g. `https://github.com/konflux-ci/build-service/pull/633`)
- **Column J**: PR Status — one of: `OPEN`, `MERGED`, `CLOSED`, `DRAFT`
- **Column K**: Notes — review state, CI status, reviewer comments summary

## Step-by-Step

### 1. Read the sheet

```
Read Sheet1!A1:K160 from spreadsheet 1Xd_lamxi-JWl8I5ndrVfnmJu2bqw3zBIBop3W5JqBeI
```

### 2. Find rows with PR URLs

Scan column I (index 8) for non-empty cells that contain `github.com`. Extract the org/repo and PR number from each URL.

### 3. For each PR, fetch status via `gh`

```bash
gh pr view <PR_NUMBER> --repo <ORG/REPO> --json state,reviews,statusCheckRollup,reviewDecision,isDraft,mergedAt,title --jq '{
  state: .state,
  reviewDecision: .reviewDecision,
  reviews: [.reviews[] | {author: .author.login, state: .state}],
  checks: [.statusCheckRollup[] | select(.conclusion != "SUCCESS" and .conclusion != "NEUTRAL" and .conclusion != "") | {name: .name, conclusion: .conclusion}]
}'
```

### 4. Compute status and notes

**Column J — PR Status** (pick one):
- `MERGED` — PR was merged
- `CLOSED` — PR was closed without merging
- `DRAFT` — PR is in draft state
- `OPEN` — PR is open and not draft

**Column K — Notes** (build from these signals, keep concise):
- If merged: `Merged` + merge date if available
- If there are failing checks: `N check(s) failing: <names>`
- Review decision: `Approved`, `Changes requested`, `Review required`, or `No reviews`
- If there are review comments requesting changes: summarize briefly (e.g. `MartinBasti: asked about 80% justification`)
- Preserve any existing manual notes that start with `Batch` (e.g. `Batch 2: preserved flags`) — append status after them with `. `

### 5. Update the sheet

For each row where the status or notes changed, update `Sheet1!J<row>:K<row>` with the new values.

**Batch updates**: Group updates and send them in parallel (up to 10 at a time) to avoid hitting API rate limits.

### 6. Highlight rows needing attention

After updating statuses, apply yellow background highlighting to rows where the PR has **changes requested** or an **open review** (i.e. `reviewDecision` is `CHANGES_REQUESTED` or `REVIEW_REQUIRED`). Use conditional formatting on columns J:K for those rows:

- Background color: yellow (`{red: 1, green: 1, blue: 0}`)
- Apply to each matching row's `J<row>:K<row>` range
- Remove yellow from rows that are now `MERGED`, `CLOSED`, or `APPROVED` (reset to white `{red: 1, green: 1, blue: 1}`)

Use `format_google_sheet_cells` to set the background color on the affected ranges.

### 7. Report summary

Print a summary to the user:
```
Codecov Sheet Refresh — <date>
- Total PRs checked: N
- MERGED: N
- OPEN: N (N approved, N changes requested, N pending review)
- CLOSED: N
- CI failures: N PRs have failing checks
```

## Notes

- Only update rows where column I has a valid GitHub PR URL
- Don't touch rows without PR URLs (repos that haven't been processed yet)
- If `gh pr view` fails (e.g. repo deleted or PR not found), set status to `ERROR` and note the error
- Keep notes under ~100 characters to fit in the sheet column
