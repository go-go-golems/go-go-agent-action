package action

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// ReviewTool is any component capable of turning PR context into review output.
type ReviewTool interface {
	Review(ctx context.Context, pr *PRContext) (*ReviewResult, error)
}

// HTTPTool posts the PR context to an external service and expects ReviewResult JSON back.
type HTTPTool struct {
	Client  *http.Client
	URL     string
	Method  string
	Headers map[string]string
	Token   string
}

func (t *HTTPTool) Review(ctx context.Context, pr *PRContext) (*ReviewResult, error) {
	if t.Client == nil {
		t.Client = http.DefaultClient
	}
	method := strings.ToUpper(strings.TrimSpace(t.Method))
	if method == "" {
		method = http.MethodPost
	}
	payload, err := json.Marshal(pr)
	if err != nil {
		return nil, fmt.Errorf("marshal context: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, t.URL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range t.Headers {
		req.Header.Set(k, v)
	}
	if t.Token != "" {
		req.Header.Set("Authorization", "Bearer "+t.Token)
	}

	resp, err := t.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("tool HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result ReviewResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse tool response: %w", err)
	}
	return &result, nil
}

// PromptHTTPTool posts rendered prompt text to an external service and expects ReviewResult JSON back.
type PromptHTTPTool struct {
	Client  *http.Client
	URL     string
	Method  string
	Headers map[string]string
	Token   string
}

func (t *PromptHTTPTool) Review(ctx context.Context, pr *PRContext) (*ReviewResult, error) {
	if t.Client == nil {
		t.Client = http.DefaultClient
	}
	method := strings.ToUpper(strings.TrimSpace(t.Method))
	if method == "" {
		method = http.MethodPost
	}
	if pr.PromptText == "" {
		return nil, fmt.Errorf("prompt_text is empty")
	}
	req, err := http.NewRequestWithContext(ctx, method, t.URL, strings.NewReader(pr.PromptText))
	if err != nil {
		return nil, err
	}
	if _, ok := t.Headers["Content-Type"]; !ok {
		req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	}
	for k, v := range t.Headers {
		req.Header.Set(k, v)
	}
	if t.Token != "" {
		req.Header.Set("Authorization", "Bearer "+t.Token)
	}

	resp, err := t.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("tool HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result ReviewResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse tool response: %w", err)
	}
	return &result, nil
}

// CommandTool executes a local process, piping context JSON to stdin and reading ReviewResult JSON from stdout.
type CommandTool struct {
	Command string
	Args    []string
	Dir     string
	Runner  func(ctx context.Context, name string, args ...string) *exec.Cmd
}

func (c *CommandTool) Review(ctx context.Context, pr *PRContext) (*ReviewResult, error) {
	if c.Command == "" {
		return nil, fmt.Errorf("tool command is required")
	}
	run := c.Runner
	if run == nil {
		run = exec.CommandContext
	}
	cmd := run(ctx, c.Command, c.Args...)
	if c.Dir != "" {
		cmd.Dir = c.Dir
	}

	payload, err := json.Marshal(pr)
	if err != nil {
		return nil, err
	}

	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = toolStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("tool command failed: %v\nstderr: %s", err, stderr.String())
	}

	var result ReviewResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("parse tool stdout: %v\nstdout: %s", err, stdout.String())
	}
	return &result, nil
}

// PromptCommandTool executes a local process, piping rendered prompt text to stdin.
type PromptCommandTool struct {
	Command string
	Args    []string
	Dir     string
	Runner  func(ctx context.Context, name string, args ...string) *exec.Cmd
}

func (c *PromptCommandTool) Review(ctx context.Context, pr *PRContext) (*ReviewResult, error) {
	if c.Command == "" {
		return nil, fmt.Errorf("tool command is required")
	}
	if pr.PromptText == "" {
		return nil, fmt.Errorf("prompt_text is empty")
	}
	run := c.Runner
	if run == nil {
		run = exec.CommandContext
	}
	cmd := run(ctx, c.Command, c.Args...)
	if c.Dir != "" {
		cmd.Dir = c.Dir
	}

	cmd.Stdin = strings.NewReader(pr.PromptText)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = toolStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("tool command failed: %v\nstderr: %s", err, stderr.String())
	}

	var result ReviewResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("parse tool stdout: %v\nstdout: %s", err, stdout.String())
	}
	return &result, nil
}

// MockTool is a deterministic in-process reviewer used for local development and tests.
type MockTool struct{}

func (MockTool) Review(_ context.Context, pr *PRContext) (*ReviewResult, error) {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("### Mock review for #%d\n", pr.Number))
	builder.WriteString(fmt.Sprintf("- %d changed file(s)\n", len(pr.ChangedFiles)))
	if len(pr.Labels) > 0 {
		builder.WriteString(fmt.Sprintf("- Labels: %s\n", strings.Join(pr.Labels, ", ")))
	}
	if pr.GuidelinesB64 != "" {
		builder.WriteString("- Guidelines attached\n")
	}

	comments := make([]ReviewComment, 0, len(pr.ChangedFiles))
	for _, file := range pr.ChangedFiles {
		if strings.Contains(file.Patch, "fmt.Print") {
			comments = append(comments, ReviewComment{
				Path: file.Path,
				Body: "Mock LLM: consider removing debug prints before merging.",
				Line: 1,
				Side: "RIGHT",
			})
		}
	}
	if len(comments) == 0 && len(pr.ChangedFiles) > 0 {
		comments = append(comments, ReviewComment{
			Path: pr.ChangedFiles[0].Path,
			Body: "Mock LLM: file reviewed automatically; no blocking issues detected.",
			Line: 1,
			Side: "RIGHT",
		})
	}

	return &ReviewResult{
		SummaryMarkdown: builder.String(),
		Comments:        comments,
		ReviewDecision:  "comment",
		ReviewBody:      "Automated mock review",
		Diary: &AgentDiary{
			Summary:       fmt.Sprintf("Reviewed %d file(s) using the mock tool.", len(pr.ChangedFiles)),
			FilesExamined: buildMockDiaryFiles(pr.ChangedFiles),
			WhatILookedFor: []string{
				"Debug prints and other temporary code",
				"Patterns that match the mock review rules",
			},
			WhatIFound: []DiaryFinding{
				{
					Type:        "info",
					Description: "Mock tool emits a sample finding for demonstration.",
				},
			},
		},
	}, nil
}

func buildMockDiaryFiles(files []ChangedFile) []DiaryFileEntry {
	if len(files) == 0 {
		return nil
	}
	out := make([]DiaryFileEntry, 0, len(files))
	for _, file := range files {
		out = append(out, DiaryFileEntry{
			Path:    file.Path,
			Summary: "Checked diff for mock review patterns.",
		})
	}
	return out
}

func toolStderr(buffer *bytes.Buffer) io.Writer {
	val := strings.TrimSpace(os.Getenv("GO_GO_AGENT_ACTION_LOG_TOOL_STDERR"))
	if strings.EqualFold(val, "1") || strings.EqualFold(val, "true") {
		return io.MultiWriter(buffer, os.Stderr)
	}
	return buffer
}
