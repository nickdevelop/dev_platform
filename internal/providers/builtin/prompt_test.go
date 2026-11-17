package builtin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPromptAssemblerTiersAndSkills(t *testing.T) {
	tmpDir := t.TempDir()

	dataDir := filepath.Join(tmpDir, "data")
	userHome := filepath.Join(tmpDir, "user")
	projectDir := filepath.Join(tmpDir, "project")

	for _, p := range []string{
		dataDir,
		filepath.Join(userHome, ".agents"),
		projectDir,
		filepath.Join(projectDir, ".skills", "my-skill"),
	} {
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Write Platform AGENTS.md
	if err := os.WriteFile(filepath.Join(dataDir, "AGENTS.md"), []byte("Platform level rule 1"), 0644); err != nil {
		t.Fatal(err)
	}
	// Write User AGENTS.md
	if err := os.WriteFile(filepath.Join(userHome, ".agents", "AGENTS.md"), []byte("User level rule 2"), 0644); err != nil {
		t.Fatal(err)
	}
	// Write Project AGENTS.md with UTF-8 BOM
	bomBytes := append([]byte("\xef\xbb\xbf"), []byte("Project level rule 3")...)
	if err := os.WriteFile(filepath.Join(projectDir, "AGENTS.md"), bomBytes, 0644); err != nil {
		t.Fatal(err)
	}

	// Write Project Skill
	skillContent := `---
name: my-skill
description: A helpful test skill
---
# Skill instructions`
	if err := os.WriteFile(filepath.Join(projectDir, ".skills", "my-skill", "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatal(err)
	}

	fixedTime := time.Date(2026, 3, 29, 15, 30, 0, 0, time.UTC)
	pa := &promptAssembler{
		dataDir: dataDir,
		userHomeFn: func() (string, error) {
			return userHome, nil
		},
		nowFn: func() time.Time {
			return fixedTime
		},
	}

	prompt := pa.assemble(context.Background(), projectDir)

	// 1. Verify persona
	if !strings.Contains(prompt, "你是 dev_platform 平台的内置智能编码 Agent") {
		t.Errorf("missing persona in prompt")
	}

	// 2. Verify Platform prompt
	if !strings.Contains(prompt, "Platform level rule 1") {
		t.Errorf("missing platform rule")
	}

	// 3. Verify User prompt
	if !strings.Contains(prompt, "User level rule 2") {
		t.Errorf("missing user rule")
	}

	// 4. Verify Project prompt (and BOM stripped)
	if !strings.Contains(prompt, "Project level rule 3") {
		t.Errorf("missing project rule")
	}
	if strings.Contains(prompt, "\xef\xbb\xbf") {
		t.Errorf("BOM was not stripped from project rule")
	}

	// 5. Verify skills XML
	if !strings.Contains(prompt, "<available_skills>") || !strings.Contains(prompt, "<name>my-skill</name>") {
		t.Errorf("missing skills XML in prompt:\n%s", prompt)
	}

	// 6. Verify temporal & OS brief
	if !strings.Contains(prompt, "2026-03-29 15:30:00 (Sunday, UTC)") {
		t.Errorf("missing temporal string in prompt")
	}
	if !strings.Contains(prompt, "操作系统环境：") {
		t.Errorf("missing OS info in prompt")
	}
}

func TestPromptAssemblerPartialFiles(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Only project AGENTS.md exists
	if err := os.WriteFile(filepath.Join(projectDir, "AGENTS.md"), []byte("Only project rules"), 0644); err != nil {
		t.Fatal(err)
	}

	pa := &promptAssembler{
		dataDir: filepath.Join(tmpDir, "nonexistent-data"),
		userHomeFn: func() (string, error) {
			return filepath.Join(tmpDir, "nonexistent-home"), nil
		},
		nowFn: time.Now,
	}

	prompt := pa.assemble(context.Background(), projectDir)

	if strings.Contains(prompt, "Platform AGENTS.md") {
		t.Errorf("expected no platform prompt")
	}
	if strings.Contains(prompt, "User AGENTS.md") {
		t.Errorf("expected no user prompt")
	}
	if !strings.Contains(prompt, "Only project rules") {
		t.Errorf("expected project prompt to be included")
	}
}
