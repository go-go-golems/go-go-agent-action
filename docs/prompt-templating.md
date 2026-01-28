---
Title: Prompt Templating System
Slug: prompt-templating
Short: Guide to using Go templates with sprig functions and embedded fragments for LLM prompts.
Topics:
- github-actions
- code-review
- prompts
- templating
IsTemplate: false
IsTopLevel: false
ShowPerDefault: true
SectionType: GeneralTopic
---

# Prompt Templating System

The go-go-agent-action includes a powerful prompt templating system that lets you customize the prompts sent to your review tool. It uses Go's `text/template` engine enhanced with [sprig](https://masterminds.github.io/sprig/) functions and embedded template fragments.

## Quick Start

1. Create a prompt template in your repository:

```
{{/* .github/prompts/review.tmpl */}}
{{ template "system-role" . }}

{{ template "review-output-format" . }}

## PR Details
- **Title:** {{ .PR.Title }}
- **Author:** {{ .PR.UserLogin }}
- **Branch:** {{ .PR.HeadRef }} → {{ .PR.BaseRef }}

## Changed Files
{{ range .PR.ChangedFiles -}}
### {{ .Path }} ({{ .Status }})
{{ if .Patch -}}
```diff
{{ .Patch }}
```
{{ end }}
{{ end }}

{{ template "review-guidelines" . }}
{{ template "response-requirements" . }}
```

2. Configure the action to use your template:

```yaml
- uses: go-go-golems/go-go-agent-action@v1
  with:
    tool_mode: http
    tool_url: https://your-llm-service/review
    prompt_template_path: .github/prompts/review.tmpl
    tool_input_mode: prompt_text
```

## Template Data

Your template receives a `TemplateData` struct with these fields:

| Field | Type | Description |
|-------|------|-------------|
| `.PR` | `*PRContext` | Full PR context (see below) |
| `.Guidelines` | `string` | Decoded contents of guidelines file |
| `.ExtraFiles` | `[]TemplateFile` | Array of extra repo files |
| `.ExtraFileMap` | `map[string]string` | Map of path → contents |
| `.Vars` | `map[string]any` | Custom variables from `prompt_template_vars_json` |

### PRContext Fields

| Field | Description |
|-------|-------------|
| `.PR.Owner` | Repository owner |
| `.PR.Repo` | Repository name |
| `.PR.Number` | PR number |
| `.PR.Title` | PR title |
| `.PR.Body` | PR description |
| `.PR.BaseRef` | Target branch |
| `.PR.HeadRef` | Source branch |
| `.PR.HeadSHA` | Head commit SHA |
| `.PR.UserLogin` | PR author |
| `.PR.Labels` | Array of label names |
| `.PR.Assignees` | Array of assignee logins |
| `.PR.ChangedFiles` | Array of changed files |
| `.PR.TriggeredBy` | User who triggered the action |
| `.PR.EventName` | GitHub event type |
| `.PR.TriggerText` | Comment body for mention triggers |
| `.PR.RunID` | Workflow run ID |

### ChangedFile Fields

| Field | Description |
|-------|-------------|
| `.Path` | File path |
| `.Status` | added/modified/removed/renamed |
| `.Patch` | Unified diff (if `include_patch=true`) |
| `.Additions` | Lines added |
| `.Deletions` | Lines removed |
| `.BlobURL` | GitHub blob URL |
| `.RawURL` | Raw content URL |
| `.ContentsB64` | Base64 file contents (if `include_file_contents=true`) |

## Embedded Fragments

The action embeds reusable template fragments in `prompts/fragments/`. Use them with the `template` directive:

### Available Fragments

| Fragment | Description |
|----------|-------------|
| `system-role` | Basic system role preamble for code review |
| `review-output-format` | Full ReviewResult JSON schema documentation |
| `review-guidelines` | Comment quality principles and issue categories |
| `review-example` | Example ReviewResult JSON output |
| `response-requirements` | Final requirements (valid JSON, limits, etc.) |
| `pr-context-description` | Documentation of PRContext input fields |

### Fragment Usage

```
{{/* Include the system role */}}
{{ template "system-role" . }}

{{/* Include output format instructions */}}
{{ template "review-output-format" . }}

{{/* Add review guidelines */}}
{{ template "review-guidelines" . }}

{{/* Show example output */}}
{{ template "review-example" . }}

{{/* Final requirements */}}
{{ template "response-requirements" . }}

{{/* Or pass custom data to fragments */}}
{{ template "response-requirements" dict "maxComments" 50 }}
```

## Sprig Functions

