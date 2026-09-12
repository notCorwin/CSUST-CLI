package adapter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestMoocCourseCatalog(t *testing.T) {
	var searchQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		if request.URL.Path != "/portal/courseNetwork/list" {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		if request.URL.Query().Get("pageNum") != "" {
			searchQuery = request.URL.Query()
		}
		_, _ = fmt.Fprint(writer, `<ul name="department"><li value="0"><a>全部</a></li><li value="20821445"><a>土木与环境工程学院</a></li></ul><form id="courseform"></form><table class="Wmtable"><tr class="Wmtr1"><td>序号</td></tr><tr class="Wmtr2"><td>266845717</td><td></td><td><a class="Limitlogin" href="/fyportal/tomoocportal?courseid=266845717&amp;ckenc=8c424c14a3fb272d376d478b87a6781d" title="结构设计原理课程设计B">结构设计原理课程设计B</a></td><td>土木与环境工程学院</td><td>夏桂云</td><td>15</td><td>2026-09-11</td></tr></table><script>page.showPage(2,65,"turnPage");</script>`)
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "mooc.cookies"))

	departments := runIssueJSON(t, "mooc", "departments")
	if len(departments["data"].([]any)) != 1 || departments["data"].([]any)[0].(map[string]any)["id"] != "20821445" {
		t.Fatalf("MOOC department model failed: %#v", departments)
	}
	courses := runIssueJSON(t, "mooc", "courses", "--keyword", "结构", "--department", "土木与环境工程学院", "--page", "2", "--page-size", "20", "--sort", "views", "--order", "asc")
	if courses["total_pages"] != float64(65) || courses["department_id"] != "20821445" {
		t.Fatalf("MOOC course pagination/filter failed: %#v", courses)
	}
	item := courses["data"].([]any)[0].(map[string]any)
	if item["id"] != "266845717" || item["title"] != "结构设计原理课程设计B" || item["views"] != float64(15) || item["external_platform"] != "chaoxing" {
		t.Fatalf("MOOC course model failed: %#v", item)
	}
	if strings.Contains(fmt.Sprint(item["public_access_url"]), "8c42414a3fb272d376d478b87a6781d") {
		t.Fatal("MOOC signed course URL leaked its access token")
	}
	if searchQuery == nil || searchQuery.Get("keyword") != "结构" || searchQuery.Get("departmentId") != "20821445" || searchQuery.Get("pageSize") != "20" || searchQuery.Get("sort") != "viewTimes" || searchQuery.Get("order") != "1" || !strings.Contains(searchQuery.Get("pageNum"), "2") {
		t.Fatalf("MOOC request mapping failed: %v", searchQuery)
	}
}

func TestMoocCourseDetailUsesDirectoryAccessLink(t *testing.T) {
	var detailSeen bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch request.URL.Path {
		case "/portal/courseNetwork/list":
			_, _ = fmt.Fprint(writer, `<table class="Wmtable"><tr><td>42</td><td><a class="Limitlogin" href="/fyportal/tomoocportal?courseid=42&amp;ckenc=0123456789abcdef0123456789abcdef">课程详情</a></td></tr></table>`)
		case "/fyportal/tomoocportal":
			detailSeen = request.URL.Query().Get("courseid") == "42" && request.URL.Query().Get("ckenc") != ""
			_, _ = fmt.Fprint(writer, `<span class="f30 mb10 wh">课程详情</span><div class="teacherDiv">主讲教师：王老师</div><table><tr><td>学校：</td><td>长沙理工大学</td></tr><tr><td>开课院系：</td><td>交通学院</td></tr><tr><td>课程编号：</td><td>0701000001</td></tr><tr><td>学分：</td><td>2</td></tr><tr><td>课时：</td><td>32</td></tr></table><span id="Topingfen">4.5</span><p>(12 人评价)</p>`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "mooc.cookies"))
	result := runIssueJSON(t, "mooc", "course", "--id", "42")
	if !detailSeen {
		t.Fatal("MOOC course detail did not use the directory access link")
	}
	data, ok := result["data"].(map[string]any)
	if !ok || data["id"] != "42" || data["title"] != "课程详情" || data["instructor"] != "王老师" || data["school"] != "长沙理工大学" || data["course_number"] != "0701000001" || data["rating"] != "4.5" || data["evaluation_count"] != float64(12) {
		t.Fatalf("MOOC course detail model failed: %#v", result)
	}
	if strings.Contains(fmt.Sprint(data["access_url"]), "0123456789abcdef") {
		t.Fatal("MOOC course detail leaked its signed access token")
	}
}
