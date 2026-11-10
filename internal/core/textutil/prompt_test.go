package textutil

import "testing"

func TestParsePromptBlockRole(t *testing.T) {
	text := "<Role name=\"代码审查\">\n请审查以下代码：\n</Role>\n\n修复登录问题"
	block, ok := ParsePromptBlock(text)
	if !ok {
		t.Fatal("ParsePromptBlock() = false, want a parsed role block")
	}
	if block.Kind != "role" {
		t.Errorf("Kind = %q, want role", block.Kind)
	}
	if block.Name != "代码审查" {
		t.Errorf("Name = %q, want 代码审查", block.Name)
	}
	if block.Body != "请审查以下代码：" {
		t.Errorf("Body = %q, want the trimmed prompt body", block.Body)
	}
	if block.Rest != "修复登录问题" {
		t.Errorf("Rest = %q, want the user's own input", block.Rest)
	}
}

func TestParsePromptBlockFlowAndEscapedName(t *testing.T) {
	text := "<Flow name=\"并行 &quot;评审&quot;\">\n先拆解再执行\n</Flow>\n\n开始"
	block, ok := ParsePromptBlock(text)
	if !ok {
		t.Fatal("ParsePromptBlock() = false, want a parsed flow block")
	}
	if block.Kind != "flow" {
		t.Errorf("Kind = %q, want flow", block.Kind)
	}
	if block.Name != `并行 "评审"` {
		t.Errorf("Name = %q, want the unescaped name", block.Name)
	}
	if block.Rest != "开始" {
		t.Errorf("Rest = %q, want 开始", block.Rest)
	}
}

func TestParsePromptBlockRejectsOrdinaryText(t *testing.T) {
	for _, text := range []string{
		"",
		"修复登录问题",
		"前言\n<Role name=\"x\">body</Role>",
		"<Role name=\"x\">body</Flow>",
	} {
		if _, ok := ParsePromptBlock(text); ok {
			t.Errorf("ParsePromptBlock(%q) = true, want false", text)
		}
	}
}

func TestDeriveSessionTitlePrefersUserInput(t *testing.T) {
	text := "<Role name=\"代码审查\">\n请审查以下代码\n</Role>\n\n帮我\n修复登录问题"
	if got := DeriveSessionTitle(text); got != "帮我 修复登录问题" {
		t.Errorf("DeriveSessionTitle() = %q, want the user's own input", got)
	}
}

func TestDeriveSessionTitleFallsBackToPromptName(t *testing.T) {
	text := "<Flow name=\"并行评审\">\n先拆解再执行\n</Flow>\n"
	if got := DeriveSessionTitle(text); got != "并行评审" {
		t.Errorf("DeriveSessionTitle() = %q, want the prompt name", got)
	}
}

func TestDeriveSessionTitleLeavesOrdinaryTextAlone(t *testing.T) {
	if got := DeriveSessionTitle("帮我\n部署日志系统"); got != "帮我 部署日志系统" {
		t.Errorf("DeriveSessionTitle() = %q, want the plain message title", got)
	}
}
