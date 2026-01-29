package action

import "encoding/base64"

// ChangedFile captures metadata for a file touched in the pull request.
type ChangedFile struct {
	Path        string `json:"path"`
	Status      string `json:"status"`
	Patch       string `json:"patch,omitempty"`
	Additions   int    `json:"additions,omitempty"`
	Deletions   int    `json:"deletions,omitempty"`
	BlobURL     string `json:"blob_url,omitempty"`
	RawURL      string `json:"raw_url,omitempty"`
	ContentsB64 string `json:"contents_b64,omitempty"`
}

// RepoFile is any additional repository file bundled into the review context.
type RepoFile struct {
	Path        string `json:"path"`
	ContentsB64 string `json:"contents_b64"`
}

// PRContext is the payload sent to the review tool.
type PRContext struct {
	Owner         string        `json:"owner"`
	Repo          string        `json:"repo"`
	Number        int           `json:"number"`
	Title         string        `json:"title"`
	Body          string        `json:"body"`
	BaseRef       string        `json:"base_ref"`
	HeadRef       string        `json:"head_ref"`
	HeadSHA       string        `json:"head_sha"`
	UserLogin     string        `json:"user_login"`
	Labels        []string      `json:"labels"`
	Assignees     []string      `json:"assignees"`
	ChangedFiles  []ChangedFile `json:"changed_files"`
	GuidelinesB64 string        `json:"guidelines_b64,omitempty"`
	ExtraFiles    []RepoFile    `json:"extra_files,omitempty"`
	TriggeredBy   string        `json:"triggered_by"`
	EventName     string        `json:"event_name"`
	TriggerText   string        `json:"trigger_text,omitempty"`
	RunID         string        `json:"run_id"`
	PromptText    string        `json:"prompt_text,omitempty"`
	PromptMeta    *PromptMeta   `json:"prompt_meta,omitempty"`
}

// PromptMeta captures details about prompt rendering.
type PromptMeta struct {
	TemplatePath string `json:"template_path,omitempty"`
	Engine       string `json:"engine,omitempty"`
	RenderedAt   string `json:"rendered_at,omitempty"`
}

// ReviewComment mirrors GitHub's modern review comment shape.
type ReviewComment struct {
	Path      string `json:"path"`
	Body      string `json:"body"`
	Line      int    `json:"line,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	Side      string `json:"side,omitempty"`
	StartSide string `json:"start_side,omitempty"`
	Subject   string `json:"subject_type,omitempty"`
}

// ReviewResult is returned by the review tool and controls how we
// publish feedback back to GitHub.
type ReviewResult struct {
	SummaryMarkdown string          `json:"summary_markdown,omitempty"`
	Comments        []ReviewComment `json:"comments,omitempty"`
	IssueComment    string          `json:"issue_comment,omitempty"`
	ReviewDecision  string          `json:"review_decision,omitempty"`
	ReviewBody      string          `json:"review_body,omitempty"`
	Diary           *AgentDiary     `json:"diary,omitempty"`
}

// AgentDiary captures the reviewer's internal reasoning in a structured format.
type AgentDiary struct {
	Summary            string             `json:"summary,omitempty"`
	FilesExamined      []DiaryFileEntry   `json:"files_examined,omitempty"`
	WhatILookedFor     []string           `json:"what_i_looked_for,omitempty"`
	WhatIFound         []DiaryFinding     `json:"what_i_found,omitempty"`
	Uncertainties      []DiaryUncertainty `json:"uncertainties,omitempty"`
	WhatILearned       []string           `json:"what_i_learned,omitempty"`
	TrickyToAnalyze    []string           `json:"tricky_to_analyze,omitempty"`
	SecondPairOfEyes   []string           `json:"second_pair_of_eyes,omitempty"`
	FutureWork         []string           `json:"future_work,omitempty"`
	ReviewInstructions string             `json:"review_instructions,omitempty"`
	TechnicalDetails   string             `json:"technical_details,omitempty"`
}

// DiaryFileEntry summarizes how a file was reviewed.
type DiaryFileEntry struct {
	Path            string   `json:"path,omitempty"`
	Summary         string   `json:"summary,omitempty"`
	PatternsChecked []string `json:"patterns_checked,omitempty"`
}

// DiaryFinding records a notable outcome from the review.
type DiaryFinding struct {
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
	File        string `json:"file,omitempty"`
	Line        int    `json:"line,omitempty"`
}

// DiaryUncertainty captures items that need verification or a second look.
type DiaryUncertainty struct {
	Description string `json:"description,omitempty"`
	Reason      string `json:"reason,omitempty"`
	File        string `json:"file,omitempty"`
	Line        int    `json:"line,omitempty"`
}

// DecodeGuidelines decodes the embedded guidelines file.
func (c *PRContext) DecodeGuidelines() ([]byte, error) {
	if c.GuidelinesB64 == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(c.GuidelinesB64)
}
