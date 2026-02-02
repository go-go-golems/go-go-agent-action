package action

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// ReviewTool is any component capable of turning PR context into review output.
type ReviewTool interface {
	Review(ctx context.Context, pr *PRContext) (*ReviewResult, error)
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
