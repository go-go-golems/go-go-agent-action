package action

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/go-github/v66/github"
)

// Publisher bridges the ReviewResult with GitHub outputs (review, comment, summary).
type Publisher struct {
	GitHub    ReviewPublisher
	Inputs    *Inputs
	Env       RuntimeEnv
	WriteFile func(filename string, data []byte, perm os.FileMode) error
	Stdout    io.Writer
}

// NewPublisher returns a Publisher with sensible defaults for file/console access.
func NewPublisher(gh ReviewPublisher, in *Inputs, env RuntimeEnv) *Publisher {
	return &Publisher{
		GitHub:    gh,
		Inputs:    in,
		Env:       env,
		WriteFile: os.WriteFile,
		Stdout:    os.Stdout,
	}
}

func (p *Publisher) Publish(ctx context.Context, pr *PRContext, result *ReviewResult) error {
	if result == nil {
		return fmt.Errorf("tool returned no result")
	}

	modes := parseOutputModes(p.Inputs.OutputMode)

	summary := result.SummaryMarkdown
	if modes["diary"] && result.Diary != nil {
		summary = appendDiaryMarkdown(summary, formatDiarySection(result.Diary, diaryRenderFull))
	}
	if modes["summary"] && summary != "" && p.Env.StepSummaryPath != "" {
		_ = p.WriteFile(p.Env.StepSummaryPath, []byte(summary), 0644)
	}
	if modes["stdout"] && summary != "" {
		fmt.Fprintln(p.Stdout, summary)
	}

	publishGitHub := !p.Inputs.DryRun && (modes["comment"] || modes["review"])
	if !publishGitHub {
		return nil
	}

	if modes["comment"] && result.IssueComment != "" {
		_, _, err := p.GitHub.CreateIssueComment(ctx, pr.Owner, pr.Repo, pr.Number, &github.IssueComment{Body: github.String(result.IssueComment)})
		if err != nil {
			return fmt.Errorf("create issue comment: %w", err)
		}
	}

	if modes["review"] {
		if err := p.publishReview(ctx, pr, result); err != nil {
			return err
		}
	}

	return nil
}

func (p *Publisher) publishReview(ctx context.Context, pr *PRContext, result *ReviewResult) error {
	sanitized, reviewBody, warnings := sanitizeReviewComments(pr, result.Comments, result.ReviewBody, p.Inputs.MaxComments)
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}
	if modes := parseOutputModes(p.Inputs.OutputMode); modes["diary"] && result.Diary != nil {
		reviewBody = appendDiaryMarkdown(reviewBody, formatDiarySection(result.Diary, diaryRenderBrief))
	}

	comments := make([]*github.DraftReviewComment, 0, len(sanitized))
	for _, comment := range sanitized {
		draft := &github.DraftReviewComment{
			Path: github.String(comment.Path),
			Body: github.String(comment.Body),
		}
		if comment.Line > 0 {
			draft.Line = github.Int(comment.Line)
			if strings.EqualFold(comment.Side, "LEFT") {
				draft.Side = github.String("LEFT")
			} else {
				draft.Side = github.String("RIGHT")
			}
		}
		if comment.StartLine > 0 {
			draft.StartLine = github.Int(comment.StartLine)
			if strings.EqualFold(comment.StartSide, "LEFT") {
				draft.StartSide = github.String("LEFT")
			} else {
				draft.StartSide = github.String("RIGHT")
			}
		}
		comments = append(comments, draft)
	}

	if len(comments) == 0 && strings.TrimSpace(reviewBody) == "" && result.ReviewDecision == "" {
		return nil
	}

	var event string
	switch strings.ToUpper(result.ReviewDecision) {
	case "APPROVE":
		event = "APPROVE"
	case "REQUEST_CHANGES":
		event = "REQUEST_CHANGES"
	case "COMMENT", "":
		event = "COMMENT"
	default:
		event = "COMMENT"
	}

	req := &github.PullRequestReviewRequest{
		Body:     github.String(reviewBody),
		Event:    github.String(event),
		Comments: comments,
	}

	if _, _, err := p.GitHub.CreateReview(ctx, pr.Owner, pr.Repo, pr.Number, req); err != nil {
		return fmt.Errorf("create review: %w; comments=%s", err, summarizeDraftComments(comments))
	}
	return nil
}

func parseOutputModes(mode string) map[string]bool {
	modes := map[string]bool{}
	for _, part := range strings.Split(mode, "+") {
		part = strings.TrimSpace(strings.ToLower(part))
		if part != "" {
			modes[part] = true
		}
	}
	if len(modes) == 0 {
		modes["review"] = true
	}
	return modes
}
