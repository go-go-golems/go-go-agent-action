---
Title: Go Agent Code Review Action
Slug: go-agent-action
Short: Architecture, configuration, and testing guide for the Go-based code review GitHub Action with mock and pluggable review tools.
Topics:
- github-actions
- code-review
- go
IsTemplate: false
IsTopLevel: false
ShowPerDefault: true
SectionType: GeneralTopic
---

# Go Agent Code Review Action

The Go Agent Code Review Action packages GitHub pull-request context into a single payload, forwards it to a review tool (mock LLM, HTTP service, or CLI), and publishes the response back to the pull request. This document explains how the action is structured, how to configure it, and how to validate it locally and in CI.

## Architecture

The action is split into modular packages so each concern can evolve independently. The entrypoint in `cmd/agent-action/main.go` parses inputs, resolves the GitHub token, selects the review tool, and hands control to the runner.

- **`internal/action/config.go`** converts `INPUT_*` values and CLI flags into a typed `Inputs` struct, enforcing defaults such as `tool_mode: mock`.
- **`internal/action/context.go`** builds a `PRContext` payload by combining event metadata, pull-request details, changed files, optional file contents, guidelines, and globbed repo files.
- **`internal/action/triggers.go`** guards execution using trigger phrase, label, and assignee filters so the action only runs when requested.
- **`internal/action/tool.go`** defines pluggable adapters: `MockTool` for deterministic local runs, `HTTPTool` for remote services, and `CommandTool` for local executables. Each adapter returns a `ReviewResult` with summary markdown, review comments, and decision metadata.
- **`internal/action/publisher.go`** converts the `ReviewResult` into GitHub side effects: pull-request reviews, timeline comments, and `$GITHUB_STEP_SUMMARY` updates.
- **`internal/action/runner.go`** orchestrates the end-to-end flow, wiring the pieces together and logging when triggers are skipped.

## Configuration

Most customization happens through action inputs described in `action.yml`. Each section below lists the intent, not just the raw options.

- **Trigger controls** ensure reviews only happen when requested: `trigger_phrase` (default `@agent`), `label_trigger`, and `assignee_trigger`.
- **Context shaping** decides how much data is sent to the review tool: `include_patch`, `include_file_contents`, `include_repo_globs`, `guidelines_path`, plus `max_file_bytes` and `max_changed_files` caps.
- **Tool selection** swaps the review backend: `tool_mode` (`mock`, `http`, or `cmd`). HTTP mode uses `tool_url`, `tool_method`, `tool_headers_json`, `tool_token`. Command mode uses `tool_cmd`, `tool_args_json`, and `working_directory`.
- **Output routing** sets `output_mode` (any mix of `review`, `comment`, `summary`, `stdout`) and `max_comments` to bound how many inline notes are posted.
- **Authentication** falls back to the runner-provided `GITHUB_TOKEN` unless `github_token` is explicitly provided.

## Execution Flow

Every run follows the same sequence. Understanding it helps when debugging or integrating with a real LLM service.

1. **Input parsing** – CLI flags emitted by `action.yml` are merged with environment overrides.
2. **Context collection** – The runner downloads PR metadata, changed files, and optional repo files from the workspace checkout.
3. **Trigger evaluation** – If the triggers do not match, the action exits early and writes a note to the console.
4. **Tool invocation** – The selected adapter receives the `PRContext` JSON and returns a structured `ReviewResult`.
5. **Publishing** – Depending on `output_mode`, the action creates review comments, posts a timeline comment, and/or writes to the job summary.

A *dry run* of `act` (invoked with `-n`) simulates this flow without launching Docker containers. It prints the planned steps so you can verify configuration quickly. A *real* run omits `-n`, allowing `act` (or GitHub Actions) to execute the container, call GitHub APIs, and produce actual reviews/comments.

## Mock Reviewer Output

The bunded `MockTool` makes it easy to validate the GitHub integration before wiring a real LLM. It summarises the PR and emits deterministic comments.

- **Summary markdown** written to `$GITHUB_STEP_SUMMARY` (and optionally stdout):

```markdown
### Mock review for #123
- 3 changed file(s)
- Labels: backend, cleanup
- Guidelines attached
```

- **Review body and decision** posted as a single review with `COMMENT` state:

```text
Automated mock review
```

- **Inline comments** flag specific files. If no obvious issues are detected the first file receives a friendly "no blocking issues" note; otherwise, debug statements trigger suggestions such as:

```text
Mock LLM: consider removing debug prints before merging.
```

## Local Testing

Validating the action locally catches integration issues before pushing to GitHub.

- **Go build/tests** ensure dependencies resolve and the code compiles:

```bash
cd go-go-agent-action
GOCACHE=$(pwd)/.cache go test ./...
```

