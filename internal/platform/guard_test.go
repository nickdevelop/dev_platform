package platform

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// allowlist 将已知的 OS 耦合文件映射到负责消除它的任务编号。
// 清单只能缩短：任务完成后必须删除对应条目；新增违规会直接失败。
// 测试文件（_test.go）不在扫描范围内。
var osGuardAllowlist = map[string]string{
	"internal/projects/trash_linux.go": "Task 2.5: trash 移入 platform/unix，业务包只引用 platform.ErrTrashUnavailable",
	"cmd/platform/main.go":             "Task 2.7: signal.NotifyContext 的 SIGINT/SIGTERM 移入 Shutdown.Signals 端口",
}

// TestBusinessPackagesStayOSNeutral 断言业务包保持 OS 中立：
// 不得 import syscall 或 golang.org/x/sys/*，不得出现 runtime.GOOS/GOARCH 判断。
// 平台差异只能通过 internal/platform 的接口或 build tag 文件表达。
func TestBusinessPackagesStayOSNeutral(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate guard test file")
	}
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	scanRoots := []string{
		filepath.Join(repoRoot, "internal"),
		filepath.Join(repoRoot, "cmd"),
	}

	type violation struct {
		file   string // 相对仓库根的斜杠路径
		detail string
	}
	var violations []violation
	seen := map[string]bool{}

	fset := token.NewFileSet()
	for _, root := range scanRoots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(info.Name(), ".go") || strings.HasSuffix(info.Name(), "_test.go") {
				return nil
			}
			rel, err := filepath.Rel(repoRoot, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if strings.HasPrefix(rel, "internal/platform/") {
				return nil
			}
			seen[rel] = true

			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", rel, err)
			}
			// import 检查：syscall 与 golang.org/x/sys/*。
			for _, imp := range file.Imports {
				impPath, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					continue
				}
				if impPath == "syscall" || impPath == "golang.org/x/sys" || strings.HasPrefix(impPath, "golang.org/x/sys/") {
					violations = append(violations, violation{rel, "imports " + impPath})
				}
			}
			// runtime.GOOS / runtime.GOARCH 检查：找出 import "runtime" 的本地名再匹配选择表达式。
			runtimeNames := map[string]bool{}
			dotRuntime := false
			for _, imp := range file.Imports {
				impPath, err := strconv.Unquote(imp.Path.Value)
				if err != nil || impPath != "runtime" {
					continue
				}
				if imp.Name != nil {
					switch imp.Name.Name {
					case "_":
						continue
					case ".":
						dotRuntime = true
					default:
						runtimeNames[imp.Name.Name] = true
					}
				} else {
					runtimeNames["runtime"] = true
				}
			}
			if len(runtimeNames) == 0 && !dotRuntime {
				return nil
			}
			ast.Inspect(file, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if ok {
					if id, ok := sel.X.(*ast.Ident); ok && runtimeNames[id.Name] &&
						(sel.Sel.Name == "GOOS" || sel.Sel.Name == "GOARCH") {
						violations = append(violations, violation{rel, "references runtime." + sel.Sel.Name})
					}
					return true
				}
				if dotRuntime {
					if id, ok := n.(*ast.Ident); ok && (id.Name == "GOOS" || id.Name == "GOARCH") {
						violations = append(violations, violation{rel, "references runtime." + id.Name})
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// 不在允许清单中的违规直接失败。
	var unexpected []violation
	for _, v := range violations {
		if _, ok := osGuardAllowlist[v.file]; !ok {
			unexpected = append(unexpected, v)
		}
	}
	if len(unexpected) > 0 {
		sort.Slice(unexpected, func(i, j int) bool { return unexpected[i].file < unexpected[j].file })
		for _, v := range unexpected {
			t.Errorf("%s: %s; OS 差异必须收敛到 internal/platform，不得在业务包中新增", v.file, v.detail)
		}
	}

	// 允许清单中已不再违规的条目必须删除（清单只能缩短）。
	violated := map[string]bool{}
	for _, v := range violations {
		violated[v.file] = true
	}
	var stale []string
	for file := range osGuardAllowlist {
		if !violated[file] {
			stale = append(stale, file)
		}
		if !seen[file] {
			t.Errorf("allowlist entry %s no longer exists; remove it from osGuardAllowlist", file)
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		for _, file := range stale {
			t.Errorf("allowlist entry %s no longer violates the guard; remove it from osGuardAllowlist (%s)", file, osGuardAllowlist[file])
		}
	}
}
