package action

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/go-github/v66/github"
)

type patchLineSet struct {
	left     map[int]struct{}
	right    map[int]struct{}
	hasPatch bool
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

func buildPatchLineSets(files []ChangedFile) map[string]*patchLineSet {
	sets := make(map[string]*patchLineSet, len(files))
	for _, file := range files {
		set := &patchLineSet{
			left:  map[int]struct{}{},
			right: map[int]struct{}{},
		}
		if strings.TrimSpace(file.Patch) == "" {
			sets[file.Path] = set
			continue
		}
		set.hasPatch = true
		var oldLine, newLine int
		lines := strings.Split(file.Patch, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "@@") {
				if oldStart, newStart, ok := parseHunkHeader(line); ok {
					oldLine = oldStart
					newLine = newStart
				}
				continue
			}
			if oldLine == 0 && newLine == 0 {
				continue
			}
			if strings.HasPrefix(line, "\\ No newline") {
				continue
			}
			if line == "" {
				continue
			}
			switch line[0] {
			case ' ':
				set.left[oldLine] = struct{}{}
				set.right[newLine] = struct{}{}
				oldLine++
				newLine++
			case '+':
				set.right[newLine] = struct{}{}
				newLine++
			case '-':
				set.left[oldLine] = struct{}{}
				oldLine++
			default:
				continue
			}
		}
		sets[file.Path] = set
	}
	return sets
}

func parseHunkHeader(line string) (int, int, bool) {
	matches := hunkHeader.FindStringSubmatch(line)
	if len(matches) < 3 {
		return 0, 0, false
	}
	oldStart, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, 0, false
	}
	newStart, err := strconv.Atoi(matches[2])
	if err != nil {
		return 0, 0, false
	}
	return oldStart, newStart, true
}

func normalizeSide(side string) string {
	if strings.EqualFold(side, "LEFT") {
		return "LEFT"
	}
	return "RIGHT"
}

func sanitizeReviewComments(pr *PRContext, comments []ReviewComment, reviewBody string, maxComments int) ([]ReviewComment, string, []string) {
	if pr == nil {
		return nil, reviewBody, []string{"no PR context; dropping all inline comments"}
	}
	lineSets := buildPatchLineSets(pr.ChangedFiles)
	var kept []ReviewComment
	var dropped []ReviewComment
	var warnings []string

	for i, comment := range comments {
		if maxComments > 0 && len(kept) >= maxComments {
			warnings = append(warnings, fmt.Sprintf("dropping comment[%d]: max_comments=%d reached", i, maxComments))
			dropped = append(dropped, comment)
			continue
		}

		if strings.EqualFold(comment.Subject, "file") {
			warnings = append(warnings, fmt.Sprintf("dropping comment[%d] path=%q: subject_type=file not supported for inline comments", i, comment.Path))
			dropped = append(dropped, comment)
			continue
		}

		if strings.TrimSpace(comment.Path) == "" {
			warnings = append(warnings, fmt.Sprintf("dropping comment[%d]: empty path", i))
			dropped = append(dropped, comment)
			continue
		}
		lineSet, ok := lineSets[comment.Path]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("dropping comment[%d] path=%q: not in changed files", i, comment.Path))
			dropped = append(dropped, comment)
			continue
		}
		if !lineSet.hasPatch {
			warnings = append(warnings, fmt.Sprintf("dropping comment[%d] path=%q: patch unavailable (include_patch required)", i, comment.Path))
			dropped = append(dropped, comment)
			continue
		}

		if comment.Line <= 0 {
			warnings = append(warnings, fmt.Sprintf("dropping comment[%d] path=%q: missing line number", i, comment.Path))
			dropped = append(dropped, comment)
			continue
		}

		side := normalizeSide(comment.Side)
		if side == "LEFT" {
			if _, ok := lineSet.left[comment.Line]; !ok {
				warnings = append(warnings, fmt.Sprintf("dropping comment[%d] %s:%d LEFT: line not in diff hunk", i, comment.Path, comment.Line))
				dropped = append(dropped, comment)
				continue
			}
		} else {
			if _, ok := lineSet.right[comment.Line]; !ok {
				warnings = append(warnings, fmt.Sprintf("dropping comment[%d] %s:%d RIGHT: line not in diff hunk", i, comment.Path, comment.Line))
				dropped = append(dropped, comment)
				continue
			}
		}

		startSide := normalizeSide(comment.StartSide)
		if comment.StartLine > 0 {
			if startSide == "LEFT" {
				if _, ok := lineSet.left[comment.StartLine]; !ok {
					warnings = append(warnings, fmt.Sprintf("dropping comment[%d] %s:%d LEFT: start_line not in diff hunk", i, comment.Path, comment.StartLine))
					dropped = append(dropped, comment)
					continue
				}
			} else {
				if _, ok := lineSet.right[comment.StartLine]; !ok {
					warnings = append(warnings, fmt.Sprintf("dropping comment[%d] %s:%d RIGHT: start_line not in diff hunk", i, comment.Path, comment.StartLine))
					dropped = append(dropped, comment)
					continue
				}
			}
			if comment.StartLine > comment.Line {
				warnings = append(warnings, fmt.Sprintf("dropping comment[%d] %s:%d: start_line greater than line", i, comment.Path, comment.StartLine))
				dropped = append(dropped, comment)
				continue
			}
			comment.StartSide = startSide
		} else if comment.StartSide != "" {
			warnings = append(warnings, fmt.Sprintf("dropping comment[%d] %s:%d: start_side set without start_line", i, comment.Path, comment.Line))
			dropped = append(dropped, comment)
			continue
		}

		comment.Side = side
		kept = append(kept, comment)
	}

	if len(dropped) > 0 {
		reviewBody = appendDroppedComments(reviewBody, dropped)
	}

	return kept, reviewBody, warnings
}

func appendDroppedComments(reviewBody string, dropped []ReviewComment) string {
	const maxNotes = 20
	var b strings.Builder
	if strings.TrimSpace(reviewBody) != "" {
		b.WriteString(reviewBody)
		b.WriteString("\n\n")
	}
	b.WriteString("### Notes (inline comments omitted)\n")
	for i, comment := range dropped {
		if i >= maxNotes {
			b.WriteString(fmt.Sprintf("- ...and %d more omitted comments\n", len(dropped)-maxNotes))
			break
		}
		loc := strings.TrimSpace(comment.Path)
		if comment.Line > 0 {
			loc = fmt.Sprintf("%s:%d", loc, comment.Line)
		}
		if loc == "" {
			loc = "(no path)"
		}
		body := strings.TrimSpace(comment.Body)
		if body == "" {
			body = "(no body)"
		}
		b.WriteString(fmt.Sprintf("- %s: %s\n", loc, body))
	}
	return b.String()
}

func summarizeDraftComments(comments []*github.DraftReviewComment) string {
	if len(comments) == 0 {
		return "none"
	}
	limit := 5
	if len(comments) < limit {
		limit = len(comments)
	}
	parts := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		c := comments[i]
		path := ""
		if c.Path != nil {
			path = *c.Path
		}
		line := 0
		if c.Line != nil {
			line = *c.Line
		}
		side := ""
		if c.Side != nil {
			side = *c.Side
		}
		parts = append(parts, fmt.Sprintf("%s:%d %s", path, line, side))
	}
	if len(comments) > limit {
		parts = append(parts, fmt.Sprintf("+%d more", len(comments)-limit))
	}
	return strings.Join(parts, "; ")
}
