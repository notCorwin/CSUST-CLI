package adapter

import "testing"

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
