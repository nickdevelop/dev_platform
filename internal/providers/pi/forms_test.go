package pi

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"devplatform/internal/core/agents"
)

func TestBuildQuestionnaireForm(t *testing.T) {
	items := []agents.StandardQuestionItem{
		{
			ID:       "q1",
			Header:   "连通测试",
			Question: "你现在能看到这题并选择一个选项吗？",
			Options: []agents.StandardQuestionOption{
				{Label: "我选这个，拿到回复了", Description: "说明问卷链路正常"},
				{Label: "还是自动取消", Description: "说明链路有异常"},
			},
			Multiple: false,
		},
		{
			ID:       "q2",
			Header:   "技术偏好",
			Question: "请选择你熟悉的技术栈？",
			Options: []agents.StandardQuestionOption{
				{Label: "Go", Description: "后端开发"},
				{Label: "TypeScript", Description: "前端开发"},
				{Label: "Python", Description: "算法开发"},
			},
			Multiple: true,
		},
	}

	formInfo, err := buildQuestionnaireForm("ses-1", "frm-101", items)
	if err != nil {
		t.Fatalf("buildQuestionnaireForm: %v", err)
	}

	if formInfo.ID != "frm-101" {
		t.Errorf("ID = %q, want %q", formInfo.ID, "frm-101")
	}
	if formInfo.Title != "连通测试" {
		t.Errorf("Title = %q, want %q", formInfo.Title, "连通测试")
	}
	if formInfo.SessionID != "ses-1" {
		t.Errorf("SessionID = %q, want %q", formInfo.SessionID, "ses-1")
	}

	var fields []formField
	if err := json.Unmarshal(formInfo.Fields, &fields); err != nil {
		t.Fatalf("unmarshal fields: %v", err)
	}
	if len(fields) != 2 {
		t.Fatalf("fields length = %d, want 2", len(fields))
	}

	// First field: single-select with custom option
	f1 := fields[0]
	if f1.Key != "q1" || f1.Type != "string" || !f1.Custom || len(f1.Options) != 2 {
		t.Errorf("f1 mismatch: %+v", f1)
	}
	if f1.Title != "连通测试" || f1.Description != "你现在能看到这题并选择一个选项吗？" {
		t.Errorf("f1 text mismatch: %+v", f1)
	}

	// Second field: multi-select
	f2 := fields[1]
	if f2.Key != "q2" || f2.Type != "multiselect" || len(f2.Options) != 3 {
		t.Errorf("f2 mismatch: %+v", f2)
	}
}

func TestFormatQuestionAnswer(t *testing.T) {
	singleItem := agents.StandardQuestionItem{
		Question: "单选问题",
		Options: []agents.StandardQuestionOption{
			{Label: "选项一"},
			{Label: "选项二"},
		},
		Multiple: false,
	}

	multiItem := agents.StandardQuestionItem{
		Question: "多选问题",
		Options: []agents.StandardQuestionOption{
			{Label: "苹果"},
			{Label: "香蕉"},
			{Label: "西瓜"},
		},
		Multiple: true,
	}

	// Single select - label
	if ans := formatQuestionAnswer(singleItem, "选项一"); ans != "选项一" {
		t.Errorf("single select option = %q, want %q", ans, "选项一")
	}
	// Single select - custom text
	if ans := formatQuestionAnswer(singleItem, "自定义其他内容"); ans != "自定义其他内容" {
		t.Errorf("single select custom = %q, want %q", ans, "自定义其他内容")
	}

	// Multi select - option indices 1-based
	if ans := formatQuestionAnswer(multiItem, []string{"苹果", "西瓜"}); ans != "1,3" {
		t.Errorf("multi select indices = %q, want %q", ans, "1,3")
	}
	// Multi select - option indices + custom text
	if ans := formatQuestionAnswer(multiItem, []string{"香蕉", "补充说明"}); ans != "2; 补充说明" {
		t.Errorf("multi select indices + custom = %q, want %q", ans, "2; 补充说明")
	}
	// Multi select - custom only
	if ans := formatQuestionAnswer(multiItem, "只有自定义内容"); ans != "只有自定义内容" {
		t.Errorf("multi select custom only = %q, want %q", ans, "只有自定义内容")
	}
	// Empty answer
	if ans := formatQuestionAnswer(singleItem, ""); ans != "" {
		t.Errorf("empty answer = %q, want empty", ans)
	}
}

