//go:build windows

package windows

import (
	"os"
	"path/filepath"
	"strings"
)

// Environment runs the Windows startup checks. Nothing blocks startup;
// every risk is a warning in Describe, which the startup log prints.
type Environment struct{}

// Check never blocks startup on Windows.
func (Environment) Check() error {
	return nil
}

// Describe reports the accepted degradations: private permissions are not
// enforced, and a data directory outside the user profile gets an extra
// warning. Each warning is bilingual: English first, then the Chinese
// explanation operators read.
func (Environment) Describe(dataDir string) []string {
	lines := []string{
		"platform: private file permissions are not enforced on Windows; secrets and backups rely on the user profile directory ACL",
		"platform：在 Windows 上不强制私有文件权限，secrets 与备份文件依赖用户配置文件目录的默认 ACL 保护；请勿将数据目录放在共享或同步目录中",
	}
	if dataDir != "" && !inUserProfile(dataDir) {
		lines = append(lines,
			"platform: data directory "+dataDir+" is outside the user profile; other local users may read platform secrets there",
			"platform：数据目录 "+dataDir+" 不在用户配置文件目录下，同机其他本地用户可能读取平台密钥，请将其移回 %APPDATA% 或 %LOCALAPPDATA% 下",
		)
	}
	return lines
}

// inUserProfile reports whether dir lives under %APPDATA% or
// %LOCALAPPDATA%, comparing case-insensitively with a separator boundary.
func inUserProfile(dir string) bool {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	lower := strings.ToLower(abs)
	for _, base := range []string{os.Getenv("APPDATA"), os.Getenv("LOCALAPPDATA")} {
		if base == "" {
			continue
		}
		if babs, err := filepath.Abs(base); err == nil {
			lowerBase := strings.ToLower(babs)
			if lower == lowerBase || strings.HasPrefix(lower, lowerBase+string(filepath.Separator)) {
				return true
			}
		}
	}
	return false
}
