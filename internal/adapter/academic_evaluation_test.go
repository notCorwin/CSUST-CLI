package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestNativeAcademicEvaluationUsesDirectService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch request.URL.Path {
		case "/jsxsd/xspj/xspj_find.do":
			_, _ = writer.Write([]byte(`<table><tr><th>序号</th><th>学期</th><th>类别</th><th>名称</th><th>开始</th><th>结束</th></tr><tr><td>1</td><td>2026-1</td><td>学生</td><td>评教</td><td>2026-01-01</td><td>2026-02-01</td><td><a href="/jsxsd/xspj/xspj_course.do?id=1">进入</a></td></tr></table>`))
		case "/jsxsd/xspj/xspj_course.do":
			_, _ = writer.Write([]byte(`<form method="post" action="/jsxsd/xspj/xspj_save.do"><input type="hidden" name="execution" value="secret"><table><tr><td><input name="pj06xh" value="q1">教学质量</td><td><input type="radio" name="q1opt" value="A"><input type="radio" name="q1opt" value="B"></td></tr></table><textarea name="suggestion"></textarea><button type="submit" onclick="saveData()">保存</button></form>`))
		case "/jsxsd/xspj/xspj_save.do":
			_, _ = writer.Write([]byte(`<script>alert('评价成功')</script>`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	handled, stdout, _, code, err := (NativeSite{}).Run(context.Background(), []string{"evaluation", "batches", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("batches: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload["items"].([]any)) != 1 {
		t.Fatalf("unexpected batches: %#v", payload)
	}

	handled, stdout, _, code, err = (NativeSite{}).Run(context.Background(), []string{"evaluation", "save", "--path", "/jsxsd/xspj/xspj_course.do", "--answer", "q1=A", "--yes", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("save: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	payload = map[string]any{}
	if err := json.Unmarshal(stdout, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != true || payload["confirmed"] != true || payload["operation"] != "save" {
		t.Fatalf("unexpected save result: %#v", payload)
	}
}