func TestPiFormManager(t *testing.T) {
	mgr := newPiFormManager()
	info := agents.FormInfo{ID: "f1", SessionID: "ses-1", Title: "测试表单"}
	items := []agents.StandardQuestionItem{{ID: "q1", Question: "测试？"}}

	mgr.addQuestionnaireForm(info, items)
	list := mgr.list("ses-1")
	if len(list) != 1 || list[0].ID != "f1" {
		t.Fatalf("list forms = %+v, want [f1]", list)
	}

	form := mgr.findActiveQuestionnaireForm()
	if form == nil || form.info.ID != "f1" {
		t.Fatalf("findActiveQuestionnaireForm = %+v, want f1", form)
	}

	// After submission, it shouldn't be listed as pending
	form.mu.Lock()
	form.submitted = true
	form.mu.Unlock()

	if pending := mgr.list("ses-1"); len(pending) != 0 {
		t.Errorf("pending after submitted = %+v, want 0", pending)
	}

	// Draining
	drained := mgr.drain()
	if len(drained) != 1 || drained[0] != "f1" {
		t.Errorf("drain = %+v, want [f1]", drained)
	}
}

func TestBuildStandaloneForm(t *testing.T) {
	selectReq := &rpcExtensionUIRequest{
		ID:      "req-sel",
		Method:  "select",
		Title:   "请选择方向",
		Options: []string{"前端", "后端"},
	}
	formSel := buildStandaloneForm("ses-1", selectReq)
	if formSel.ID != "req-sel" || formSel.Title != "请选择方向" {
		t.Errorf("formSel mismatch: %+v", formSel)
	}

	inputReq := &rpcExtensionUIRequest{
		ID:          "req-inp",
		Method:      "input",
		Title:       "请输入姓名",
		Placeholder: "张三",
	}
	formInp := buildStandaloneForm("ses-1", inputReq)
	if formInp.ID != "req-inp" || formInp.Title != "请输入姓名" {
		t.Errorf("formInp mismatch: %+v", formInp)
	}
}

