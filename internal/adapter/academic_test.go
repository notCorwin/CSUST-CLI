package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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

func TestAcademicGradesExtractJavascriptDetailLink(t *testing.T) {
	document, err := parsePage(`<table id="dataList"><tr><th>学期</th><th>课程名称</th><th>成绩</th><th>学分</th><th>学时</th><th>绩点</th></tr><tr><td>2025-2026-1</td><td>线性代数</td><td><a href="javascript:openWindow('/jsxsd/kscj/pscj_list.do?cj0708id=ABC&amp;zcj=95',700,500)">95</a></td><td>3</td><td>48</td><td>4.0</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	rows, parseErr := parseGradesPage(document, "http://xk.csust.edu.cn/jsxsd/kscj/cjcx_list")
	if parseErr != nil || len(rows) != 1 || rows[0]["grade_detail_url"] != "http://xk.csust.edu.cn/jsxsd/kscj/pscj_list.do?cj0708id=ABC&zcj=95" {
		t.Fatalf("javascript grade detail link was not normalized: %#v %v", rows, parseErr)
	}
}

func TestAcademicGradesParseSummaryMetrics(t *testing.T) {
	document, err := parsePage(`<div>查询条件：全部 已获得总学分:79(其中必修71.5,公选3,选修4.5)，平均学分绩点:2.81，平均成绩:81.64;</div>`)
	if err != nil {
		t.Fatal(err)
	}
	summary := parseGradeSummary(document)
	if summary["earned_credit"] != "79" || summary["required_credit"] != "71.5" || summary["general_elective_credit"] != "3" || summary["elective_credit"] != "4.5" || summary["average_grade_point"] != "2.81" || summary["average_score"] != "81.64" {
		t.Fatalf("unexpected grade summary: %#v", summary)
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

func TestGraduationConclusionKeepsSemanticFields(t *testing.T) {
	document := `<table id="xjkpTable"><tr><td></td><td>姓&nbsp;&nbsp;名: 张三</td></tr><tr><td>入学年份: 2024</td></tr><tr><td>上课专业: 道路工程</td></tr><tr><td>毕业结论: 合格</td></tr><tr><td>学位结论: 授予</td></tr></table>`
	result := parseGraduationConclusionPage(document, "http://xk.csust.edu.cn/jsxsd/bygl/bygl_ckxsList")
	if result["name"] != "张三" || result["enrollment_year"] != "2024" || result["major"] != "道路工程" || result["graduation_conclusion"] != "合格" || result["degree_conclusion"] != "授予" {
		t.Fatalf("unexpected graduation conclusion: %#v", result)
	}
}

func TestGraduationInfoCheckKeepsVisibleFieldsAndStatus(t *testing.T) {
	document, err := parsePage(`<table><tr><td>所属学院:</td><td>交通学院</td><td>所属专业:</td><td>道路桥梁与渡河工程</td></tr><tr><td>所在班级:</td><td>道桥渡24-1</td><td>培养层次:</td><td>普通本科</td></tr><tr><td>学制:</td><td>4</td><td>性别:</td><td>男</td></tr><tr><td>证件类型:</td><td>身份证</td><td>证件号:</td><td></td></tr><tr><td>学号:</td><td>202401150107</td><td>姓名:</td><td></td></tr><tr><td></td><td>注：毕业生信息核对时间未到！</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	result := parseGraduationInfoCheckPage(document, "http://xk.csust.edu.cn/jsxsd/bygl/bysxx")
	if result["college"] != "交通学院" || result["major"] != "道路桥梁与渡河工程" || result["class"] != "道桥渡24-1" || result["student_id"] != "202401150107" || result["id_number"] != "" || result["status_message"] != "注：毕业生信息核对时间未到！" {
		t.Fatalf("unexpected graduation info check: %#v", result)
	}
}

func TestEnrollmentProofRowsKeepSemanticFields(t *testing.T) {
	document, err := parsePage(`<table><tr><th>序号</th><th>学号</th><th>姓名</th><th>性别</th><th>籍贯</th><th>民族</th><th>培养层次</th><th>入学日期</th><th>身份证号</th><th>出生日期</th><th>备注</th><th>申请时间</th></tr><tr><td>1</td><td>202401150107</td><td>张三</td><td>男</td><td>湖南</td><td>汉族</td><td>普通本科</td><td>2024-09-01</td><td>ID</td><td>2006-01-01</td><td>在读</td><td>2026-09-11</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	items := academicStructuredRows(document, "", academicEnrollmentProofField)
	if len(items) != 1 || items[0]["student_id"] != "202401150107" || items[0]["study_level"] != "普通本科" || items[0]["enrollment_date"] != "2024-09-01" || items[0]["applied_at"] != "2026-09-11" {
		t.Fatalf("unexpected enrollment proof row: %#v", items)
	}
}

func TestTeachingCalendarRowsKeepWeekdayFields(t *testing.T) {
	document, err := parsePage(`<table><tr><th></th><th>星期日</th><th>星期一</th><th>星期二</th><th>星期三</th><th>星期四</th><th>星期五</th><th>星期六</th><th>备注</th></tr><tr><td>1</td><td>09月06日</td><td>07</td><td>08</td><td>09</td><td>10</td><td>11</td><td>09月12日</td><td>开学</td></tr><tr><td>周历编制</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	items, parseErr := parseTeachingCalendarPage(document)
	if parseErr != nil || len(items) != 1 || items[0]["week"] != "1" || items[0]["monday"] != "07" || items[0]["saturday"] != "09月12日" || items[0]["note"] != "开学" {
		t.Fatalf("unexpected teaching calendar: %#v %v", items, parseErr)
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

func TestAcademicDeferredExamRowsKeepSemanticFields(t *testing.T) {
	document, err := parsePage(`<table><tr><th>序号</th><th>学年学期</th><th>课程编号</th><th>课程名称</th><th>学时</th><th>学分</th><th>考试方式</th><th>成绩标识</th><th>缓考原因</th><th>审核状态</th><th>申请时间</th><th>操作</th></tr><tr><td>1</td><td>2025-2026-1</td><td>CS001</td><td>数据结构</td><td>48</td><td>3</td><td>考试</td><td>正常</td><td>因病</td><td>通过</td><td>2026-01-02</td><td>查看</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	rows := academicStructuredRows(document, "", academicDeferredExamField)
	if len(rows) != 1 || rows[0]["term"] != "2025-2026-1" || rows[0]["course_id"] != "CS001" || rows[0]["course"] != "数据结构" || rows[0]["hours"] != "48" || rows[0]["credit"] != "3" || rows[0]["assessment_method"] != "考试" || rows[0]["score_mark"] != "正常" || rows[0]["reason"] != "因病" || rows[0]["status"] != "通过" || rows[0]["submitted_at"] != "2026-01-02" || rows[0]["action"] != "查看" {
		t.Fatalf("unexpected deferred exam row: %#v", rows)
	}
}

func TestAcademicExemptExamRowsKeepDistinctStatuses(t *testing.T) {
	document, err := parsePage(`<table><tr><th>序号</th><th>学年学期</th><th>上课院系</th><th>姓名</th><th>课程名称</th><th>考试性质</th><th>考试状态</th><th>考试方式</th><th>班级名称</th><th>学时</th><th>学分</th><th>课程属性</th><th>免考原因</th><th>审核状态</th><th>操作</th></tr><tr><td>1</td><td>2025-2026-1</td><td>计算机学院</td><td>张三</td><td>数据结构</td><td>正常考试</td><td>已通过</td><td>考试</td><td>计科24-1</td><td>48</td><td>3</td><td>专业课</td><td>竞赛获奖</td><td>通过</td><td>查看</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	rows := academicStructuredRows(document, "", academicExemptExamField)
	if len(rows) != 1 || rows[0]["department"] != "计算机学院" || rows[0]["course"] != "数据结构" || rows[0]["exam_status"] != "已通过" || rows[0]["review_status"] != "通过" || rows[0]["reason"] != "竞赛获奖" || rows[0]["course_attribute"] != "专业课" {
		t.Fatalf("unexpected exempt exam row: %#v", rows)
	}
}

func TestAcademicEnrollmentStatusChangeRowsKeepBeforeAndAfterFields(t *testing.T) {
	document, err := parsePage(`<table><tr><th>详情</th><th>原班级</th><th>原学籍</th><th>原在校</th><th>新学院</th><th>新专业</th><th>新班级</th><th>新学籍</th><th>新状态</th><th>新在校</th><th>异动类别</th><th>终审状态</th></tr><tr><td>查看</td><td>土木24-1</td><td>正常</td><td>在校</td><td>交通学院</td><td>道路工程</td><td>道桥24-1</td><td>正常</td><td>正常</td><td>在校</td><td>转专业</td><td>通过</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	rows := academicStructuredRows(document, "", academicEnrollmentStatusChangeField)
	if len(rows) != 1 || rows[0]["previous_class"] != "土木24-1" || rows[0]["new_college"] != "交通学院" || rows[0]["new_major"] != "道路工程" || rows[0]["change_type"] != "转专业" || rows[0]["final_review_status"] != "通过" {
		t.Fatalf("unexpected enrollment status change row: %#v", rows)
	}
}

func TestGraduateExamCampusUsesSemanticValues(t *testing.T) {
	if name, value, err := graduateExamCampus("yuntang"); err != nil || name != "云塘校区" || value != "1" {
		t.Fatalf("unexpected yuntang campus: %q %q %v", name, value, err)
	}
	if _, _, err := graduateExamCampus("unknown"); err == nil || err.Code != "invalid_argument" {
		t.Fatalf("invalid campus was accepted: %#v", err)
	}
}

func TestAcademicRegistrationStatusMessageKeepsVisibleStatus(t *testing.T) {
	document, err := parsePage(`<html><body><b>当前不在报名时间范围内或未启用报名！</b></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	page, pageErr := pageInspect(`<html><body><b>当前不在报名时间范围内或未启用报名！</b></body></html>`, "http://xk.csust.edu.cn/jsxsd/kscj/hkbm_list")
	if pageErr != nil || academicRegistrationStatusMessage(document, page) != "当前不在报名时间范围内或未启用报名！" {
		t.Fatalf("unexpected registration status: %#v %v", page, pageErr)
	}
}

func TestAcademicGradeRecognitionRowsKeepApplicationFields(t *testing.T) {
	document, err := parsePage(`<table><tr><th>序号</th><th>学年学期</th><th>课程编号</th><th>课程名称</th><th>学分</th><th>总学时</th><th>成绩项目</th><th>原成绩</th><th>申请时间</th><th>审核状态</th><th>操作</th></tr><tr><td>1</td><td>2025-2026-1</td><td>CS001</td><td>数据结构</td><td>3</td><td>48</td><td>平时成绩</td><td>90</td><td>2026-01-02</td><td>通过</td><td>查看</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	rows := academicStructuredRows(document, "", academicGradeRecognitionField)
	if len(rows) != 1 || rows[0]["term"] != "2025-2026-1" || rows[0]["course_id"] != "CS001" || rows[0]["grade_item"] != "平时成绩" || rows[0]["original_score"] != "90" || rows[0]["review_status"] != "通过" {
		t.Fatalf("unexpected grade recognition row: %#v", rows)
	}
}

func TestAcademicDeferredExamApplicationsUsesActivityAPIAndSemanticFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch request.URL.Path {
		case "/jsxsd/kscj/hksq_query":
			_, _ = writer.Write([]byte(`<select id="xnxqid"><option value="2026-2027-1" selected>2026-2027-1</option></select>`))
		case "/jsxsd/kscj/hksq_query_ajax":
			if request.URL.Query().Get("xnxq01id") != "2025-2026-1" {
				t.Fatalf("unexpected deferred exam activity term: %s", request.URL.String())
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`[{"cj0701id":"A1","cjlrmc":"2025-2026-1期末考试"}]`))
		case "/jsxsd/kscj/hksq_list":
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if request.Form.Get("xnxqid") != "2025-2026-1" || request.Form.Get("cj0701id") != "A1" || request.Form.Get("kch") != "CS001" || request.Form.Get("iswfmes") != "1" {
				t.Fatalf("unexpected deferred exam filters: %#v", request.Form)
			}
			_, _ = writer.Write([]byte(`<table><tr><th>序号</th><th>学年学期</th><th>课程编号</th><th>课程名称</th><th>学时</th><th>学分</th><th>考试方式</th><th>成绩标识</th><th>缓考原因</th><th>审核状态</th><th>申请时间</th><th>操作</th></tr><tr><td>1</td><td>2025-2026-1</td><td>CS001</td><td>数据结构</td><td>48</td><td>3</td><td>考试</td><td>正常</td><td>因病</td><td>通过</td><td>2026-01-02</td><td>查看</td></tr></table>`))
		default:
			t.Fatalf("unexpected deferred exam path: %s", request.URL.Path)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	result, err := (NativeSite{}).academicDeferredExamApplications(context.Background(), []string{"--term", "2025-2026-1", "--activity", "期末考试", "--course", "CS001", "--status", "approved"})
	if err != nil || result["term"] != "2025-2026-1" || result["activity"] != "2025-2026-1期末考试" || result["status"] != "approved" || result["item_count"] != 1 {
		t.Fatalf("unexpected deferred exam result: %#v %v", result, err)
	}
	items, ok := result["items"].([]map[string]any)
	if !ok || items[0]["course_id"] != "CS001" || items[0]["status"] != "通过" {
		t.Fatalf("unexpected deferred exam items: %#v", result["items"])
	}
}

func TestAcademicDropCourseRowsKeepSemanticFields(t *testing.T) {
	document, err := parsePage(`<table><tr><th>课程名称</th><th>课程编号</th><th>授课教师</th><th>总学时</th><th>学分</th><th>课程属性</th><th>课程性质</th><th>审核状态</th><th>操作</th></tr><tr><td>数据结构</td><td>CS001</td><td>张老师</td><td>48</td><td>3</td><td>专业核心</td><td>必修</td><td>待审核</td><td>申请</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	rows := academicStructuredRows(document, "", academicDropCourseField)
	if len(rows) != 1 || rows[0]["course"] != "数据结构" || rows[0]["course_id"] != "CS001" || rows[0]["teacher"] != "张老师" || rows[0]["hours"] != "48" || rows[0]["credit"] != "3" || rows[0]["course_attribute"] != "专业核心" || rows[0]["course_nature"] != "必修" || rows[0]["status"] != "待审核" || rows[0]["action"] != "申请" {
		t.Fatalf("unexpected drop course row: %#v", rows)
	}
}

func TestAcademicStudentStatusChangeRowsKeepAuditFields(t *testing.T) {
	document, err := parsePage(`<table><tr><th>序号</th><th>学号</th><th>姓名</th><th>修改字段</th><th>修改信息</th><th>审核状态</th><th>修改时间</th><th>操作</th></tr><tr><td>1</td><td>202401150107</td><td>张三</td><td>电话</td><td>电话:无数据-->13800000000</td><td>通过</td><td>2026-01-02</td><td>查看</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	rows := academicStructuredRows(document, "", academicStudentStatusChangeField)
	if len(rows) != 1 || rows[0]["student_id"] != "202401150107" || rows[0]["name"] != "张三" || rows[0]["changed_field"] != "电话" || rows[0]["change_detail"] != "电话:无数据-->13800000000" || rows[0]["review_status"] != "通过" || rows[0]["modified_at"] != "2026-01-02" || rows[0]["action"] != "查看" {
		t.Fatalf("unexpected student status change row: %#v", rows)
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

func TestAcademicSecondClassCreditApplicationRowsKeepWorkflowID(t *testing.T) {
	document, err := parsePage(`<table><tr><th>序号</th><th>组织方式</th><th>学年学期</th><th>分类名称</th><th>获得项目时间</th><th>认定学分</th><th>审核状态</th><th>认定状态</th><th>备注</th><th>操作</th></tr><tr><td>1</td><td>个人</td><td>2025-2026-1</td><td>学科竞赛</td><td>2026-01-01</td><td>2</td><td>已通过</td><td>已认定</td><td>备注</td><td><a href="javascript:void(0);" onclick="openWindow('/jsxsd/pyfa/cxxfsb_shyj?cxxf04id=APP001',500,400)">流程</a></td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	rows := academicSecondClassCreditApplicationRows(document, "", "http://xk.csust.edu.cn/jsxsd/pyfa/cxxfsb_query")
	if len(rows) != 1 || rows[0]["application_id"] != "APP001" || rows[0]["workflow_path"] != "/jsxsd/pyfa/cxxfsb_shyj?cxxf04id=APP001" || rows[0]["detail_path"] != nil {
		t.Fatalf("unexpected second-class credit application row: %#v", rows)
	}
}

func TestAcademicSecondClassCreditApplicationDetailKeepsHistories(t *testing.T) {
	document, err := parsePage(`<table><tr><td>项目获得时间：</td><td>2025-02-27</td></tr><tr><td>审核状态：</td><td>2025-02-28 申请；2025-03-01 审核通过</td></tr><tr><td>认定状态：</td><td>2025-03-02 认定通过</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	detail, detailErr := parseSecondClassCreditApplicationPage(document, "http://xk.csust.edu.cn/jsxsd/pyfa/cxxfsb_shyj?cxxf04id=APP001")
	if detailErr != nil || detail["project_time"] != "2025-02-27" || detail["review_history"] != "2025-02-28 申请；2025-03-01 审核通过" || detail["recognition_history"] != "2025-03-02 认定通过" {
		t.Fatalf("unexpected second-class credit application detail: %#v %v", detail, detailErr)
	}
}

func TestAcademicSecondClassCreditApplicationUsesSemanticID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/jsxsd/pyfa/cxxfsb_shyj" || request.URL.Query().Get("cxxf04id") != "APP001" {
			t.Fatalf("unexpected second-class credit application request: %s", request.URL.String())
		}
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(`<table><tr><td>项目获得时间：</td><td>2025-02-27</td></tr><tr><td>审核状态：</td><td>审核通过</td></tr><tr><td>认定状态：</td><td>认定通过</td></tr></table>`))
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	result, err := (NativeSite{}).academicSecondClassCreditApplication(context.Background(), []string{"--id", "APP001"})
	if err != nil || result["application_id"] != "APP001" || result["project_time"] != "2025-02-27" {
		t.Fatalf("unexpected second-class credit application result: %#v %v", result, err)
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
	if len(rows) != 1 || rows[0]["title"] != "选课通知" || rows[0]["category"] != "通知公告" || rows[0]["sender"] != "教务处" || rows[0]["detail_path"] != "/jsxsd/ggly/ggly_show?ggid=ABC" || rows[0]["announcement_id"] != "ABC" {
		t.Fatalf("unexpected announcement row: %#v", rows)
	}
}

func TestAcademicAnnouncementDetailUsesSemanticID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/jsxsd/ggly/ggly_show" || request.URL.Query().Get("ggid") != "ABC" {
			t.Fatalf("unexpected announcement request: %s", request.URL.String())
		}
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(`<html><head><title>选课通知</title></head><body><div>请按通知完成选课。</div></body></html>`))
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	result, err := (NativeSite{}).academicAnnouncement(context.Background(), []string{"--id", "ABC"})
	if err != nil || result["announcement_id"] != "ABC" || !strings.Contains(result["content"].(string), "完成选课") {
		t.Fatalf("unexpected announcement detail: %#v %v", result, err)
	}
}

func TestAcademicMessageRowsUseMessageID(t *testing.T) {
	document, err := parsePage(`<table><tr><th>序号</th><th>标题</th><th>类别</th><th>发送人</th><th>发送时间</th><th>操作</th></tr><tr><td>1</td><td>留言标题</td><td>留言</td><td>教务处</td><td>2026-09-11</td><td><a href="javascript:openWindow('/jsxsd/ggly/ggly_show?dpt=ly&amp;ggid=MSG')">查看</a></td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	rows := academicMessageRows(document, "", "http://xk.csust.edu.cn/jsxsd/ggly/ysly_query")
	if len(rows) != 1 || rows[0]["title"] != "留言标题" || rows[0]["message_id"] != "MSG" || rows[0]["announcement_id"] != nil {
		t.Fatalf("unexpected message row: %#v", rows)
	}
}

func TestAcademicMessageDetailUsesSemanticID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/jsxsd/ggly/ggly_show" || request.URL.Query().Get("dpt") != "ly" || request.URL.Query().Get("ggid") != "MSG" {
			t.Fatalf("unexpected message request: %s", request.URL.String())
		}
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(`<html><head><title>公告留言</title></head><body><div>留言内容。</div></body></html>`))
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	result, err := (NativeSite{}).academicMessage(context.Background(), []string{"--id", "MSG"})
	if err != nil || result["message_id"] != "MSG" || !strings.Contains(result["content"].(string), "留言内容") {
		t.Fatalf("unexpected message detail: %#v %v", result, err)
	}
}

func TestAcademicMessageReplyRequiresServerSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/jsxsd/ggly/ggly_show":
			if request.URL.Query().Get("dpt") != "ly" || request.URL.Query().Get("ggid") != "MSG" {
				t.Fatalf("unexpected message detail request: %s", request.URL.String())
			}
			_, _ = writer.Write([]byte(`<html><body>留言详情</body></html>`))
		case "/jsxsd/ggly/lyhf_save":
			if err := request.ParseForm(); err != nil || request.Form.Get("ggid") != "MSG" || request.Form.Get("xmms") != "回复内容" {
				t.Fatalf("unexpected reply form: %v %#v", err, request.Form)
			}
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = writer.Write([]byte(`<script>alert('回复成功！')</script>`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	result, err := (NativeSite{}).academicMessageReply(context.Background(), []string{"--id", "MSG", "--content", "回复内容", "--yes"})
	if err != nil || result["submitted"] != true || result["confirmed"] != true || result["evidence"] != "reply-response-confirmed" || result["content_length"] != 4 {
		t.Fatalf("unexpected message reply: %#v %v", result, err)
	}
}

func TestAcademicRetakeRowsKeepEligibilityAndCourseID(t *testing.T) {
	document, err := parsePage(`<table><tr><th>序号</th><th>是否报名</th><th>上课院审</th><th>开课院审</th><th>取得资格</th><th>学年学期</th><th>开课学期</th><th>课程名称</th><th>学时</th><th>学分</th><th>最好成绩</th><th>替代课程编号</th><th>替代课程名称</th><th>替代课程学时</th><th>替代课程学分</th><th>是否选课</th><th>是否收费</th><th>是否缴费</th><th>重修报名类别</th><th>操作</th></tr><tr><td>1</td><td>×</td><td>√</td><td>√</td><td>×</td><td>2026-2027-1</td><td>2025-2026-1</td><td>线性代数</td><td>40</td><td>2.5</td><td>46</td><td>×</td><td>×</td><td>×</td><td>×</td><td>√</td><td>×</td><td>×</td><td>必选</td><td>+</td></tr><tr><td colspan="20">课程编号:0701001215; 考试性质:重修一</td></tr><tr><td>2</td><td>×</td><td>√</td><td>√</td><td>×</td><td>2026-2027-1</td><td>2025-2026-1</td><td>大学物理</td><td>32</td><td>2</td><td>60</td><td>×</td><td>×</td><td>×</td><td>×</td><td>√</td><td>×</td><td>×</td><td>必选</td><td>+</td></tr><tr><td colspan="20">课程编号:0702000405; 考试性质:重修一</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	rows := academicRetakeRows(document, "线性")
	if len(rows) != 1 || rows[0]["course"] != "线性代数" || rows[0]["eligible"] != "×" || rows[0]["best_score"] != "46" || rows[0]["registration_type"] != "必选" || rows[0]["course_id"] != "0701001215" {
		t.Fatalf("unexpected retake row: %#v", rows)
	}
}
