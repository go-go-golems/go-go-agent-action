package action

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/go-go-golems/go-go-agent-action/prompts"
)

type TemplateFile struct {
	Path     string
	Contents string
}

type TemplateData struct {
	PR           *PRContext
	Guidelines   string
	ExtraFiles   []TemplateFile
	ExtraFileMap map[string]string
	Vars         map[string]any
}

// RenderPrompt loads and renders the prompt template when configured.
func RenderPrompt(in *Inputs, env RuntimeEnv, pr *PRContext, readFile FileLoader) (string, *PromptMeta, error) {
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

	tmpl, err := buildTemplate(extraMap)
	if err != nil {
		return "", nil, err
	}

	tmpl, err = tmpl.Parse(content)
	if err != nil {
		return "", nil, fmt.Errorf("parse prompt template: %w", err)
	}

	var out strings.Builder
	if err := tmpl.Execute(&out, data); err != nil {
		return "", nil, fmt.Errorf("render prompt template: %w", err)
	}

	meta := &PromptMeta{
		TemplatePath: in.PromptTemplatePath,
		Engine:       engine,
		RenderedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	return out.String(), meta, nil
}

// buildTemplate creates a template with all functions and embedded fragments.
func buildTemplate(extraMap map[string]string) (*template.Template, error) {
	// Start with sprig functions
	funcs := sprig.TxtFuncMap()

	// Add our custom functions (may override sprig where needed)
	funcs["file"] = func(path string) string {
		return extraMap[path]
	}
	// Override sprig's indent to match our behavior if needed
	// (sprig's indent works differently - it doesn't indent empty lines)
	funcs["indentBlock"] = func(spaces int, input string) string {
		if spaces <= 0 || input == "" {
			return input
		}
		pad := strings.Repeat(" ", spaces)
		lines := strings.Split(input, "\n")
		for i, line := range lines {
			lines[i] = pad + line
		}
		return strings.Join(lines, "\n")
	}

	tmpl := template.New("prompt").Option("missingkey=zero").Funcs(funcs)

	// Load embedded fragments
	fragmentFiles, err := prompts.Fragments.ReadDir("fragments")
	if err != nil {
		return nil, fmt.Errorf("read embedded fragments: %w", err)
	}

	for _, f := range fragmentFiles {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		if !strings.HasSuffix(name, ".tmpl") {
			continue
		}
		content, err := prompts.Fragments.ReadFile("fragments/" + name)
		if err != nil {
			return nil, fmt.Errorf("read fragment %q: %w", name, err)
		}
		_, err = tmpl.Parse(string(content))
		if err != nil {
			return nil, fmt.Errorf("parse fragment %q: %w", name, err)
		}
	}

	return tmpl, nil
}

func readWorkspaceFile(workspace, rel string, limit int, readFile FileLoader) (string, error) {
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