- **Workflow rehearsal with `act`** offers two modes:
  - *Dry run* (no containers, quick feedback):

    ```bash
    ~/go/bin/act -n -W examples/review.yml -P ubuntu-latest=ghcr.io/catthehacker/ubuntu:act-latest
    ```

  - *Full execution* (runs the Docker job exactly as GitHub would):

    ```bash
    ~/go/bin/act -W examples/review.yml -P ubuntu-latest=ghcr.io/catthehacker/ubuntu:act-latest
    ```

    The real run pulls the image, builds the action container, and executes the mock review, producing the summary and review artifacts described above.

## Tutorial: Build Your Own Reviewing Action

This tutorial walks through cloning the action skeleton, wiring your own reviewer, and releasing it for other repositories. The steps assume you are comfortable with Git, Go, and Docker-based GitHub Actions.

1. **Fork or copy the template** – Clone `go-go-golems/go-go-agent-action`, then rename the module in `go.mod` and the import in `cmd/agent-action/main.go` to match your GitHub repository path.
2. **Replace the review brain** – Implement an HTTP service or CLI that accepts the `PRContext` JSON. Swap `tool_mode` defaults in `internal/action/config.go` or change `buildTool` in `cmd/agent-action/main.go` to point at your service.
3. **Extend context if needed** – Add fields to `internal/action/types.go` and populate them in `internal/action/context.go` (for example, include CI results or commit history) before sending the payload to your reviewer.
4. **Run tests** – Execute `GOCACHE=$(pwd)/.cache go test ./...` and use `act` to simulate a workflow run.
5. **Publish the action** – Push the repository, tag a release (for example `v1.0.0`), and verify `action.yml` references `Dockerfile` at the repository root.

Once the action is public, consumers can reference `uses: your-org/your-review-action@v1` exactly like the examples below.

## Integrate the Action into a Repository

Adding automated reviews to another project follows the same pattern the `go-go-labs` repository uses.

- **Always-on reviews** – Create `.github/workflows/go-agent-review.yml` that triggers on `pull_request` events, sets `pull-requests: write` permissions, runs `actions/checkout`, and invokes the action with `tool_mode` of your choice.
- **On-demand mentions** – Add a second workflow listening to `issue_comment` events with a guard such as `contains(github.event.comment.body, '@agent')`. This keeps noise low while letting developers request a review when they are ready.
- **Token management** – Pass `github_token: ${{ secrets.GITHUB_TOKEN }}` (or a PAT) so the action can post reviews. Without it, the GitHub API responds with 401 errors when creating comments.

Test both workflows by opening a PR and leaving a comment that contains the trigger phrase. The mock reviewer responds instantly, giving you confidence before pointing the action at a real LLM backend.

## Customising go-go-agent-action

The starter is intentionally modular so you can extend it without rewriting everything.

- **New triggers** – Update `internal/action/triggers.go` to check additional signals (for example, branch name patterns or files touched). When triggers become complex, consider exposing new inputs in `action.yml` and plumbing them through `internal/action/config.go`.
- **Alternate outputs** – Modify `internal/action/publisher.go` to write to other destinations. You can add support for Checks API summaries, Slack webhooks, or custom artifacts alongside the pull-request review.
- **Different packaging** – If Docker is not desirable, port `cmd/agent-action/main.go` into a composite or JavaScript action, reusing the Go packages for context collection by calling them through `go run`.
- **Enhanced tooling** – Implement additional tools in `internal/action/tool.go`, such as a gRPC client or a multi-agent coordinator that fans out to several services before aggregating comments.

Whenever you customise the action, document new inputs in `action.yml`, update `README.md`, and add regression tests or fixtures so `act` runs continue to succeed.

## GitHub Integration

The action slots into standard PR workflows. Include `actions/checkout` with `persist-credentials: false`, then invoke the action. With the mock tool you can iterate safely; switching to `tool_mode: http` or `cmd` later keeps the surrounding workflow identical.

```yaml
jobs:
  review:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write
    steps:
      - uses: actions/checkout@v4
        with:
          persist-credentials: false

      - name: Automated review
        uses: go-go-golems/go-go-agent-action@v1
        with:
          tool_mode: http
          tool_url: https://agent.internal.example/review
          include_patch: true
          output_mode: review+summary
```

When deployed in GitHub Actions, the published outputs appear in three locations:

- Pull request **Review tab** with the review state and inline comments.
- Pull request **Timeline** if `issue_comment` is provided.
- Run summary page via the **Job Summary** tab, showing `summary_markdown`.

## Publishing Checklist

Packaging the action for reuse involves standard Docker-action steps.

1. Build the container locally to validate the Dockerfile: `docker build -t agent-action .`
2. Push the repository to GitHub and create a release tag (for example `v1`).
3. Update downstream workflows to reference `uses: go-go-golems/go-go-agent-action@v1` and switch `tool_mode` from `mock` to your production reviewer once it is ready.

Following these steps keeps the review boundary stable while letting you iterate on the backend tool independently of the GitHub integration.
