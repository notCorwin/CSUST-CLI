package adapter

import (
	"testing"
)

func TestPageSnapshotExtractsControlsAndStableActions(t *testing.T) {
	source := `<html><head><title>测试页</title><script>function save(){alert('操作成功')} const api='/api/items.json'</script></head><body><form id="f" method="post" action="/save"><label for="name">名称</label><input id="name" name="name" value="旧值"><input name="password" type="password" value="secret"><select name="kind"><option value="a" selected>A</option></select><button type="submit">保存</button></form><table><tr><th>列</th></tr><tr><td>值</td></tr></table></body></html>`
	page, err := pageInspect(source, "https://www.csust.edu.cn/")
	if err != nil {
		t.Fatal(err)
	}
	if page["title"] != "测试页" || page["kind"] != "html" {
		t.Fatalf("unexpected page identity: %#v", page)
	}
	forms := page["forms"].([]map[string]any)
	if len(forms) != 1 || len(forms[0]["controls"].([]map[string]any)) != 4 {
		t.Fatalf("unexpected forms: %#v", forms)
	}
	password := forms[0]["controls"].([]map[string]any)[1]
	if password["value"] != "<redacted>" || password["text"] != "" {
		t.Fatalf("password was not redacted: %#v", password)
	}
	actions := page["actions"].([]map[string]any)
	if len(actions) == 0 || actions[0]["ref"] == "" {
		t.Fatalf("missing stable action: %#v", actions)
	}
	if endpoints := page["endpoints"].([]string); len(endpoints) != 1 || endpoints[0] != "https://www.csust.edu.cn/api/items.json" {
		t.Fatalf("unexpected endpoints: %#v", endpoints)
	}
}

func TestPageFeedbackRequiresDirectEvidence(t *testing.T) {
	if state, known := pageFeedback("操作成功", "text/plain"); !state || !known {
		t.Fatalf("plain success was not confirmed: %v %v", state, known)
	}
	if state, known := pageFeedback(`<html><body>历史记录：操作成功</body></html>`, "text/html"); state || known {
		t.Fatalf("historical HTML text should remain unverified: %v %v", state, known)
	}
	if state, known := pageFeedback(`<script>alert('操作成功')</script>`, "text/html"); !state || !known {
		t.Fatalf("direct script feedback was not confirmed: %v %v", state, known)
	}
	if state, known := pageFeedback(`<script>alert('回复成功！')</script>`, "text/html"); !state || !known {
		t.Fatalf("reply success was not confirmed: %v %v", state, known)
	}
}

func TestPageActionResolvesFunctionBody(t *testing.T) {
	document, err := parsePage(`<html><body><button onclick="login()">登录</button><script>function login(){ location.href = '/login/check'; }</script></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	actions := pageActionNodes(document)
	if len(actions) != 1 {
		t.Fatalf("actions=%d", len(actions))
	}
	method, target := pageActionTarget(actions[0], nil, document, "https://example.test/")
	if method != "GET" || target != "https://example.test/login/check" {
		t.Fatalf("method=%s target=%s", method, target)
	}
}

func TestSiteExplorationCommandsAreOptIn(t *testing.T) {
	t.Setenv("CSUST_EXPLORATION", "0")
	if _, err := parseSiteCommand([]string{"discover", "--service", "official"}); err == nil || err.Code != "exploration_required" {
		t.Fatalf("discover should require exploration mode: %#v", err)
	}
	t.Setenv("CSUST_EXPLORATION", "1")
	command, err := parseSiteCommand([]string{"discover", "--service", "official", "--depth", "0"})
	if err != nil || command.depth != 0 {
		t.Fatalf("discover parsing failed: %#v %#v", command, err)
	}
}
