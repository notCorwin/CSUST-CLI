package adapter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestUndergraduateAdmissionsAPIsMapSemanticQueries(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/static/front/csust/basic/html_web/") {
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(writer, `<html><head><title>招生计划</title></head><body>招生计划</body></html>`)
			return
		}
		if request.Method != http.MethodPost {
			http.NotFound(writer, request)
			return
		}
		calls = append(calls, request.URL.Path)
		if request.Header.Get("X-Requested-With") != "XMLHttpRequest" || request.Header.Get("X-Requested-Time") == "" {
			t.Fatalf("missing browser request headers: %#v", request.Header)
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/f/ajax_get_csrfToken":
			_, _ = fmt.Fprint(writer, `{"state":1,"msg":"操作成功","data":"test-token,test-token,test-token","jessionid":"do-not-output"}`)
		case "/f/ajax_zsjh":
			if err := request.ParseForm(); err != nil || request.Form.Get("ssmc") != "湖南" || request.Form.Get("zsnf") != "2026" || request.Form.Get("klmc") != "物理类" || request.Form.Get("zslx") != "普通类" {
				t.Fatalf("semantic plan fields were not mapped: %v", request.Form)
			}
			_, _ = fmt.Fprint(writer, `{"state":1,"data":{"zsjhTotal":[{"ssmc":"湖南","nf":"2026","klmc":"物理类","zslx":"普通类","zsjhs":100}],"zsjhList":[{"ssmc":"湖南","nf":"2026","klmc":"物理类","zslx":"普通类","zymc":"土木工程","zydm":"081001","zsjhs":80,"yxdm":"土木工程学院","xkkm":"物理,化学","zycc":"本科批"}]}}`)
		case "/f/ajax_lnfs":
			_, _ = fmt.Fprint(writer, `{"state":1,"data":{"zsSsgradeList":[{"ssmc":"湖南","nf":"2025","klmc":"物理类","zslx":"普通类","zymc":"土木工程","maxScore":"600","minScore":"580","avgScore":"590","maxRank":"10","minRank":"20","avgRank":"15","pcmc":"本科批","rs":80}]}}`)
		case "/f/ajax_lqjc":
			_, _ = fmt.Fprint(writer, `{"state":1,"data":[{"province":"湖南","zsnf":"2026","klmc":"物理类","zslx":"普通类","lqzt":"录取结束","jssj":"2026-07-21","maxScore":"631","minScore":"434","avgScore":"570.27","count":6979}]}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	plans := runIssueJSON(t, "undergraduate-admissions", "plans", "--province", "湖南", "--year", "2026", "--category", "物理类", "--type", "普通类")
	if plans["operation"] != "plans" || len(plans["items"].([]any)) != 1 || len(plans["summary"].([]any)) != 1 {
		t.Fatalf("plan response was not semantic: %#v", plans)
	}
	plan := plans["items"].([]any)[0].(map[string]any)
	if plan["major"] != "土木工程" || plan["plan_count"] != float64(80) || plan["raw"] == nil {
		t.Fatalf("plan fields were not preserved: %#v", plan)
	}

	scores := runIssueJSON(t, "undergraduate-admissions", "scores", "--province", "湖南", "--year", "2025", "--category", "物理类", "--type", "普通类", "--major", "土木工程")
	if len(scores["items"].([]any)) != 1 || scores["filters"].(map[string]any)["major"] != "土木工程" {
		t.Fatalf("score response was not semantic: %#v", scores)
	}

	progress := runIssueJSON(t, "undergraduate-admissions", "progress", "--province", "湖南", "--year", "2026", "--category", "物理类", "--type", "普通类")
	item := progress["items"].([]any)[0].(map[string]any)
	if item["status"] != "录取结束" || item["count"] != float64(6979) {
		t.Fatalf("progress response was not mapped: %#v", item)
	}

	if len(calls) != 6 || !strings.Contains(strings.Join(calls, ","), "/f/ajax_zsjh") || !strings.Contains(strings.Join(calls, ","), "/f/ajax_lnfs") || !strings.Contains(strings.Join(calls, ","), "/f/ajax_lqjc") {
		t.Fatalf("unexpected API call count: %#v", calls)
	}
}
