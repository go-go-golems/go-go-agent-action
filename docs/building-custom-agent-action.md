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

## Step 1 – Implement your reviewer

The action sends a `PRContext` JSON and expects a `ReviewResult` JSON. You can respond in two ways:

### Option A: HTTP reviewer

Expose a POST endpoint that accepts the context and returns review feedback.

```yaml
# Workflow snippet
with:
  tool_mode: http
  tool_url: https://reviewer.internal/api/review
  tool_token: ${{ secrets.REVIEW_TOKEN }}
  include_patch: true
  output_mode: review+summary
```

Your service can run anywhere, as long as the GitHub runner can reach it.

### Option B: CLI reviewer

Build an executable that reads `PRContext` from stdin and prints a `ReviewResult` JSON to stdout. Place the source in the repository under review, for example `cmd/reviewers/custom/main.go`:

```go
package main

import (
    "encoding/json"
    "os"
)

type prContext struct {
    Number int `json:"number"`
}

type result struct {
    SummaryMarkdown string `json:"summary_markdown"`
    ReviewDecision  string `json:"review_decision"`
    ReviewBody      string `json:"review_body"`
    IssueComment    string `json:"issue_comment"`
}

func main() {
    var ctx prContext
    if err := json.NewDecoder(os.Stdin).Decode(&ctx); err != nil {
        panic(err)
    }

    body := "Automated feedback for PR #" + strconv.Itoa(ctx.Number)
    res := result{
        SummaryMarkdown: "### Custom review\n- PR #" + strconv.Itoa(ctx.Number),
        ReviewDecision:  "comment",
        ReviewBody:      body,
        IssueComment:    body,
    }

    if err := json.NewEncoder(os.Stdout).Encode(res); err != nil {
        panic(err)
    }
}
```

The binary can run on any language/runtime; just honour stdin/stdout JSON.

## Step 2 – Create the workflow glue

Add a workflow that builds your reviewer (if using CLI) and invokes the action. The merged workflow pattern below triggers on both pull-request events and comment mentions.

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

For HTTP reviewers, drop the build step and set `tool_mode: http` with the appropriate URL/token.

## Step 3 – Pass configuration to the reviewer

You control configuration entirely in your repository and workflow:

- Check in YAML/JSON config files and let the reviewer load them from the workspace.
- Use workflow inputs or environment variables to pass options (e.g., `export REVIEW_COMMANDS='go test ./...,golangci-lint run'`).
- Store secrets (API keys, tokens) in repository secrets and expose them via `env:`.

Because the action only forwards context and posts results, the reviewer has full freedom to run tests, call APIs, or aggregate data before returning `ReviewResult`.

## Step 4 – Test locally (optional)

Use `go test` (or your language’s test suite) to check the reviewer, then dry-run the workflow with `act`:

```bash
GOCACHE=$(pwd)/.cache go test ./cmd/reviewers/custom
~/go/bin/act -W .github/workflows/go-agent-review.yml -P ubuntu-latest=ghcr.io/catthehacker/ubuntu:act-latest
```

Provide event fixtures (`.github/events/pull_request.json`, `.github/events/issue_comment.json`) to simulate PR updates and `@agent` comments.

## Step 5 – Commit and merge

- Commit the reviewer source, any config files, and the workflow.
- Open a pull request so the workflow lands on `main`. GitHub only recognises workflow changes from the default branch.
- Once merged, every PR update runs the reviewer automatically; anyone can comment `@agent` to trigger it on demand without additional code changes.

## Step 6 – Reuse across repositories (optional)

If multiple repositories should share the same reviewer logic:

- Package the reviewer as a Go module (or npm package, etc.) and `go install`/`npm install` it inside the workflow.
- Publish the reviewer as a Docker image, build/push once, and `docker run` it via `tool_mode=cmd`.

You still reference `go-go-golems/go-go-agent-action@v1.0.0`; only the reviewer artefact changes.

## Troubleshooting

- **401 errors** – Ensure `github_token` (or a PAT) is passed to the action so it can post reviews.
- **422 diff errors** – When triggered via `issue_comment`, avoid inline comments unless your reviewer knows the diff lines. Use summary/issue comments instead.
- **Reviewer crashes** – The action surfaces stdout/stderr in the job log. Add logging in your reviewer to troubleshoot command execution.

By keeping reviewer logic in your repository and reusing the published action, you get a flexible review workflow with minimal maintenance overhead.