func TestQuestionnaireFullPipeline(t *testing.T) {
	instID := "inst-pipeline"
	sessionID := "ses-pipeline"

	var publishedMu sync.Mutex
	var published []agents.NormalizedEvent

	inst := &instance{
		id:   instID,
		now:  time.Now,
		subs: make(map[*subscriber]struct{}),
	}
	// Record published events by subscribing
	sub := newSubscriber()
	inst.subs[sub] = struct{}{}

	child := &rpcChild{
		sessionID: sessionID,
		forms:     newPiFormManager(),
		pending:   newPiPermissions(),
		inst:      inst,
	}

	toolInput := []byte(`{
		"questions": [
			{
				"header": "连通测试",
				"question": "你现在能看到这题并选择一个选项吗？",
				"options": [
					{"label": "我选这个，拿到回复了", "description": "说明正常"},
					{"label": "还是自动取消", "description": "说明异常"}
				],
				"multiSelect": false
			},
			{
				"header": "开发偏好",
				"question": "常用的语言有哪些？",
				"options": [
					{"label": "Go"},
					{"label": "Rust"},
					{"label": "TypeScript"}
				],
				"multiSelect": true
			}
		]
	}`)

	// 1. Simulate tool_execution_start through parser
	p := newParser(instID, sessionID, time.Now)
	var startEvents []agents.NormalizedEvent
	p.parseLine([]byte(`{"type":"tool_execution_start","toolCallId":"call-q1","toolName":"ask_user_question","args":`+string(toolInput)+`}`), func(ev agents.NormalizedEvent) {
		startEvents = append(startEvents, ev)
	})

	if len(startEvents) != 1 || startEvents[0].Type != agents.EventToolCall {
		t.Fatalf("expected 1 EventToolCall, got %+v", startEvents)
	}
	tc := startEvents[0].ToolCall
	if tc.Name != agents.StandardToolQuestion {
		t.Errorf("tool call name = %q, want %q", tc.Name, agents.StandardToolQuestion)
	}

	// 2. Child observes tool call
	child.observe(startEvents[0])

	forms := child.listForms()
	if len(forms) != 1 || forms[0].ID != "call-q1" {
		t.Fatalf("expected 1 form with id call-q1, got %+v", forms)
	}

	// Read published events from subscriber
	for {
		select {
		case ev := <-sub.stash:
			publishedMu.Lock()
			published = append(published, ev)
			publishedMu.Unlock()
		default:
			goto drainedEvents
		}
	}
drainedEvents:

	hasRequested := false
	for _, ev := range published {
		if ev.Type == agents.EventInteraction && ev.Interaction != nil &&
			ev.Interaction.Phase == agents.InteractionRequested && ev.Interaction.ID == "call-q1" {
			hasRequested = true
			break
		}
	}
	if !hasRequested {
		t.Error("expected InteractionRequested event for call-q1")
	}

	// 3. Pi emits select request for first question (intercepted by handleDialogQuestion)
	writer := &testCaptureWriter{}
	child.conn = newRPCConn(writer, p, func(ev agents.NormalizedEvent) {})
	child.conn.extensionUI = child.handleExtensionUI

	selectReq := &rpcExtensionUIRequest{
		ID:      "req-step-1",
		Method:  "select",
		Title:   "连通测试",
		Options: []string{"我选这个，拿到回复了", "还是自动取消"},
	}
	handled := child.handleExtensionUI(selectReq)
	if !handled {
		t.Fatal("handleExtensionUI for select returned false, should be intercepted")
	}
	if len(writer.lines) != 0 {
		t.Fatalf("expected 0 responses written before user replies, got %d", len(writer.lines))
	}

	// 4. User replies via replyForm
	answers := map[string]any{
		"q0": "我选这个，拿到回复了",
		"q1": []string{"Go", "TypeScript"},
	}
	err := child.replyForm("call-q1", answers)
	if err != nil {
		t.Fatalf("replyForm: %v", err)
	}

	// 5. First question should have been answered over child's connection
	if len(writer.lines) != 1 {
		t.Fatalf("expected 1 response written for question 1, got %d: %v", len(writer.lines), writer.lines)
	}
	var resp1 rpcExtensionUIResponse
	if err := json.Unmarshal(writer.lines[0], &resp1); err != nil {
		t.Fatalf("unmarshal resp1: %v", err)
	}
	if resp1.ID != "req-step-1" || resp1.Value == nil || *resp1.Value != "我选这个，拿到回复了" {
		t.Errorf("resp1 mismatch: %+v", resp1)
	}

	// 6. Pi now emits input request for second question
	inputReq := &rpcExtensionUIRequest{
		ID:     "req-step-2",
		Method: "input",
		Title:  "开发偏好",
	}
	handled2 := child.handleExtensionUI(inputReq)
	if !handled2 {
		t.Fatal("handleExtensionUI for question 2 returned false")
	}
	if len(writer.lines) != 2 {
		t.Fatalf("expected 2 responses written after question 2 arrived, got %d", len(writer.lines))
	}
	var resp2 rpcExtensionUIResponse
	if err := json.Unmarshal(writer.lines[1], &resp2); err != nil {
		t.Fatalf("unmarshal resp2: %v", err)
	}
	if resp2.ID != "req-step-2" || resp2.Value == nil || *resp2.Value != "1,3" {
		t.Errorf("resp2 mismatch: %+v, want Value='1,3'", resp2)
	}

	// 7. Tool execution ends
	var endEvents []agents.NormalizedEvent
	p.parseLine([]byte(`{"type":"tool_execution_end","toolCallId":"call-q1","toolName":"ask_user_question","result":{"content":[{"type":"text","text":"User has answered your questions"}]}}`), func(ev agents.NormalizedEvent) {
		endEvents = append(endEvents, ev)
	})
	if len(endEvents) != 1 || endEvents[0].Type != agents.EventToolResult {
		t.Fatalf("expected EventToolResult, got %+v", endEvents)
	}
	if endEvents[0].ToolResult.Output != "User has answered your questions" {
		t.Errorf("tool result output = %q, want the raw answer text", endEvents[0].ToolResult.Output)
	}

	// 8. Subsequent replyForm fails with ErrReplyExpired
	err = child.replyForm("call-q1", answers)
	if !errors.Is(err, agents.ErrReplyExpired) {
		t.Errorf("expected ErrReplyExpired on second replyForm, got %v", err)
	}
}

