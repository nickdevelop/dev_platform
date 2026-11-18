package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DiscoveryPaths defines the candidate search directories across the three tiers.
type DiscoveryPaths struct {
	ProjectSkillsDir string // <projectDir>/.skills
	UserSkillsDir    string // ~/.agents/skills
	DataSkillsDir    string // <dataDir>/skills
}

// LoadSkills scans the three tiers in priority order:
// ProjectSkillsDir > UserSkillsDir > DataSkillsDir.
// If multiple skills share the same Name, higher priority tiers override lower ones.
func LoadSkills(paths DiscoveryPaths) []Skill {
	seen := make(map[string]bool)
	var result []Skill

	scanTier := func(dir string) {
		if strings.TrimSpace(dir) == "" {
			return
		}
		skills := ScanSkillsDir(dir)
		for _, s := range skills {
			if seen[s.Name] {
				// Already provided by a higher priority tier; skip
				continue
			}
			seen[s.Name] = true
			result = append(result, s)
		}
	}

	// 1. Project-level (.skills in project workspace)
	scanTier(paths.ProjectSkillsDir)
	// 2. User-level (~/.agents/skills)
	scanTier(paths.UserSkillsDir)
	// 3. Platform-level (dataDir/skills)
	scanTier(paths.DataSkillsDir)

	return result
}

// FindSkillByName finds a skill by case-insensitive name from a loaded skill slice.
func FindSkillByName(skills []Skill, name string) *Skill {
	clean := strings.ToLower(strings.TrimSpace(name))
	for i := range skills {
		if strings.ToLower(skills[i].Name) == clean {
			return &skills[i]
		}
	}
	return nil
}

// ResolveSkill loads available skills from the paths and finds one matching name.
func ResolveSkill(paths DiscoveryPaths, name string) (*Skill, error) {
	skills := LoadSkills(paths)
	s := FindSkillByName(skills, name)
	if s == nil {
		return nil, fmt.Errorf("skill %q not found", name)
	}
	return s, nil
}

// ScanSkillsDir scans a single directory for skills according to Agent Skills conventions:
// - A subdirectory containing SKILL.md is treated as a skill root and not recursed further.
// - Direct *.md files in the directory with valid frontmatter are treated as standalone skills.
// - Dotfiles, dotfolders (e.g. .git), and node_modules are ignored.
func ScanSkillsDir(dir string) []Skill {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var found []Skill
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" {
			continue
		}

		fullPath := filepath.Join(dir, name)
		if entry.IsDir() {
			// Check for SKILL.md or skill.md inside the subdirectory
			skillFile := filepath.Join(fullPath, "SKILL.md")
			if fi, err := os.Stat(skillFile); err == nil && !fi.IsDir() {
				if skill, err := ParseSkillFile(skillFile); err == nil {
					found = append(found, *skill)
					continue
				}
			}
			skillFileLower := filepath.Join(fullPath, "skill.md")
			if fi, err := os.Stat(skillFileLower); err == nil && !fi.IsDir() {
				if skill, err := ParseSkillFile(skillFileLower); err == nil {
					found = append(found, *skill)
					continue
				}
			}
			continue
		}

		// Direct .md file
		if strings.HasSuffix(strings.ToLower(name), ".md") && !strings.EqualFold(name, "SKILL.md") {
			if skill, err := ParseSkillFile(fullPath); err == nil {
				found = append(found, *skill)
			}
		}
	}

	return found
}
