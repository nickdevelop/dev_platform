package skills

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
)

var validNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Skill represents a loaded skill matching the Agent Skills specification.
type Skill struct {
	Name                   string `json:"name"`
	Description            string `json:"description"`
	FilePath               string `json:"filePath"`
	BaseDir                string `json:"baseDir"`
	DisableModelInvocation bool   `json:"disableModelInvocation"`
}

// frontmatter defines the metadata fields extracted from the YAML header.
type frontmatter struct {
	Name                   string `yaml:"name"`
	Description            string `yaml:"description"`
	DisableModelInvocation bool   `yaml:"disable-model-invocation"`
}

// ParseSkillFile reads a markdown file and extracts skill metadata from its YAML frontmatter.
func ParseSkillFile(path string) (*Skill, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read skill file %q: %w", path, err)
	}

	// Strip UTF-8 BOM if present
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))

	fm, err := extractFrontmatter(raw)
	if err != nil {
		return nil, err
	}

	description := strings.TrimSpace(fm.Description)
	if description == "" {
		return nil, fmt.Errorf("skill in %q missing description", path)
	}

	baseDir := filepath.Dir(path)
	name := strings.TrimSpace(fm.Name)
	if name == "" {
		// Fallback to parent directory name if file is SKILL.md, otherwise file base name
		baseName := filepath.Base(path)
		if strings.EqualFold(baseName, "SKILL.md") {
			name = filepath.Base(baseDir)
		} else {
			name = strings.TrimSuffix(baseName, filepath.Ext(baseName))
		}
	}
	name = sanitizeName(name)
	if !validNamePattern.MatchString(name) {
		return nil, fmt.Errorf("invalid skill name %q in %q: must match lowercase letters, digits, and hyphens", name, path)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		absBaseDir = baseDir
	}

	return &Skill{
		Name:                   name,
		Description:            description,
		FilePath:               absPath,
		BaseDir:                absBaseDir,
		DisableModelInvocation: fm.DisableModelInvocation,
	}, nil
}

// extractFrontmatter extracts and parses YAML between leading `---` separators.
func extractFrontmatter(data []byte) (frontmatter, error) {
	var fm frontmatter
	trimmed := bytes.TrimLeft(data, " \r\n\t")
	if !bytes.HasPrefix(trimmed, []byte("---")) {
		return fm, fmt.Errorf("missing leading frontmatter delimiter")
	}

	// Skip the opening "---"
	afterOpening := trimmed[3:]
	// Must have a newline after opening delimiter
	if len(afterOpening) == 0 || (afterOpening[0] != '\n' && afterOpening[0] != '\r') {
		return fm, fmt.Errorf("invalid frontmatter opening delimiter")
	}

	// Find the closing "\n---"
	idx := bytes.Index(afterOpening, []byte("\n---"))
	if idx < 0 {
		return fm, fmt.Errorf("missing closing frontmatter delimiter")
	}

	yamlContent := afterOpening[:idx]
	if err := yaml.Unmarshal(yamlContent, &fm); err != nil {
		return fm, fmt.Errorf("parse YAML frontmatter: %w", err)
	}

	return fm, nil
}

// ReadSkillInstructions reads the markdown body of a skill file, stripping the YAML frontmatter.
func ReadSkillInstructions(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read skill file %q: %w", path, err)
	}
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	trimmed := bytes.TrimLeft(raw, " \r\n\t")
	if !bytes.HasPrefix(trimmed, []byte("---")) {
		return strings.TrimSpace(string(raw)), nil
	}
	afterOpening := trimmed[3:]
	if len(afterOpening) == 0 || (afterOpening[0] != '\n' && afterOpening[0] != '\r') {
		return strings.TrimSpace(string(raw)), nil
	}
	idx := bytes.Index(afterOpening, []byte("\n---"))
	if idx < 0 {
		return strings.TrimSpace(string(raw)), nil
	}
	body := afterOpening[idx+len("\n---"):]
	return strings.TrimSpace(string(body)), nil
}

// BuildSkillInvocationPrompt formats a user turn that explicitly invokes a skill via slash command.
func BuildSkillInvocationPrompt(skill *Skill, instructions string, userArgs string) string {
	taskText := strings.TrimSpace(userArgs)
	if taskText == "" {
		taskText = "请激活并根据该技能的指导原则开展工作。"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<invoked_skill name=%q location=%q>\n", skill.Name, skill.FilePath))
	if strings.TrimSpace(instructions) != "" {
		sb.WriteString("<instructions>\n")
		sb.WriteString(strings.TrimSpace(instructions))
		sb.WriteString("\n</instructions>\n")
	}
	sb.WriteString("<task>\n")
	sb.WriteString(taskText)
	sb.WriteString("\n</task>\n")
	sb.WriteString("</invoked_skill>")
	return sb.String()
}

// sanitizeName normalizes candidate skill names to lowercase alphanumeric with hyphens.
func sanitizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// Replace spaces or underscores with hyphens
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

// FormatSkillsForPrompt formats the available skills as standard XML for system prompt injection.
// Skills with DisableModelInvocation=true are excluded.
func FormatSkillsForPrompt(skills []Skill) string {
	visible := make([]Skill, 0, len(skills))
	for _, s := range skills {
		if !s.DisableModelInvocation {
			visible = append(visible, s)
		}
	}
	if len(visible) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("The following skills provide specialized instructions for specific tasks.\n")
	sb.WriteString("Use the read_file tool to load a skill's file when the task matches its description.\n")
	sb.WriteString("Resolve relative paths against the skill's directory (parent of SKILL.md). Use absolute paths in tool calls.\n\n")
	sb.WriteString("<available_skills>\n")

	for _, s := range visible {
		sb.WriteString("  <skill>\n")
		sb.WriteString(fmt.Sprintf("    <name>%s</name>\n", escapeXML(s.Name)))
		sb.WriteString(fmt.Sprintf("    <description>%s</description>\n", escapeXML(s.Description)))
		sb.WriteString(fmt.Sprintf("    <location>%s</location>\n", escapeXML(s.FilePath)))
		sb.WriteString("  </skill>\n")
	}

	sb.WriteString("</available_skills>")
	return sb.String()
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}
