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

This guide focuses on using the published `go-go-golems/go-go-agent-action` without modifying its source. You will learn how to build a reviewer binary or service, consume the full pull-request context, return multiple inline comments, and run the action from a GitHub workflow that supports both pull-request events and `@agent` mentions.

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
  "body": "Sample change\n```agent\ngo test ./...\n```",
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
      "contents_b64": "..."
    }
  ],
  "guidelines_b64": "...",
  "extra_files": [
    { "path": "docs/adr/001.md", "contents_b64": "..." }
  ],
  "triggered_by": "octocat",
  "event_name": "issue_comment",
  "trigger_text": "@agent run npm test",
  "run_id": "123456789"
}
```

Key fields:

- `body` – the PR description. You can embed reviewer-specific directives here.
- `trigger_text` – the comment body when a user mentions `@agent`.
- `changed_files` – always present for pull-request events; includes diff metadata and optionally file contents.
- `guidelines_b64` / `extra_files` – additional files requested via action inputs (base64 encoded).

### ReviewResult structure

```json
{
  "summary_markdown": "### Review summary\n- run `go test ./...`\n- run `npm test`",
  "review_decision": "comment",
  "review_body": "Requested commands listed below.",
  "issue_comment": "Requested commands:\n\n- `go test ./...`\n- `npm test`",
  "comments": [
    {
      "path": "cmd/app/main.go",
      "body": "Please run `go test ./...` and update the PR with the results.",
      "subject_type": "file"
    },
    {
      "path": "cmd/app/main.go",
      "body": "Please run `npm test` and update the PR with the results.",
      "subject_type": "file"
    }
  ]
}
```

- `summary_markdown` can list commands or findings.
- `issue_comment` is optional but recommended for mention-triggered runs (inline comments may not be accepted by GitHub when diff context is missing).
- `comments` accepts multiple entries. Use `subject_type:"file"` for file-level comments or specify `line`/`side` (and optionally `start_line`/`start_side`) for inline diffs.

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

The action sends the JSON payload to `tool_url` and expects a `ReviewResult` response. Use this when your reviewer runs outside GitHub (e.g., Kubernetes, serverless).

### CLI reviewer with PR-defined commands

Keep the reviewer next to your code so it evolves with the repository. The example below extracts commands from:

- Fenced code blocks labelled `agent` in the PR body or comment.
- Lines starting with `@agent run` in the PR body or comment.
- A default list when nothing is specified.

It posts file-level comments for each command on pull-request events and falls back to summary/issue comments for mention-triggered runs.

```go
package main

import (
    "bufio"
    "encoding/json"
    "fmt"
    "os"
    "strings"
)

func main() {
    var ctx struct {
        Number       int `json:"number"`
        Body         string `json:"body"`
        TriggerText  string `json:"trigger_text"`
        EventName    string `json:"event_name"`
        ChangedFiles []struct {
            Path string `json:"path"`
        } `json:"changed_files"`
    }
    if err := json.NewDecoder(os.Stdin).Decode(&ctx); err != nil {
        panic(err)
    }

    commands := gatherCommands(ctx.Body, ctx.TriggerText)
    if len(commands) == 0 {
        commands = []string{"go test ./...", "go vet ./...", "golangci-lint run"}
    }

    summary := buildSummary(commands)
    message := buildMessage(commands)

    result := struct {
        SummaryMarkdown string `json:"summary_markdown"`
        ReviewDecision  string `json:"review_decision"`
        ReviewBody      string `json:"review_body"`
        IssueComment    string `json:"issue_comment"`
        Comments        []struct {
            Path    string `json:"path"`
            Body    string `json:"body"`
            Subject string `json:"subject_type"`
        } `json:"comments"`
    }{
        SummaryMarkdown: summary,
        ReviewDecision:  "comment",
        ReviewBody:      message,
    }

    if ctx.EventName == "issue_comment" {
        result.IssueComment = message
    }

    if ctx.EventName == "pull_request" && len(ctx.ChangedFiles) > 0 {
        for i, cmd := range commands {
            path := ctx.ChangedFiles[i%len(ctx.ChangedFiles)].Path
            result.Comments = append(result.Comments, struct {
                Path    string `json:"path"`
                Body    string `json:"body"`
                Subject string `json:"subject_type"`
            }{
                Path:    path,
                Body:    fmt.Sprintf("Please run `%s` and update the PR with the results.", cmd),
                Subject: "file",
            })
        }
    }

    if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
        panic(err)
    }
}

