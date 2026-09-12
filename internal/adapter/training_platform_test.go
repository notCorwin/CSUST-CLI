package adapter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrainingPlatformListAndDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch request.URL.Path {
		case "/article/list":
			if request.URL.Query().Get("cid") != "42" || request.URL.Query().Get("page") != "2" {
				t.Fatalf("training category or page was not mapped: %s", request.URL.RawQuery)
			}
			_, _ = fmt.Fprint(writer, `<ul class="m-newlist"><li><a href="/article/detail?id=8001">研修班开班</a><span class="date">2025-12-08</span></li><li><a href="/article/detail?id=8000">培训通知</a><span class="date">2025-11-17</span></li></ul><a href="/article/list?cid=42&page=3">3</a>`)
		case "/article/detail":
			if request.URL.Query().Get("id") != "8001" {
				t.Fatalf("training detail id was not mapped: %s", request.URL.RawQuery)
			}
			_, _ = fmt.Fprint(writer, `<html><head><title>研修班开班</title></head><body><div class="info-content"><h1>研修班开班</h1><span>发布日期：2025-12-08</span><div class="info-content padding-top-20">正文内容<a href="/file.pdf">附件</a></div></div></body></html>`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	list := runIssueJSON(t, "training-platform", "list", "--category", "news", "--page", "2")
	if list["confirmed"] != true || list["category"] != "news" || list["total_pages"] != float64(3) {
		t.Fatalf("training list was not mapped: %#v", list)
	}
	items, ok := list["data"].([]any)
	if !ok || len(items) != 2 || items[0].(map[string]any)["published_at"] != "2025-12-08" {
		t.Fatalf("training list items were not parsed: %#v", list)
	}

	detail := runIssueJSON(t, "training-platform", "detail", "--id", "8001")
	if detail["title"] != "研修班开班" || detail["published_at"] != "2025-12-08" || !strings.Contains(detail["content"].(string), "正文内容") {
		t.Fatalf("training detail was not parsed: %#v", detail)
	}
}
