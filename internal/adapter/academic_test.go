package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestAcademicParsersKeepSemanticRows(t *testing.T) {
	schedule := `<select id="xnxq01id"><option value="2025-2026-1" selected>2025-2026-1</option></select><table id="kbtable"><tr><th></th><th>星期一</th><th>星期二</th><th>星期三</th><th>星期四</th><th>星期五</th><th>星期六</th><th>星期日</th></tr><tr><th>1、2节</th><td><div class="kbcontent">线性代数<br><font title="老师">张老师</font><br><font title="周次(节次)">1-2(周)[01-02节]</font><br><font title="教室">A101</font></div></td><td></td><td></td><td></td><td></td><td></td><td></td></tr></table>`
	document, err := parsePage(schedule)
	if err != nil {
		t.Fatal(err)
	}
	items, parseErr := parseSchedulePage(document, "2025-2026-1")
	if parseErr != nil || len(items) != 1 {
		t.Fatalf("unexpected schedule: %#v %v", items, parseErr)
	}
	if items[0]["weekday"] != "星期一" || items[0]["course"] != "线性代数" {
		t.Fatalf("unexpected schedule row: %#v", items[0])
	}

	grades := `<table id="dataList"><tr><th>学期</th><th>课程名称</th><th>成绩</th><th>学分</th><th>学时</th><th>绩点</th></tr><tr><td>2025-2026-1</td><td>线性代数</td><td>95</td><td>3</td><td>48</td><td>4.0</td></tr></table>`
	document, err = parsePage(grades)
	if err != nil {
		t.Fatal(err)
	}
	gradeRows, parseErr := parseGradesPage(document, "http://xk.csust.edu.cn/jsxsd/kscj/cjcx_list")
	if parseErr != nil || len(gradeRows) != 1 || gradeRows[0]["score"] != "95" {
		t.Fatalf("unexpected grades: %#v %v", gradeRows, parseErr)
	}
	detailDocument, err := parsePage(`<table id="dataList"><tr><td>只有一行</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	detail, parseErr := parseGradeDetailPage(detailDocument, "http://xk.csust.edu.cn/jsxsd/kscj/detail")
	if parseErr == nil || detail != nil {
		t.Fatalf("expected missing grade detail error: %#v %v", detail, parseErr)
	}
}

func TestAcademicCourseSelectionRowsNormalizeCrossMajorFields(t *testing.T) {
	document, err := parsePage(`<table><tr><th>课程代码</th><th>课程名称</th><th>教师</th><th>学分</th><th>课程性质</th></tr><tr><td>CS001</td><td>跨专业选修</td><td>张老师</td><td>2</td><td>选修</td></tr><tr><td>CS002</td><td>不相关课程</td><td>李老师</td><td>3</td><td>选修</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	rows := academicCourseRows(document, "跨专业")
	if len(rows) != 1 || rows[0]["course_id"] != "CS001" || rows[0]["course"] != "跨专业选修" || rows[0]["teacher"] != "张老师" || rows[0]["credit"] != "2" {
		t.Fatalf("unexpected cross-major rows: %#v", rows)
	}
}

func TestAcademicStructuredRowsNormalizeRequestFields(t *testing.T) {
	document, err := parsePage(`<table><tr><th>申请编号</th><th>教室</th><th>借用日期</th><th>状态</th></tr><tr><td>R001</td><td>A101</td><td>2026-09-10</td><td>待审核</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	rows := academicStructuredRows(document, "", academicPageField)
	if len(rows) != 1 || rows[0]["id"] != "R001" || rows[0]["room"] != "A101" || rows[0]["date"] != "2026-09-10" || rows[0]["status"] != "待审核" {
		t.Fatalf("unexpected request rows: %#v", rows)
	}
}

func TestAcademicStructuredRowsDoesNotReturnNoDataAsCourse(t *testing.T) {
	document, err := parsePage(`<table><tr><th>课程名称</th></tr><tr><td>未查询到数据</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	if rows := academicStructuredRows(document, "", academicCourseField); len(rows) != 0 {
		t.Fatalf("no-data row became a course: %#v", rows)
	}
}

func TestAcademicPathAcceptsSchoolDetailURL(t *testing.T) {
	path, err := academicPath("http://xk.csust.edu.cn/jsxsd/kscj/detail?id=1")
	if err != nil || path != "/jsxsd/kscj/detail?id=1" {
		t.Fatalf("unexpected detail path: %q %v", path, err)
	}
}

func TestAcademicSelectionEntryFindsCrossMajorWindow(t *testing.T) {
	document, err := parsePage(`<table><tr><th>名称</th><th>操作</th></tr><tr><td>跨专业选修课补选</td><td><a href="/jsxsd/xsxk/xklc_view?jx0502zbid=abc">进入选课</a></td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	path := academicSelectionEntry(document, "http://xk.csust.edu.cn/jsxsd/xsxk/xklc_list", "跨专业选修")
	if path != "/jsxsd/xsxk/xklc_view?jx0502zbid=abc" {
		t.Fatalf("unexpected cross-major entry: %q", path)
	}
}

func TestAcademicCourseSelectionFollowsCrossMajorWindow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		if request.URL.Path == "/jsxsd/xsxk/xklc_list" {
			_, _ = writer.Write([]byte(`<table><tr><th>名称</th><th>操作</th></tr><tr><td>跨专业选修课补选</td><td><a href="/jsxsd/xsxk/xklc_view?jx0502zbid=1">进入选课</a></td></tr></table>`))
			return
		}
		_, _ = writer.Write([]byte(`<table><tr><th>课程代码</th><th>课程名称</th><th>教师</th><th>学分</th></tr><tr><td>CS001</td><td>数据结构</td><td>张老师</td><td>3</td></tr></table>`))
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))
	result, err := (NativeSite{}).academicCourseSelection(context.Background(), []string{"--scope", "cross-major"})
	if err != nil || result["item_count"] != 1 {
		t.Fatalf("unexpected cross-major selection: %#v %v", result, err)
	}
	items, ok := result["items"].([]map[string]any)
	if !ok || items[0]["course_id"] != "CS001" || result["path"] != "/jsxsd/xsxk/xklc_view?jx0502zbid=1" {
		t.Fatalf("cross-major entry was not followed: %#v", result)
	}
}