type testCaptureWriter struct {
	lines [][]byte
}

func (w *testCaptureWriter) Write(p []byte) (n int, err error) {
	for _, line := range splitLF(p) {
		if len(line) > 0 {
			w.lines = append(w.lines, line)
		}
	}
	return len(p), nil
}

func splitLF(data []byte) [][]byte {
	var res [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			res = append(res, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		res = append(res, data[start:])
	}
	return res
}

func TestStandaloneSelectReply(t *testing.T) {
	instID := "inst-standalone"
	sessionID := "ses-standalone"
	writer := &testCaptureWriter{}

	inst := &instance{
		id:   instID,
		now:  time.Now,
		subs: make(map[*subscriber]struct{}),
	}
	child := &rpcChild{
		sessionID: sessionID,
		forms:     newPiFormManager(),
		pending:   newPiPermissions(),
		inst:      inst,
	}
	child.conn = newRPCConn(writer, newParser(instID, sessionID, time.Now), func(agents.NormalizedEvent) {})

	req := &rpcExtensionUIRequest{
		ID:      "req-dialog-alone",
		Method:  "select",
		Title:   "单独的提问",
		Options: []string{"A", "B"},
	}

	handled := child.handleExtensionUI(req)
	if !handled {
		t.Fatal("handleExtensionUI for standalone select returned false")
	}

	forms := child.listForms()
	if len(forms) != 1 || forms[0].ID != "req-dialog-alone" {
		t.Fatalf("expected 1 standalone form, got %+v", forms)
	}

	err := child.replyForm("req-dialog-alone", map[string]any{"req-dialog-alone": "A"})
	if err != nil {
		t.Fatalf("replyForm: %v", err)
	}

	if len(writer.lines) != 1 {
		t.Fatalf("expected 1 line written, got %d", len(writer.lines))
	}
	var resp rpcExtensionUIResponse
	if err := json.Unmarshal(writer.lines[0], &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ID != "req-dialog-alone" || resp.Value == nil || *resp.Value != "A" {
		t.Errorf("resp mismatch: %+v", resp)
	}
}

func TestAdapterListAndReplyForm(t *testing.T) {
	adapter := New(Options{})
	ref := agents.SessionRef{InstanceID: "inst-none", SessionID: "ses-none"}

	// When instance does not exist
	forms, err := adapter.ListForms(context.Background(), ref)
	if err == nil {
		t.Errorf("expected error for non-existent instance, got forms: %+v", forms)
	}

	err = adapter.ReplyForm(context.Background(), ref, "f1", map[string]any{"q": "a"})
	if err == nil {
		t.Error("expected error for non-existent instance")
	}
}
