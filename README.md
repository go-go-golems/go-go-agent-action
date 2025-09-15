# Agent Code Review Action (Go)

A modular GitHub Action that collects rich pull-request context, hands it to a review tool (mock LLM for local runs, HTTP service, or CLI), and maps the structured response back into GitHub reviews, comments, and step summaries.

> Built from the go-go-labs agent starter, but refactored around a clear architecture so you can swap the review brain while keeping the GitHub plumbing stable.

## Architecture at a Glance

- **Input parsing (`internal/action/config.go`)** – normalises `INPUT_*` / CLI args into a typed config object.
- **Context collector (`internal/action/context.go`)** – reads the workflow event payload and GitHub APIs to build a single `PRContext` JSON blob (metadata, labels, assignees, diff, optional file contents, guidelines, extra globs).
- **Trigger engine (`internal/action/triggers.go`)** – enforces `trigger_phrase`, `label_trigger`, and `assignee_trigger` before we spend tokens.
- **Tool adapters (`internal/action/tool.go`)** – pluggable “review brains”: `mock` (in-process heuristic), `http`, or `cmd`. They all speak the same `ReviewResult` contract.
- **Publisher (`internal/action/publisher.go`)** – pushes the result back to GitHub (`CreateReview`, `CreateComment`, `$GITHUB_STEP_SUMMARY`, stdout) with proper batching/limits.
- **Runner (`internal/action/runner.go`)** – orchestrates the flow and handles logging.

Everything is wired from `cmd/agent-action/main.go`, which bootstraps the GitHub client, chooses the tool, then calls the runner.

## Key Inputs

See [`action.yml`](./action.yml) for the full list. Highlights:

| Input | Purpose |
| ----- | ------- |
| `tool_mode` | `mock` (default), `http`, or `cmd` |
| `tool_url`, `tool_method`, `tool_headers_json`, `tool_token` | HTTP tool wiring |
| `tool_cmd`, `tool_args_json`, `working_directory` | CLI tool wiring |
| `trigger_phrase`, `label_trigger`, `assignee_trigger` | Control when the action actually runs |
| `include_patch`, `include_file_contents`, `include_repo_globs`, `max_*` | Shape the code-review context |
| `output_mode` | Mix of `review`, `comment`, `summary`, `stdout` |
| `max_comments` | Inline comment cap |

## Mock Reviewer for Local Development

Use the built-in mock to exercise the full GitHub integration without hitting a real LLM:

```yaml
name: agent-review
on:
  pull_request:
    types: [opened, synchronize, reopened]

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

      - name: Mock code review
        uses: ./.  # swap for go-go-golems/go-go-agent-action@v1 after publishing
        with:
          tool_mode: mock
          include_patch: true
          output_mode: review+summary
```

The mock engine summarises the PR, leaves deterministic file-level comments (it flags obvious `fmt.Print` debug statements), and submits a `COMMENT` review so you can verify the plumbing.

## Integrating a Real Review Service

Switch `tool_mode` to `http` or `cmd` once your backend is ready:

```yaml
      - uses: go-go-golems/go-go-agent-action@v1
        with:
          tool_mode: http
          tool_url: https://agent.internal.example/review
          tool_token: ${{ secrets.AGENT_TOKEN }}
          include_patch: true
          output_mode: review+summary
```

Implement the response contract from [`internal/action/types.go`](internal/action/types.go):

```json
{
  "summary_markdown": "### Automated review\n- 5 files analysed",
  "review_decision": "comment",
  "review_body": "Automated feedback",
  "comments": [
    { "path": "pkg/foo/foo.go", "body": "nit:", "line": 42, "side": "RIGHT" }
  ]
}
```

## How to Test It

1. **Unit/integration build** – run `GOCACHE=$(pwd)/.cache go test ./...` from this folder (requires Go 1.22+ and module deps). Tests validate the orchestration, trigger gating, and tool adapters.
2. **Workflow rehearsal** – use [`examples/review.yml`](examples/review.yml) with [`act`](https://github.com/nektos/act) or a throwaway repo. Set `tool_mode: mock` so no external calls are made.
3. **End-to-end in GitHub** – push a branch with the action, open a PR, and mention the trigger phrase (`@agent` by default). Check the PR timeline, review tab, and job summary for the mock output.

## Automation & CI

### Makefile targets

- `make test` – runs `go test ./...` (mirrors the CI job) and is the default way to exercise the suite locally.
- `make lint` / `make lintmax` – invoke [`golangci-lint`](.golangci.yml) with the repo's settings; use `lintmax` when you need a larger duplicate issue budget.
- `make build` – compiles all packages to ensure the action container still builds cleanly.
- `make gosec` / `make govulncheck` – optional security sweeps you can trigger before larger releases.
- `make tag-{major,minor,patch}` – call [`svu`](https://github.com/caarlos0/svu) to propose the next semantic version. Install it once with `go install github.com/caarlos0/svu/cmd/svu@latest`.
- `make docker-lint` – runs the lint suite inside the official Docker image, which matches the GitHub Action.

### GitHub Actions workflow

- `.github/workflows/ci.yml` runs on pushes to `main` and every pull request. The `golangci-lint` job uses `golangci/golangci-lint-action@v8` against this repo's config, and the `go test` job executes `make test` with the same Go version declared in `go.mod` (1.22).
- Tags pushed with the `svu` helpers automatically benefit from fetching the full tag history in CI, so the workflow keeps `fetch-depth: 0` to make those commands consistent locally and in automation.
- Security coverage comes from `codeql-analysis`, `dependency-scanning`, and `secret-scanning` workflows. They run weekly plus on every PR/push to `main`, covering CodeQL, Go vuln checks (`govulncheck`, Nancy, gosec), GitHub dependency review, and TruffleHog secret scanning.

## Publishing Checklist

1. Build the action container: `docker build -t agent-action .`
2. Push to a new repository, tag a release (`v1`), and reference it via `uses: <owner>/<repo>@v1`.
3. Gradually migrate consumers from mock to `http`/`cmd` modes once your review service is ready.

The interfaces are intentionally small: once your LLM backend matures you can plug it in without rewiring how GitHub data is collected or posted.
