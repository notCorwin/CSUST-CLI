package adapter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfficialSearchAndArticleAdapters(t *testing.T) {
	var posted bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch request.URL.Path {
		case "/":
			_, _ = fmt.Fprint(writer, `<form method="post" action="/sch.jsp?wbtreeid=1020"><input type="hidden" name="_lucenesearchtype" value="1"><input type="hidden" name="searchScope" value="0"><input name="lucenenewssearchkey"><input name="showkeycode"></form>`)
		case "/sch.jsp":
			if request.Method == http.MethodPost {
				if err := request.ParseForm(); err != nil {
					t.Fatalf("parse official search form: %v", err)
				}
				posted = request.Form.Get("lucenenewssearchkey") == "人工智能" && request.Form.Get("showkeycode") == "人工智能"
				_, _ = fmt.Fprint(writer, `<div>共有 2 条结果，共 2 页</div><table class="listFrame"><tr><td><a href="/info/1299/22951.htm">人工智能通知</a></td></tr><tr><td>第一条摘要</td></tr><tr><td>发表时间:2026年09月09日</td></tr><tr><td><a href="/sch.jsp?wbtreeid=1020&searchScope=0&currentnum=2&newskeycode2=short-token">2</a></td></tr></table>`)
				return
			}
			if request.URL.Query().Get("currentnum") != "2" {
				t.Fatalf("official pagination was not mapped: %s", request.URL.RawQuery)
			}
			_, _ = fmt.Fprint(writer, `<table class="listFrame"><tr><td><a href="/info/1299/22952.htm">第二条通知</a></td></tr><tr><td>第二条摘要</td></tr><tr><td>发表时间:2026年09月08日</td></tr></table>`)
		case "/info/1299/22951.htm":
			_, _ = fmt.Fprint(writer, `<div class="show_title"><h3>人工智能通知</h3><span>发布日期：2026年09月09日 来源：校办</span></div><div class="tot_content"><div class="v_news_content">正文内容<a href="/download/notice.pdf">附件</a></div></div>`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	result := runIssueJSON(t, "official", "search", "--keyword", "人工智能", "--page", "2")
	if !posted || result["confirmed"] != true || result["total"] != float64(2) {
		t.Fatalf("official search was not mapped: posted=%v result=%#v", posted, result)
	}
	items, ok := result["data"].([]any)
	if !ok || len(items) != 1 || items[0].(map[string]any)["id"] != "1299/22952" {
		t.Fatalf("official search results were not parsed: %#v", result)
	}

	article := runIssueJSON(t, "official", "article", "--id", "1299/22951")
	if article["title"] != "人工智能通知" || article["published_at"] != "2026年09月09日" || !strings.Contains(article["content"].(string), "正文内容") {
		t.Fatalf("official article was not parsed: %#v", article)
	}
}