All [sprig template functions](https://masterminds.github.io/sprig/) are available. Common examples:

### String Functions

```
{{ .PR.Title | upper }}                    {{/* UPPERCASE */}}
{{ .PR.Title | lower }}                    {{/* lowercase */}}
{{ .PR.Title | title }}                    {{/* Title Case */}}
{{ .PR.Title | trim }}                     {{/* trim whitespace */}}
{{ .PR.Title | trunc 50 }}                 {{/* truncate to 50 chars */}}
{{ .PR.Labels | join ", " }}               {{/* join array: "bug, feature" */}}
{{ .PR.Body | replace "old" "new" }}       {{/* string replace */}}
{{ .PR.Body | nindent 4 }}                 {{/* newline + indent */}}
```

### List Functions

```
{{ .PR.Labels | first }}                   {{/* first element */}}
{{ .PR.Labels | last }}                    {{/* last element */}}
{{ .PR.ChangedFiles | len }}               {{/* count elements */}}
{{ has "bug" .PR.Labels }}                 {{/* check if list contains */}}
{{ .PR.Labels | uniq }}                    {{/* remove duplicates */}}
{{ .PR.Labels | sortAlpha }}               {{/* sort alphabetically */}}
```

### Conditionals

```
{{ if .Guidelines }}
## Guidelines
{{ .Guidelines }}
{{ end }}

{{ if gt (len .PR.ChangedFiles) 10 }}
⚠️ Large PR with {{ len .PR.ChangedFiles }} files
{{ end }}

{{ ternary "many" "few" (gt (len .PR.ChangedFiles) 5) }}
```

### Default Values

```
{{ .Vars.maxIssues | default 10 }}         {{/* default if empty */}}
{{ .PR.Body | default "No description" }}
```

### Dictionaries

```
{{/* Create a dict to pass to templates */}}
{{ template "response-requirements" dict "maxComments" 50 "strict" true }}

{{/* Access custom vars */}}
{{ .Vars.customField }}
```

### Date/Time

```
{{ now | date "2006-01-02" }}              {{/* current date */}}
{{ now | dateModify "-24h" }}              {{/* yesterday */}}
```

### Encoding

```
{{ .PR.Body | b64enc }}                    {{/* base64 encode */}}
{{ .SomeB64 | b64dec }}                    {{/* base64 decode */}}
{{ .Data | toJson }}                       {{/* convert to JSON */}}
{{ .Data | toPrettyJson }}                 {{/* pretty JSON */}}
```

## Custom Functions

In addition to sprig, these custom functions are available:

| Function | Description |
|----------|-------------|
| `file "path"` | Get contents of an extra file by path |
| `indentBlock N "text"` | Indent all lines (including empty) by N spaces |

### Examples

```
{{/* Get contents of a specific extra file */}}
{{ file "docs/CONTRIBUTING.md" }}

{{/* Indent a block of text */}}
{{ .Guidelines | indentBlock 4 }}
```

## Action Inputs

| Input | Description | Default |
|-------|-------------|---------|
| `prompt_template_path` | Path to template file in repo | (none) |
| `prompt_template_engine` | Template engine | `go-template` |
| `prompt_template_vars_json` | JSON object of custom variables | `{}` |
| `prompt_template_max_bytes` | Max bytes to read from template | `200000` |
| `tool_input_mode` | What to send: `pr_context`, `prompt_text`, or `both` | `pr_context` |

## Complete Example

Here's a full prompt template that uses fragments and sprig functions:

```
{{/* .github/prompts/full-review.tmpl */}}

{{ template "system-role" . }}

You are reviewing PR #{{ .PR.Number }} in {{ .PR.Owner }}/{{ .PR.Repo }}.

{{ if .Guidelines -}}
## Repository Guidelines

{{ .Guidelines }}

{{ end -}}

## Pull Request

**Title:** {{ .PR.Title }}
**Author:** {{ .PR.UserLogin }}
**Branch:** {{ .PR.HeadRef }} → {{ .PR.BaseRef }}
{{- if .PR.Labels }}
**Labels:** {{ .PR.Labels | join ", " }}
{{- end }}
{{- if .PR.Assignees }}
**Assignees:** {{ .PR.Assignees | join ", " }}
{{- end }}

{{ if .PR.Body -}}
### Description

{{ .PR.Body }}

{{ end -}}

## Changed Files ({{ len .PR.ChangedFiles }})

{{ range .PR.ChangedFiles -}}
### {{ .Path }}

- **Status:** {{ .Status }}
- **Changes:** +{{ .Additions }} / -{{ .Deletions }}

{{ if .Patch -}}
```diff
{{ .Patch }}
```
{{ end }}

{{ end }}

{{ template "review-output-format" . }}

{{ template "review-guidelines" . }}

{{ template "response-requirements" dict "maxComments" (.Vars.maxComments | default 30) }}

Focus on: {{ .Vars.focus | default "security, correctness, performance, maintainability" }}

Output ONLY the JSON object, no other text.
```

Configure with custom variables:

```yaml
- uses: go-go-golems/go-go-agent-action@v1
  with:
    prompt_template_path: .github/prompts/full-review.tmpl
    prompt_template_vars_json: |
      {
        "maxComments": 20,
        "focus": "security vulnerabilities and API design"
      }
    tool_input_mode: prompt_text
```

## ReviewResult Output Format

Your review tool must return JSON matching the `ReviewResult` schema. See the embedded `review-output-format` fragment or [`internal/action/types.go`](../internal/action/types.go) for the full specification.

```json
{
  "summary_markdown": "### Review Summary\n...",
  "review_decision": "APPROVE" | "COMMENT" | "REQUEST_CHANGES",
  "review_body": "Main review text",
  "issue_comment": "Optional timeline comment",
  "comments": [
    {
      "path": "file.go",
      "body": "Comment text",
      "line": 42,
      "side": "RIGHT"
    }
  ]
}
```

## Tips

1. **Start with fragments** – Use the embedded fragments to ensure correct output format
2. **Test locally** – Use `tool_mode: mock` to validate template rendering
3. **Keep prompts focused** – Large prompts may hit token limits
4. **Use `tool_input_mode: prompt_text`** – When sending rendered prompts to LLMs
5. **Pass custom vars** – Use `prompt_template_vars_json` for configurable behavior
