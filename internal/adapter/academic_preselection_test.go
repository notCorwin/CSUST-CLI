package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestAcademicPreselectionRowsExtractTermAndCourseContracts(t *testing.T) {
	terms, err := parsePage(`<table><tr><th>序号</th><th>学年学期</th><th>选课阶段</th><th>选课开始时间</th><th>选课结束时间</th><th>操作</th></tr><tr><td>1</td><td>2026-2027-1</td><td>预选</td><td>2026-09-12</td><td>2026-09-13</td><td><a href="#" onclick="comeInXk('TERM-1')">进入</a></td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	termRows := academicPreselectionTermRows(terms, "")
	if len(termRows) != 1 || termRows[0]["term_id"] != "TERM-1" || termRows[0]["term"] != "2026-2027-1" {
		t.Fatalf("unexpected preselection term row: %#v", termRows)
	}

	courses, err := parsePage(`<form name="xkOperForm" action="/jsxsd/xkgl/yxxkOper"><table><tr><th></th><th>序号</th><th>课程组名称</th><th>学分要求</th><th>课程编号</th><th>课程名称</th><th>开课单位</th><th>已选人数</th><th>总学时</th><th>学分</th><th>是否选中</th><th>操作</th></tr><tr><td><input type="checkbox" name="kcCheck" value="SEL-1"></td><td>1</td><td>专业课</td><td>2</td><td>C-001</td><td>课程甲</td><td>教务处</td><td>0</td><td>32</td><td>2</td><td>否</td><td><button onclick="yxtkOper('DROP-1',this)">退选</button></td></tr></table></form>`)
	if err != nil {
		t.Fatal(err)
	}
	courseRows := academicPreselectionCourseRows(courses)
	if len(courseRows) != 1 || courseRows[0]["course_id"] != "C-001" || courseRows[0]["selection_value"] != "SEL-1" || courseRows[0]["drop_id"] != "DROP-1" || courseRows[0]["selected"] != false {
		t.Fatalf("unexpected preselection course row: %#v", courseRows)
	}
}

func TestAcademicPreselectionSelectRequiresReadback(t *testing.T) {
	selected := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch request.URL.Path {
		case academicPreselectionListPath:
			if request.Method != http.MethodGet {
				t.Fatalf("unexpected term method: %s", request.Method)
			}
			_, _ = writer.Write([]byte(`<table><tr><th>序号</th><th>学年学期</th><th>选课阶段</th><th>选课开始时间</th><th>选课结束时间</th><th>操作</th></tr><tr><td>1</td><td>2026-2027-1</td><td>预选</td><td>2026-09-12</td><td>2026-09-13</td><td><a href="#" onclick="comeInXk('TERM-1')">进入</a></td></tr></table>`))
		case academicPreselectionCoursesPath:
			if request.Method != http.MethodPost || request.FormValue("xnxq01id") != "TERM-1" {
				t.Fatalf("unexpected course request: %s %s", request.Method, request.URL.String())
			}
			checked := ""
			if selected {
				checked = " checked"
			}
			_, _ = writer.Write([]byte(`<form name="xkOperForm" action="/jsxsd/xkgl/yxxkOper"><table><tr><th></th><th>序号</th><th>课程组名称</th><th>学分要求</th><th>课程编号</th><th>课程名称</th><th>开课单位</th><th>已选人数</th><th>总学时</th><th>学分</th><th>是否选中</th><th>操作</th></tr><tr><td><input type="checkbox" name="kcCheck" value="SEL-1"` + checked + `></td><td>1</td><td>专业课</td><td>2</td><td>C-001</td><td>课程甲</td><td>教务处</td><td>0</td><td>32</td><td>2</td><td>` + map[bool]string{true: "是", false: "否"}[selected] + `</td><td></td></tr></table></form>`))
		case academicPreselectionSelectPath:
			if request.Method != http.MethodGet || request.URL.Query().Get("xnxq01id") != "TERM-1" || request.URL.Query().Get("kcCheck") != "SEL-1" {
				t.Fatalf("unexpected select request: %s", request.URL.String())
			}
			selected = true
			_, _ = writer.Write([]byte(`<script>alert('选课成功')</script>`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	handled, output, _, code, err := (NativeSite{}).Run(context.Background(), []string{"preselection", "select", "--term", "2026-2027-1", "--course-id", "C-001", "--yes", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("select: handled=%v code=%d err=%v output=%s", handled, code, err, output)
	}
	var payload map[string]any
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["submitted"] != true || payload["confirmed"] != true || payload["evidence"] != "server-success-and-course-list-readback" {
		t.Fatalf("unexpected preselection result: %#v", payload)
	}
}
