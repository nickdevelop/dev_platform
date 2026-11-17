package builtin

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"devplatform/internal/providers/builtin/skills"
)

// maxContextFileSize caps reading single AGENTS.md files to prevent huge prompt injection.
const maxContextFileSize = 256 * 1024 // 256 KB

// promptAssembler builds the full system prompt for a turn across all tiers.
type promptAssembler struct {
	dataDir     string
	globalFn    func(ctx context.Context) (string, error)
	userHomeFn  func() (string, error)
	nowFn       func() time.Time
}

func newPromptAssembler(dataDir string, globalFn func(ctx context.Context) (string, error), nowFn func() time.Time) *promptAssembler {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &promptAssembler{
		dataDir:    dataDir,
		globalFn:   globalFn,
		userHomeFn: os.UserHomeDir,
		nowFn:      nowFn,
	}
}

// assemble builds the multi-tiered system prompt for the given project directory.
func (pa *promptAssembler) assemble(ctx context.Context, projectDir string) string {
	parts := make([]string, 0, 7)

	// 1. Base agent persona
	parts = append(parts, "你是 dev_platform 平台的内置智能编码 Agent，直接操作用户的项目工作区。")

	// 2. Platform-level AGENTS.md (dataDir/AGENTS.md or via platform settings fallback)
	if platformPrompt := pa.loadPlatformPrompt(ctx); platformPrompt != "" {
		parts = append(parts, "## 平台全局规范 (Platform AGENTS.md)\n\n"+platformPrompt)
	}

	// 3. User-level AGENTS.md (~/.agents/AGENTS.md)
	if userPrompt := pa.loadUserPrompt(); userPrompt != "" {
		parts = append(parts, "## 个人偏好规范 (~/.agents/AGENTS.md)\n\n"+userPrompt)
	}

	// 4. Project-level AGENTS.md (<projectDir>/AGENTS.md)
	if projectPrompt := pa.loadProjectPrompt(projectDir); projectPrompt != "" {
		parts = append(parts, "## 项目约定规范 (Project AGENTS.md)\n\n"+projectPrompt)
	}

	// 5. Available skills (<available_skills>)
	loadedSkills := pa.loadSkills(projectDir)
	if skillsXML := skills.FormatSkillsForPrompt(loadedSkills); skillsXML != "" {
		parts = append(parts, skillsXML)
	}

	// 6. Workspace, Temporal & Runtime Environment Brief
	parts = append(parts, pa.workspaceBrief(projectDir))

	return strings.Join(parts, "\n\n---\n\n")
}

// loadPlatformPrompt loads the platform AGENTS.md from dataDir, falling back to the globalFn hook.
func (pa *promptAssembler) loadPlatformPrompt(ctx context.Context) string {
	if pa.dataDir != "" {
		candidate := filepath.Join(pa.dataDir, "AGENTS.md")
		if content, ok := readContextFile(candidate); ok {
			return content
		}
	}
	if pa.globalFn != nil {
		if prompt, err := pa.globalFn(ctx); err == nil && strings.TrimSpace(prompt) != "" {
			return strings.TrimSpace(prompt)
		}
	}
	return ""
}

// loadUserPrompt loads ~/.agents/AGENTS.md if available.
func (pa *promptAssembler) loadUserPrompt() string {
	if pa.userHomeFn == nil {
		return ""
	}
	home, err := pa.userHomeFn()
	if err != nil || home == "" {
		return ""
	}
	candidate := filepath.Join(home, ".agents", "AGENTS.md")
	content, ok := readContextFile(candidate)
	if !ok {
		return ""
	}
	return content
}

// loadProjectPrompt loads <projectDir>/AGENTS.md if available.
func (pa *promptAssembler) loadProjectPrompt(projectDir string) string {
	if strings.TrimSpace(projectDir) == "" {
		return ""
	}
	candidate := filepath.Join(projectDir, "AGENTS.md")
	content, ok := readContextFile(candidate)
	if !ok {
		return ""
	}
	return content
}

// loadSkills discovers skills from the three tiers: project, user, and dataDir.
func (pa *promptAssembler) loadSkills(projectDir string) []skills.Skill {
	paths := skills.DiscoveryPaths{}
	if strings.TrimSpace(projectDir) != "" {
		paths.ProjectSkillsDir = filepath.Join(projectDir, ".skills")
	}
	if pa.userHomeFn != nil {
		if home, err := pa.userHomeFn(); err == nil && home != "" {
			paths.UserSkillsDir = filepath.Join(home, ".agents", "skills")
		}
	}
	if pa.dataDir != "" {
		paths.DataSkillsDir = filepath.Join(pa.dataDir, "skills")
	}
	return skills.LoadSkills(paths)
}

// workspaceBrief composes environment information including time, OS, and operational guidelines.
func (pa *promptAssembler) workspaceBrief(projectDir string) string {
	now := pa.nowFn()
	timeStr := now.Format("2006-01-02 15:04:05 (Monday, MST)")
	osArch := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)

	return fmt.Sprintf(
		"当前工作目录：%s\n"+
			"操作系统环境：%s\n"+
			"当前系统时间：%s\n\n"+
			"你运行在 dev_platform 平台进程内，通过平台注入的工具读写该工作目录。遵循以下原则：\n"+
			"- 修改文件前先用 read_file 读取目标文件，避免盲目覆盖；edit_file 的 old_string 必须精确且唯一。\n"+
			"- 修改代码后按需运行项目自身的测试或构建命令验证。\n"+
			"- 工具长输出会被截断，重要信息请分步获取。\n"+
			"- 回复使用简体中文。",
		projectDir, osArch, timeStr,
	)
}

// readContextFile reads a single context file safely with size and BOM guards.
func readContextFile(path string) (string, bool) {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return "", false
	}
	if fi.Size() == 0 {
		return "", false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}

	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if len(data) > maxContextFileSize {
		data = data[:maxContextFileSize]
	}

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}