func gatherCommands(body, trigger string) []string {
    seen := map[string]struct{}{}
    add := func(list []string, commands *[]string) {
        for _, cmd := range list {
            trimmed := strings.TrimSpace(cmd)
            if trimmed == "" {
                continue
            }
            if _, ok := seen[trimmed]; ok {
                continue
            }
            seen[trimmed] = struct{}{}
            *commands = append(*commands, trimmed)
        }
    }

    var commands []string
    add(extractAgentBlocks(body), &commands)
    add(extractAgentBlocks(trigger), &commands)
    add(extractRunLines(body), &commands)
    add(extractRunLines(trigger), &commands)
    return commands
}

func extractAgentBlocks(text string) []string {
    const marker = "```agent"
    lower := strings.ToLower(text)
    search := 0
    var cmds []string
    for {
        idx := strings.Index(lower[search:], marker)
        if idx == -1 {
            break
        }
        idx += search
        start := idx + len(marker)
        for start < len(text) && (text[start] == '\n' || text[start] == '\r') {
            start++
        }
        endIdx := strings.Index(lower[start:], "```")
        if endIdx == -1 {
            break
        }
        end := start + endIdx
        block := text[start:end]
        scanner := bufio.NewScanner(strings.NewReader(block))
        for scanner.Scan() {
            trimmed := strings.TrimSpace(scanner.Text())
            if trimmed != "" {
                cmds = append(cmds, trimmed)
            }
        }
        search = end + len("```")
    }
    return cmds
}

func extractRunLines(text string) []string {
    scanner := bufio.NewScanner(strings.NewReader(text))
    var cmds []string
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if strings.HasPrefix(strings.ToLower(line), "@agent run ") {
            cmds = append(cmds, strings.TrimSpace(line[len("@agent run ") :]))
        }
    }
    return cmds
}

func buildSummary(commands []string) string {
    if len(commands) == 0 {
        return "### Automated review\n- no commands requested"
    }
    lines := []string{"### Automated review"}
    for _, cmd := range commands {
        lines = append(lines, fmt.Sprintf("- run `%s`", cmd))
    }
    return strings.Join(lines, "\n")
}

func buildMessage(commands []string) string {
    if len(commands) == 0 {
        return "No commands were provided for this review."
    }
    var b strings.Builder
    b.WriteString("Requested commands:\n\n")
    for _, cmd := range commands {
        b.WriteString("- `")
        b.WriteString(cmd)
        b.WriteString("`\n")
    }
    return b.String()
}
```

This reviewer allows authors to declare commands directly in the PR body or mention comment.

## Step 3 – Create the workflow glue

Use a single workflow to cover both pull requests and mentions (see `go-go-labs/.github/workflows/go-agent.yml` for a full example).

```yaml
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

When triggered by a pull request update, the reviewer receives the full diff and can leave file-level comments. When triggered by an `@agent` mention, the action posts a timeline comment with the commands.

## Step 4 – Pass configuration and secrets

Configuration remains entirely in your repository:

- Commit `reviewer.yaml` and load it inside the reviewer.
- Use workflow inputs (`with:`) or environment variables to toggle behaviour.
- Store tokens in repository secrets and expose them via `env:`.

## Step 5 – Test locally (optional)

Use `go test` and `act` to simulate both triggers. Supply event fixtures for pull requests and issue comments so you can confirm command extraction works.

## Step 6 – Commit and merge

Once the reviewer and workflow are committed to `main`, developers receive automated reviews whenever they update a PR. They can also request one manually with `@agent run <command>` in a comment.

## Step 7 – Reuse across repositories (optional)

Package the reviewer as a module or container if multiple repos require it. The workflow installs the reviewer artefact before calling the action.

## Troubleshooting

- **401 errors** – Ensure `github_token` (or a PAT) is passed; the action cannot post reviews without it.
- **422 diff errors** – Avoid inline comments on mention-triggered runs unless your reviewer sets file-level comments (`subject_type:"file"`) or has diff context.
- **Reviewer crashes** – Inspect the job logs; stdout/stderr from the reviewer is piped back so you can add logging and debug command failures.

By keeping reviewer logic in your repository or service and reusing the action, you get a flexible review workflow without maintaining a fork of the action itself.
