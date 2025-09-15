---
Title: Integrating Your Custom Reviewer with the Go Agent Action
Slug: build-custom-agent-action
Short: How to wire your own review tool into the published Go agent action and deploy it in GitHub workflows.
Topics:
- github-actions
- code-review
- go
IsTemplate: false
IsTopLevel: false
ShowPerDefault: true
SectionType: GeneralTopic
---

# Integrating Your Custom Reviewer with the Go Agent Action

This guide focuses on using the published `go-go-golems/go-go-agent-action` without modifying its source. You will learn how to build a reviewer binary or service, pass it to the action via `tool_mode`, and run the action from a GitHub workflow that supports both pull-request events and `@agent` mentions.

## Prerequisites

- Go 1.22+ (or another language for your reviewer) if you plan to build a CLI.
- Familiarity with JSON input/output and simple GitHub workflow edits.
- Optional: Docker if you want to rehearse the workflow locally with `act`.

## Step 1 – Understand the payload and response contracts

The action sends a `PRContext` JSON describing the pull request and expects a `ReviewResult` JSON in return. Knowing the fields lets you extract exactly the data your reviewer needs.

### PRContext structure

```json
{
  "owner": "go-go-golems",
  "repo": "go-go-labs",
  "number": 58,
  "title": "Update test review program",
  "body": "Sample change",
  "base_ref": "main",
  "head_ref": "chore/random-review-test",
  "head_sha": "abc123",
  "user_login": "octocat",
  "labels": ["backend", "urgent"],
  "assignees": ["octocat", "hubot"],
  "changed_files": [
    {
      "path": "cmd/app/main.go",
      "status": "modified",
      "patch": "@@ -10,6 +10,8 @@",
      "additions": 2,
      "deletions": 0,
      "blob_url": "https://github.com/...",
      "raw_url": "https://raw.githubusercontent.com/...",
      "contents_b64": "..."   // set when include_file_contents=true
    }
  ],
  "guidelines_b64": "...",    // optional CLAUDE.md or similar
  "extra_files": [
    { "path": "docs/adr/001.md", "contents_b64": "..." }
  ],
  "triggered_by": "octocat",
  "event_name": "issue_comment",
  "trigger_text": "@agent please run",
  "run_id": "123456789"
}
```

Important points:

- `changed_files` contains diff metadata and, optionally, file contents if your workflow sets `include_file_contents: true`.
- `guidelines_b64` and `extra_files` let you embed additional references (e.g. style guides) in the request.
- `trigger_text` helps reviewers react differently to mentions vs automatic runs.

### ReviewResult structure

```json
{
  "summary_markdown": "### Review summary\n- 2 files checked",
  "review_decision": "comment",
  "review_body": "Automated review report",
  "issue_comment": "Summary-only response for mention runs",
  "comments": [
    {
      "path": "cmd/app/main.go",
      "body": "Consider extracting this logic into a helper.",
      "line": 42,
      "side": "RIGHT"
    },
    {
      "path": "cmd/app/main.go",
      "body": "Suggestion for lines 30-35.",
      "start_line": 30,
      "line": 35,
      "side": "RIGHT",
      "start_side": "RIGHT"
    }
  ]
}
```

- `summary_markdown` appears both in the job summary and (optionally) stdout.
- `review_decision` can be `approve`, `request_changes`, or `comment`. The action batches all inline comments into a single review with that state.
- `issue_comment` posts to the PR timeline. This is useful when inline comments are not feasible (e.g., mention-triggered runs without diff context).
- `comments` lets you target specific files and lines. To add multiple inline notes, simply append more entries to this array. Multi-line comments use `start_line`/`start_side` alongside `line`/`side`.

## Step 2 – Implement your reviewer

Choose whether your reviewer runs as an HTTP service or a CLI binary. Both options receive the same `PRContext` JSON.

### HTTP reviewer

Expose a POST endpoint and configure the workflow accordingly:

```yaml
with:
  tool_mode: http
  tool_url: https://reviewer.internal/api/review
  tool_headers_json: '{"X-Auth":"secret"}'
  include_patch: true
  output_mode: review+summary
  github_token: ${{ secrets.GITHUB_TOKEN }}
```

The action sends the JSON payload to `tool_url` and expects a `ReviewResult` response. Use this mode when your reviewer runs outside GitHub (e.g., Kubernetes service).

### CLI reviewer (recommended for repository-local logic)

Keep the reviewer next to your code so it evolves with the repository. The binary reads stdin and writes the `ReviewResult`. Here’s an example that emits two inline comments:

```go
package main

import (
    "encoding/json"
    "fmt"
    "os"
)

type Context struct {
    Number       int           `json:"number"`
    ChangedFiles []struct {
        Path string `json:"path"`
    } `json:"changed_files"`
}

type Result struct {
    SummaryMarkdown string   `json:"summary_markdown"`
    ReviewDecision  string   `json:"review_decision"`
    ReviewBody      string   `json:"review_body"`
    Comments        []Comment `json:"comments"`
}

type Comment struct {
    Path string `json:"path"`
    Body string `json:"body"`
    Line int    `json:"line"`
    Side string `json:"side"`
}

func main() {
    var ctx Context
    if err := json.NewDecoder(os.Stdin).Decode(&ctx); err != nil {
        panic(err)
    }

    comments := []Comment{}
    if len(ctx.ChangedFiles) > 0 {
        comments = append(comments,
            Comment{Path: ctx.ChangedFiles[0].Path, Body: "Run go test ./...", Line: 1, Side: "RIGHT"},
            Comment{Path: ctx.ChangedFiles[0].Path, Body: "Consider extracting this block.", Line: 10, Side: "RIGHT"},
        )
    }

    res := Result{
        SummaryMarkdown: fmt.Sprintf("### Review for PR #%d\n- %d comments", ctx.Number, len(comments)),
        ReviewDecision:  "comment",
        ReviewBody:      "Automated review feedback",
        Comments:        comments,
    }

    if err := json.NewEncoder(os.Stdout).Encode(res); err != nil {
        panic(err)
    }
}
```

Your reviewer can add as many inline comments as necessary by populating the `Comments` slice.

## Step 3 – Create the workflow glue

Add a workflow that builds the reviewer (if using CLI) and invokes the action. The merged workflow pattern below supports both PR events and mentions.

```yaml
name: go-agent-review

on:
  pull_request:
    types: [opened, synchronize, reopened]
  issue_comment:
    types: [created]

permissions:
  contents: read
  pull-requests: write

jobs:
  review:
    if: |
      github.event_name == 'pull_request' ||
      (github.event_name == 'issue_comment' &&
       github.event.issue.pull_request &&
       contains(github.event.comment.body, '@agent'))
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          persist-credentials: false

      - name: Build reviewer
        run: |
          mkdir -p .tooling
          go build -o .tooling/custom-reviewer ./cmd/reviewers/custom

      - name: Automated review
        uses: go-go-golems/go-go-agent-action@v1.0.0
        with:
          tool_mode: cmd
          tool_cmd: ./.tooling/custom-reviewer
          include_patch: true
          output_mode: review+summary
          github_token: ${{ secrets.GITHUB_TOKEN }}
```

For HTTP reviewers, replace the build step with network configuration and set `tool_mode: http`.

## Step 4 – Add configuration and secrets

Keep configuration close to your reviewer:

- Commit `reviewer.yaml`, load it from the workspace path, and adjust logic accordingly.
- Use workflow inputs or `env:` to pass flags (e.g., `REVIEW_COMMANDS`).
- Store secrets (API keys) in repository secrets and expose them to your reviewer via environment variables.

## Step 5 – Test locally (optional)

Use `go test` (or your language’s tests) and `act` to simulate both triggers:

```bash
GOCACHE=$(pwd)/.cache go test ./cmd/reviewers/custom
~/go/bin/act -W .github/workflows/go-agent-review.yml -P ubuntu-latest=ghcr.io/catthehacker/ubuntu:act-latest
```

Provide event fixtures for pull requests and issue comments to exercise both branches.

## Step 6 – Commit and merge

- Commit the reviewer source, configuration, and workflow.
- Open a pull request; once merged to `main`, GitHub picks up the workflow changes.
- Developers now get reviews automatically on PR updates and can request one with `@agent` at any time.

## Step 7 – Reuse across repositories (optional)

Package the reviewer separately if multiple repos need it:

- Build a Go module and `go install` it inside the workflow.
- Ship a Docker image and run it via `tool_mode=cmd`.
- Publish an npm/pip package and `npx`/`pipx run` it before calling the action.

## Troubleshooting

- **401 errors** – Ensure `github_token` (or a PAT) is passed. The action can’t post reviews without it.
- **422 diff errors** – For mention-triggered runs, avoid inline comments unless your reviewer knows the diff. Use `IssueComment` or `SummaryMarkdown` instead.
- **Reviewer crashes** – The action surfaces stdout/stderr in the job log. Add logging in your reviewer to diagnose command failures.

By keeping reviewer logic in your repository or service and reusing the action, you get a flexible review workflow without maintaining a fork of the action itself.
