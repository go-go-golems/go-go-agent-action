package action

import (
	"fmt"
	"strings"
)

type diaryRenderMode int

const (
	diaryRenderBrief diaryRenderMode = iota
	diaryRenderFull
)

func formatDiarySection(diary *AgentDiary, mode diaryRenderMode) string {
	if diary == nil {
		return ""
	}
	var b strings.Builder
	title := "Agent diary"
	if mode == diaryRenderFull {
		title = "Agent diary (full)"
	}

	b.WriteString("<details>\n")
	b.WriteString(fmt.Sprintf("<summary>%s</summary>\n\n", title))

	if strings.TrimSpace(diary.Summary) != "" {
		b.WriteString(diary.Summary)
		b.WriteString("\n\n")
	}

	if mode == diaryRenderBrief {
		writeDiaryQuickStats(&b, diary)
		b.WriteString("\n</details>")
		return b.String()
	}

	writeDiaryFiles(&b, diary)
	writeDiaryList(&b, "What I looked for", diary.WhatILookedFor)
	writeDiaryFindings(&b, diary.WhatIFound)
	writeDiaryUncertainties(&b, diary.Uncertainties)
	writeDiaryList(&b, "What I learned", diary.WhatILearned)
	writeDiaryList(&b, "Tricky to analyze", diary.TrickyToAnalyze)
	writeDiaryList(&b, "Second pair of eyes", diary.SecondPairOfEyes)
	writeDiaryList(&b, "Future work", diary.FutureWork)

	if strings.TrimSpace(diary.ReviewInstructions) != "" {
		b.WriteString("#### Review instructions\n\n")
		b.WriteString(diary.ReviewInstructions)
		b.WriteString("\n\n")
	}
	if strings.TrimSpace(diary.TechnicalDetails) != "" {
		b.WriteString("#### Technical details\n\n")
		b.WriteString(diary.TechnicalDetails)
		b.WriteString("\n\n")
	}

	b.WriteString("</details>")
	return b.String()
}

func writeDiaryQuickStats(b *strings.Builder, diary *AgentDiary) {
	stats := []string{}
	if len(diary.FilesExamined) > 0 {
		stats = append(stats, fmt.Sprintf("Files examined: %d", len(diary.FilesExamined)))
	}
	if len(diary.WhatIFound) > 0 {
		stats = append(stats, fmt.Sprintf("Findings: %d", len(diary.WhatIFound)))
	}
	if len(diary.Uncertainties) > 0 {
		stats = append(stats, fmt.Sprintf("Uncertainties: %d", len(diary.Uncertainties)))
	}
	if len(stats) == 0 {
		stats = append(stats, "No additional diary details provided.")
	}
	for _, stat := range stats {
		fmt.Fprintf(b, "- %s\n", stat)
	}
}

func writeDiaryFiles(b *strings.Builder, diary *AgentDiary) {
	if len(diary.FilesExamined) == 0 {
		return
	}
	b.WriteString("#### Files examined\n\n")
	for _, file := range diary.FilesExamined {
		path := strings.TrimSpace(file.Path)
		summary := strings.TrimSpace(file.Summary)
		line := ""
		if path != "" && summary != "" {
			line = fmt.Sprintf("- `%s` - %s\n", path, summary)
		} else if path != "" {
			line = fmt.Sprintf("- `%s`\n", path)
		} else if summary != "" {
			line = fmt.Sprintf("- %s\n", summary)
		}
		if line != "" {
			b.WriteString(line)
		}
		if len(file.PatternsChecked) > 0 {
			b.WriteString("  - Patterns checked:\n")
			for _, pattern := range file.PatternsChecked {
				pattern = strings.TrimSpace(pattern)
				if pattern == "" {
					continue
				}
				fmt.Fprintf(b, "    - %s\n", pattern)
			}
		}
	}
	b.WriteString("\n")
}

func writeDiaryList(b *strings.Builder, title string, entries []string) {
	entries = filterBlank(entries)
	if len(entries) == 0 {
		return
	}
	fmt.Fprintf(b, "#### %s\n\n", title)
	for _, entry := range entries {
		fmt.Fprintf(b, "- %s\n", entry)
	}
	b.WriteString("\n")
}

func writeDiaryFindings(b *strings.Builder, findings []DiaryFinding) {
	if len(findings) == 0 {
		return
	}
	b.WriteString("#### What I found\n\n")
	for _, finding := range findings {
		desc := strings.TrimSpace(finding.Description)
		if desc == "" {
			continue
		}
		prefix := strings.TrimSpace(finding.Type)
		if prefix != "" {
			prefix = fmt.Sprintf("[%s] ", prefix)
		}
		location := formatDiaryLocation(finding.File, finding.Line)
		if location != "" {
			b.WriteString(fmt.Sprintf("- %s%s (%s)\n", prefix, desc, location))
		} else {
			b.WriteString(fmt.Sprintf("- %s%s\n", prefix, desc))
		}
	}
	b.WriteString("\n")
}

func writeDiaryUncertainties(b *strings.Builder, uncertainties []DiaryUncertainty) {
	if len(uncertainties) == 0 {
		return
	}
	b.WriteString("#### Uncertainties\n\n")
	for _, entry := range uncertainties {
		desc := strings.TrimSpace(entry.Description)
		if desc == "" {
			continue
		}
		reason := strings.TrimSpace(entry.Reason)
		location := formatDiaryLocation(entry.File, entry.Line)
		switch {
		case reason != "" && location != "":
			b.WriteString(fmt.Sprintf("- %s (reason: %s, %s)\n", desc, reason, location))
		case reason != "":
			b.WriteString(fmt.Sprintf("- %s (reason: %s)\n", desc, reason))
		case location != "":
			b.WriteString(fmt.Sprintf("- %s (%s)\n", desc, location))
		default:
			b.WriteString(fmt.Sprintf("- %s\n", desc))
		}
	}
	b.WriteString("\n")
}

func formatDiaryLocation(file string, line int) string {
	file = strings.TrimSpace(file)
	if file == "" {
		return ""
	}
	if line > 0 {
		return fmt.Sprintf("%s:%d", file, line)
	}
	return file
}

func filterBlank(entries []string) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			out = append(out, entry)
		}
	}
	return out
}

func appendDiaryMarkdown(base, diarySection string) string {
	if strings.TrimSpace(diarySection) == "" {
		return base
	}
	if strings.TrimSpace(base) == "" {
		return diarySection
	}
	return strings.TrimSpace(base) + "\n\n" + diarySection
}
