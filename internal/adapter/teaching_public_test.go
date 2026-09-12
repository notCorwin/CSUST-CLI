package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
)

func TestTeachingPublicQueries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch request.URL.Path {
		case "/meol/course.do":
			if request.Method == http.MethodPost {
				if request.FormValue("s_keyword") != "数学" {
					t.Fatalf("course keyword was not submitted: %q", request.FormValue("s_keyword"))
				}
				if request.URL.Query().Get("s_gotopage") == "" {
					_, _ = writer.Write([]byte(teachingPublicCourseHTML("数学", "M001", "王老师", 2)))
					return
				}
			}
			if request.URL.Query().Get("s_gotopage") == "2" {
				_, _ = writer.Write([]byte(teachingPublicCourseHTML("高等数学", "M002", "李老师", 2)))
				return
			}
			_, _ = writer.Write([]byte(`<form action="./course.do" method="post"><input type="hidden" name="searchtoken" value="token"><input type="hidden" name="deptId" value="0"><input type="hidden" name="s_keywordrealation" value="0"><input name="s_keyword"></form>`))
		case "/meol/teacher.do":
			_, _ = writer.Write([]byte(`<form action="./teacher.do" method="post"><input type="hidden" name="searchtoken" value="token"><input type="hidden" name="s_keywordrealation" value="0"><input type="hidden" name="deptId" value="0"><input name="s_keyword"></form><div class="teanamewrap"><ul><li><a href="./teacherLesson.do?uid=7"><div class="teainfo">王老师<p>数学学院</p></div></a></li></ul></div><div class="navigation">共<b>1</b>页</div>`))
		case "/meol/teacherLesson.do":
			_, _ = writer.Write([]byte(`<a href="/meol/homepage/course/course_index.jsp?courseId=8">高等数学<br>课程编号:M002<br>主讲教师: 王老师</a>`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "theol.cookies"))

	handled, stdout, _, code, err := (NativeSite{}).Run(context.Background(), []string{"teaching", "public-courses", "--keyword", "数学", "--page", "2", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("public courses: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	var courses map[string]any
	if err := json.Unmarshal(stdout, &courses); err != nil {
		t.Fatal(err)
	}
	items := courses["data"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["course_code"] != "M002" || courses["total_pages"] != float64(2) {
		t.Fatalf("unexpected public courses: %#v", courses)
	}

	handled, stdout, _, code, err = (NativeSite{}).Run(context.Background(), []string{"teaching", "public-teachers", "--keyword", "王", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("public teachers: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	var teachers map[string]any
	if err := json.Unmarshal(stdout, &teachers); err != nil {
		t.Fatal(err)
	}
	teacherItems := teachers["data"].([]any)
	if len(teacherItems) != 1 || teacherItems[0].(map[string]any)["id"] != "7" {
		t.Fatalf("unexpected public teachers: %#v", teachers)
	}

	handled, stdout, _, code, err = (NativeSite{}).Run(context.Background(), []string{"teaching", "public-teacher", "--id", "7", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("public teacher: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	var teacher map[string]any
	if err := json.Unmarshal(stdout, &teacher); err != nil {
		t.Fatal(err)
	}
	if len(teacher["data"].([]any)) != 1 {
		t.Fatalf("unexpected teacher courses: %#v", teacher)
	}
}

func teachingPublicCourseHTML(title, code, teacher string, pages int) string {
	return `<div class="blockPicText"><div class="blockbody"><ul><li><a class="wrapa" href="/meol/homepage/course/course_index.jsp?courseId=1"><div class="text"><h5 class="name">` + title + `</h5><div class="content">课程编号：` + code + `<br>主讲教师：` + teacher + `</div></div></a></li></ul></div></div><div class="navigation">共<b>` + strconv.Itoa(pages) + `</b>页</div>`
}
