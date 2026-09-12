package adapter

import "testing"

func TestAcademicLabBookingRowsExposeSemanticEntries(t *testing.T) {
	document, err := parsePage(`<table><tr><th>课程编号</th><th>课程名称</th><th>容量</th><th>余量</th><th>授课教师</th><th>上课时间</th><th>学时</th><th>学分</th><th>操作</th></tr><tr><td>LAB001</td><td>交通工程实验</td><td>30</td><td>12</td><td>张老师</td><td>周一1-2节</td><td>2</td><td>1</td><td><a href="javascript:void(0)" onclick="toyy('COURSE-1')">预约</a></td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	rows := academicLabBookingRows(document, "交通工程", "2026-2027-1", false, false)
	if len(rows) != 1 || rows[0]["course_id"] != "LAB001" || rows[0]["remaining"] != "12" || rows[0]["booking_id"] != "COURSE-1" || rows[0]["booking_path"] != "/jsxsd/syjx/toSyYy.do?sj0403id=COURSE-1&xnxq01id=" {
		t.Fatalf("unexpected lab rows: %#v", rows)
	}
}

func TestAcademicOpenLabSelectedRowsExposeCancelEntry(t *testing.T) {
	document, err := parsePage(`<table><tr><th>项目名称</th><th>批次</th><th>批次名称</th><th>项目性质</th><th>授课教师</th><th>周次节次</th><th>地点</th><th>操作</th></tr><tr><td>开放实验</td><td>1</td><td>第一批</td><td>开放</td><td>李老师</td><td>第3周</td><td>A101</td><td><a href="javascript:void(0)" onclick="doyy('SELECT-1',this)">退选</a></td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	rows := academicLabBookingRows(document, "", "2026-2027-1", true, true)
	if len(rows) != 1 || rows[0]["project"] != "开放实验" || rows[0]["selection_id"] != "SELECT-1" || rows[0]["cancel_path"] != "/jsxsd/syjx/toQxXm.do?sj0501id=SELECT-1&xnxq=2026-2027-1" {
		t.Fatalf("unexpected selected lab rows: %#v", rows)
	}
}
