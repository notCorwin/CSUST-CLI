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
	if searchQuery == nil || searchQuery.Get("keyword") != "结构" || searchQuery.Get("departmentId") != "20821445" || searchQuery.Get("pageSize") != "20" || searchQuery.Get("sort") != "viewTimes" || searchQuery.Get("order") != "1" || !strings.Contains(searchQuery.Get("pageNum"), "2") {
		t.Fatalf("MOOC request mapping failed: %v", searchQuery)
	}
}
