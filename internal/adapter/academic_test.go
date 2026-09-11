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

func TestAcademicCourseSelectionNormalizesWindows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(`<table id="tbKxkc"><tr><th>学年学期</th><th>选课名称</th><th>选课时间</th><th>操作</th></tr><tr><td>2026-2027-1</td><td>公共选修课</td><td>2026-09-11 14:00~23:00</td><td><a href="/jsxsd/xsxk/xsxk_index?jx0502zbid=ABC123">进入选课</a></td></tr></table>`))
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	result, err := (NativeSite{}).academicCourseSelection(context.Background(), []string{"--scope", "center"})
	if err != nil {
		t.Fatal(err)
	}
	items, ok := result["items"].([]map[string]any)
	if !ok || len(items) != 1 || items[0]["term"] != "2026-2027-1" || items[0]["selection_name"] != "公共选修课" || items[0]["selection_time"] != "2026-09-11 14:00~23:00" || items[0]["entry_path"] != "/jsxsd/xsxk/xsxk_index?jx0502zbid=ABC123" {
		t.Fatalf("unexpected selection window: %#v", result)
	}
}

func TestAcademicTrainingPlanUsesSemanticFields(t *testing.T) {
	method := ""
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		method = request.Method
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(`<table><tr><th>学期</th><th>课程代码</th><th>课程名称</th><th>开课单位</th><th>学分</th><th>考核方式</th><th>是否考试</th></tr><tr><td>第1学期</td><td>CS001</td><td>数据结构</td><td>计算机学院</td><td>3</td><td>考试</td><td>是</td></tr></table>`))
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	result, err := (NativeSite{}).academicTrainingPlan(context.Background(), nil)
	if err != nil || method != "POST" {
		t.Fatalf("unexpected training plan request: method=%s result=%#v err=%v", method, result, err)
	}
	items, ok := result["items"].([]map[string]any)
	if !ok || len(items) != 1 || items[0]["term"] != "第1学期" || items[0]["course_id"] != "CS001" || items[0]["department"] != "计算机学院" || items[0]["assessment_method"] != "考试" || items[0]["exam"] != "是" {
		t.Fatalf("unexpected semantic training plan: %#v", result)
	}
}

func TestAcademicTrainingProgressKeepsCompletionAndCreditSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(`<table><tr><th>课程体系</th><th>选课组</th><th>课程编号</th><th>课程名称</th><th>完成情况</th><th>课程性质</th><th>课程属性</th><th>学分</th><th>学时分类</th><th>开设学期</th></tr><tr><th>讲课学时</th><th>上机学时</th><th>其它学时</th><th>实验学时</th><th>实践学时</th><th>总学时</th></tr><tr><td colspan="10">必修 (应修 10 / 已修 5)</td></tr><tr><td>公共课</td><td></td><td>CS001</td><td>数据结构</td><td>已修(90)</td><td>公共课</td><td>必修</td><td>3</td><td>32</td><td>0</td><td>0</td><td>0</td><td>0</td><td>32</td><td>第2学期</td></tr></table>`))
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	result, err := (NativeSite{}).academicTrainingProgress(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := result["items"].([]map[string]any)
	if !ok || len(items) != 1 || items[0]["course_id"] != "CS001" || items[0]["completion"] != "已修(90)" || items[0]["credit"] != "3" || items[0]["lecture_hours"] != "32" || items[0]["total_hours"] != "32" || items[0]["offered_term"] != "第2学期" {
		t.Fatalf("unexpected training progress: %#v", result)
	}
	summary, ok := result["credit_summary"].(map[string]map[string]string)
	if !ok || summary["必修"]["required"] != "10" || summary["必修"]["earned"] != "5" {
		t.Fatalf("unexpected credit summary: %#v", result["credit_summary"])
	}
}

func TestAcademicSecondClassCreditRowsKeepApplicationStates(t *testing.T) {
	document, err := parsePage(`<table><tr><th>序号</th><th>组织方式</th><th>学年学期</th><th>分类名称</th><th>获得项目时间</th><th>认定学分</th><th>审核状态</th><th>认定状态</th><th>备注</th><th>操作</th></tr><tr><td>1</td><td>个人</td><td>2025-2026-1</td><td>学科竞赛</td><td>2026-01-01</td><td>2</td><td>已通过</td><td>已认定</td><td>备注</td><td>流程</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	rows := academicStructuredRows(document, "", academicSecondClassCreditField)
	if len(rows) != 1 || rows[0]["organization"] != "个人" || rows[0]["term"] != "2025-2026-1" || rows[0]["category"] != "学科竞赛" || rows[0]["recognized_credit"] != "2" || rows[0]["review_status"] != "已通过" || rows[0]["recognition_status"] != "已认定" || rows[0]["action"] != "流程" {
		t.Fatalf("unexpected second-class credit row: %#v", rows)
	}
}

func TestAcademicStatusWarningRowsKeepDecisionFields(t *testing.T) {
	document, err := parsePage(`<table><tr><th>序号</th><th>预警学期</th><th>预警名称</th><th>预警条件</th><th>处理结果</th><th>提示信息</th><th>对象名称</th><th>实际值</th></tr><tr><td>1</td><td>2025-2026-1</td><td>学分预警</td><td>已修学分不足</td><td>未处理</td><td>请及时处理</td><td>学分</td><td>10</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	rows := academicStructuredRows(document, "", academicStatusWarningField)
	if len(rows) != 1 || rows[0]["term"] != "2025-2026-1" || rows[0]["warning"] != "学分预警" || rows[0]["condition"] != "已修学分不足" || rows[0]["result"] != "未处理" || rows[0]["message"] != "请及时处理" || rows[0]["object"] != "学分" || rows[0]["actual_value"] != "10" {
		t.Fatalf("unexpected academic warning row: %#v", rows)
	}
}

func TestAcademicAnnouncementRowsKeepDetailPath(t *testing.T) {
	document, err := parsePage(`<table><tr><th>序号</th><th>标题</th><th>类别</th><th>发送人</th><th>发送时间</th><th>操作</th></tr><tr><td>1</td><td>选课通知</td><td>通知公告</td><td>教务处</td><td>2026-09-11</td><td><a href="javascript:void(0);" onclick="openWindow('/jsxsd/ggly/ggly_show?ggid=ABC',500,400)">查看</a></td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	rows := academicStructuredRowsWithLinks(document, "", academicAnnouncementField, "http://xk.csust.edu.cn/jsxsd/ggly/ysgg_query")
	if len(rows) != 1 || rows[0]["title"] != "选课通知" || rows[0]["category"] != "通知公告" || rows[0]["sender"] != "教务处" || rows[0]["detail_path"] != "/jsxsd/ggly/ggly_show?ggid=ABC" {
		t.Fatalf("unexpected announcement row: %#v", rows)
	}
}
