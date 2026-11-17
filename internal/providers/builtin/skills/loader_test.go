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

	skillMap := make(map[string]Skill)
	for _, s := range skills {
		skillMap[s.Name] = s
	}

	// shared-tool should be from project!
	if s, ok := skillMap["shared-tool"]; !ok {
		t.Errorf("missing shared-tool")
	} else if s.Description != "shared tool from project" {
		t.Errorf("expected shared-tool from project, got %q", s.Description)
	}

	// project-only, user-only, platform-only should all exist
	if _, ok := skillMap["project-only"]; !ok {
		t.Errorf("missing project-only")
	}
	if _, ok := skillMap["user-only"]; !ok {
		t.Errorf("missing user-only")
	}
	if _, ok := skillMap["platform-only"]; !ok {
		t.Errorf("missing platform-only")
	}

	// Test XML formatting
	xml := FormatSkillsForPrompt(skills)
	if !strings.Contains(xml, "<available_skills>") {
		t.Errorf("expected <available_skills> in xml, got:\n%s", xml)
	}
	if !strings.Contains(xml, "<name>shared-tool</name>") {
		t.Errorf("expected shared-tool in xml")
	}
}

func writeSkill(t *testing.T, parentDir, name, desc string) {
	t.Helper()
	dir := filepath.Join(parentDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n# Body"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
