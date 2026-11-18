package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSkillFile(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Valid directory skill with SKILL.md
	skillDir := filepath.Join(tmpDir, "plantuml")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	content := `---
name: plantuml-drawer
description: 用 PlantUML 出图的技能
disable-model-invocation: false
---
# PlantUML Skill Instructions
Detailed guidelines here...`
	if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	skill, err := ParseSkillFile(skillPath)
	if err != nil {
		t.Fatalf("unexpected error parsing valid skill: %v", err)
	}
	if skill.Name != "plantuml-drawer" {
		t.Errorf("expected name 'plantuml-drawer', got %q", skill.Name)
	}
	if skill.Description != "用 PlantUML 出图的技能" {
		t.Errorf("expected description, got %q", skill.Description)
	}
	if skill.DisableModelInvocation {
		t.Errorf("expected DisableModelInvocation false")
	}

	// 2. Fallback to directory name when name is empty
	skillDir2 := filepath.Join(tmpDir, "my-custom-tool")
	if err := os.MkdirAll(skillDir2, 0755); err != nil {
		t.Fatal(err)
	}
	skillPath2 := filepath.Join(skillDir2, "SKILL.md")
	content2 := `---
description: Custom tool description
---
Instructions...`
	if err := os.WriteFile(skillPath2, []byte(content2), 0644); err != nil {
		t.Fatal(err)
	}

	skill2, err := ParseSkillFile(skillPath2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skill2.Name != "my-custom-tool" {
		t.Errorf("expected name 'my-custom-tool', got %q", skill2.Name)
	}

	// 3. Invalid skill (missing description)
	badSkillPath := filepath.Join(tmpDir, "bad.md")
	if err := os.WriteFile(badSkillPath, []byte("---\nname: bad\n---\nbody"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSkillFile(badSkillPath); err == nil {
		t.Errorf("expected error for missing description, got nil")
	}
}

func TestLoadSkillsTiersAndPrecedence(t *testing.T) {
	tmpDir := t.TempDir()

	projectSkills := filepath.Join(tmpDir, "project", ".skills")
	userSkills := filepath.Join(tmpDir, "user", "skills")
	dataSkills := filepath.Join(tmpDir, "data", "skills")

	for _, p := range []string{projectSkills, userSkills, dataSkills} {
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// In platform: skill-shared (platform version), skill-platform-only
	writeSkill(t, dataSkills, "shared-tool", "shared tool from platform")
	writeSkill(t, dataSkills, "platform-only", "only in platform")

	// In user: skill-shared (user version, should override platform), skill-user-only
	writeSkill(t, userSkills, "shared-tool", "shared tool from user")
	writeSkill(t, userSkills, "user-only", "only in user")

	// In project: skill-shared (project version, should override user), skill-project-only
	writeSkill(t, projectSkills, "shared-tool", "shared tool from project")
	writeSkill(t, projectSkills, "project-only", "only in project")

	paths := DiscoveryPaths{
		ProjectSkillsDir: projectSkills,
		UserSkillsDir:    userSkills,
		DataSkillsDir:    dataSkills,
	}

	skills := LoadSkills(paths)
	if len(skills) != 4 {
		t.Fatalf("expected 4 unique skills, got %d", len(skills))
	}

	byName := make(map[string]Skill)
	for _, s := range skills {
		byName[s.Name] = s
	}

	if byName["shared-tool"].Description != "shared tool from project" {
		t.Errorf("expected project-level skill to override others, got %q", byName["shared-tool"].Description)
	}
	if byName["user-only"].Description != "only in user" {
		t.Errorf("expected user-only skill")
	}
	if byName["platform-only"].Description != "only in platform" {
		t.Errorf("expected platform-only skill")
	}
	if byName["project-only"].Description != "only in project" {
		t.Errorf("expected project-only skill")
	}

	// Test ResolveSkill & FindSkillByName
	resolved, err := ResolveSkill(paths, "shared-tool")
	if err != nil {
		t.Fatalf("ResolveSkill shared-tool: %v", err)
	}
	if resolved.Description != "shared tool from project" {
		t.Errorf("expected resolved skill description from project, got %q", resolved.Description)
	}

	if _, err := ResolveSkill(paths, "nonexistent"); err == nil {
		t.Errorf("expected error for nonexistent skill")
	}
}

func TestReadInstructionsAndBuildInvocation(t *testing.T) {
	tmpDir := t.TempDir()
	skillPath := filepath.Join(tmpDir, "SKILL.md")
	content := `---
name: test-skill
description: A test skill
---
# Main Instructions
Line 1
Line 2`
	if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	instructions, err := ReadSkillInstructions(skillPath)
	if err != nil {
		t.Fatalf("ReadSkillInstructions failed: %v", err)
	}
	if !strings.Contains(instructions, "# Main Instructions") || !strings.Contains(instructions, "Line 2") {
		t.Errorf("instructions missing expected content: %q", instructions)
	}
	if strings.Contains(instructions, "name: test-skill") {
		t.Errorf("instructions should have stripped YAML frontmatter: %q", instructions)
	}

	skill := &Skill{
		Name:     "test-skill",
		FilePath: skillPath,
	}

	promptWithArgs := BuildSkillInvocationPrompt(skill, instructions, "do something cool")
	if !strings.Contains(promptWithArgs, `<invoked_skill name="test-skill"`) ||
		!strings.Contains(promptWithArgs, "<instructions>\n# Main Instructions") ||
		!strings.Contains(promptWithArgs, "<task>\ndo something cool\n</task>") {
		t.Errorf("BuildSkillInvocationPrompt output unexpected:\n%s", promptWithArgs)
	}

	promptEmptyArgs := BuildSkillInvocationPrompt(skill, instructions, "")
	if !strings.Contains(promptEmptyArgs, "请激活并根据该技能的指导原则开展工作。") {
		t.Errorf("expected default task prompt, got:\n%s", promptEmptyArgs)
	}
}

func writeSkill(t *testing.T, dir, name, description string) {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\nbody"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
