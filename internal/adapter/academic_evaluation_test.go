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
			_, _ = writer.Write([]byte(`<table><tr><th>序号</th><th>学期</th><th>类别</th><th>名称</th><th>开始</th><th>结束</th></tr><tr><td>1</td><td>2026-1</td><td>学生</td><td>评教</td><td>2026-01-01</td><td>2026-02-01</td><td><a href="/jsxsd/xspj/xspj_courses.do?id=batch1">进入</a></td></tr></table>`))
		case "/jsxsd/xspj/xspj_courses.do":
			_, _ = writer.Write([]byte(`<table id="dataList"><tr><th>序号</th><th>课程编号</th><th>课程</th><th>教师</th><th>类别</th><th>总分</th><th>已评价</th><th>已提交</th><th>学时</th></tr><tr><td>1</td><td>C001</td><td>课程</td><td>老师</td><td>学生</td><td>90</td><td>否</td><td>否</td><td>48</td><td><a href="/jsxsd/xspj/xspj_course.do?id=course1">进入</a></td></tr></table>`))
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

	handled, stdout, _, code, err = (NativeSite{}).Run(context.Background(), []string{"evaluation", "save", "--batch", "1", "--course-id", "C001", "--answer", "q1=A", "--yes", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("save: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	payload = map[string]any{}
	if err := json.Unmarshal(stdout, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != true || payload["submitted"] != true || payload["confirmed"] != true || payload["operation"] != "save" || payload["evidence"] != "save-response-confirmed" {
		t.Fatalf("unexpected save result: %#v", payload)
	}
}

func TestSelectEvaluationItemUsesSemanticSelectors(t *testing.T) {
	items := []map[string]any{
		{"index": 1, "sequence": "1", "course_id": "CS001", "course": "算法"},
		{"index": 2, "sequence": "2", "course_id": "CS002", "course": "操作系统"},
	}
	item, err := selectEvaluationItem(items, "CS002", "course_id", "course")
	if err != nil || item["course"] != "操作系统" {
		t.Fatalf("unexpected semantic selection: %#v %v", item, err)
	}
	if _, err := selectEvaluationItem(items, "", "course_id"); err == nil || err.Code != "ambiguous_target" {
		t.Fatalf("expected ambiguous empty selector, got %v", err)
	}
}
