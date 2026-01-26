package templating

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/go-go-golems/go-go-agent-action/internal/action"
)

type TemplateFile struct {
	Path     string
	Contents string
}

type TemplateData struct {
	PR           *action.PRContext
	Guidelines   string
	ExtraFiles   []TemplateFile
	ExtraFileMap map[string]string
	Vars         map[string]any
}

// RenderPrompt loads and renders the prompt template when configured.
func RenderPrompt(in *action.Inputs, env action.RuntimeEnv, pr *action.PRContext, readFile action.FileLoader) (string, *action.PromptMeta, error) {
	if in == nil || in.PromptTemplatePath == "" {
		return "", nil, nil
	}
	if readFile == nil {
		return "", nil, fmt.Errorf("readFile is required")
	}
	engine := strings.ToLower(strings.TrimSpace(in.PromptTemplateEngine))
	if engine == "" {
		engine = "go-template"
	}
	if engine != "go-template" {
		return "", nil, fmt.Errorf("unsupported prompt_template_engine %q", in.PromptTemplateEngine)
	}

	content, err := readWorkspaceFile(env.Workspace, in.PromptTemplatePath, in.PromptTemplateMaxBytes, readFile)
	if err != nil {
		return "", nil, err
	}

	guidelines, _ := decodeB64(pr.GuidelinesB64)
	extraFiles := make([]TemplateFile, 0, len(pr.ExtraFiles))
	extraMap := make(map[string]string, len(pr.ExtraFiles))
	for _, f := range pr.ExtraFiles {
		decoded, _ := decodeB64(f.ContentsB64)
		extraFiles = append(extraFiles, TemplateFile{Path: f.Path, Contents: decoded})
		extraMap[f.Path] = decoded
	}

	data := TemplateData{
		PR:           pr,
		Guidelines:   guidelines,
		ExtraFiles:   extraFiles,
		ExtraFileMap: extraMap,
		Vars:         in.PromptTemplateVars,
	}

	tmpl, err := template.New("prompt").Option("missingkey=zero").Funcs(template.FuncMap{
		"file": func(path string) string {
			return extraMap[path]
		},
		"indent": func(spaces int, input string) string {
			if spaces <= 0 || input == "" {
				return input
			}
			pad := strings.Repeat(" ", spaces)
			lines := strings.Split(input, "\n")
			for i, line := range lines {
				lines[i] = pad + line
			}
			return strings.Join(lines, "\n")
		},
	}).Parse(content)
	if err != nil {
		return "", nil, fmt.Errorf("parse prompt template: %w", err)
	}

	var out strings.Builder
	if err := tmpl.Execute(&out, data); err != nil {
		return "", nil, fmt.Errorf("render prompt template: %w", err)
	}

	meta := &action.PromptMeta{
		TemplatePath: in.PromptTemplatePath,
		Engine:       engine,
		RenderedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	return out.String(), meta, nil
}

func readWorkspaceFile(workspace, rel string, limit int, readFile action.FileLoader) (string, error) {
	path := filepath.Join(workspace, rel)
	data, err := readFile(path)
	if err != nil {
		return "", fmt.Errorf("read prompt template %q: %w", rel, err)
	}
	if limit > 0 && len(data) > limit {
		data = data[:limit]
	}
	return string(data), nil
}

func decodeB64(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}
