package adapter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestLibraryRemoteDatabaseQueries(t *testing.T) {
	var listForm map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch request.URL.Path {
		case "/":
			_, _ = fmt.Fprint(writer, `<button value="">全部</button><button value="ed42cf22f6cd">工学</button><button value="A">A</button>`)
		case "/accessData":
			if request.Method != http.MethodPost {
				t.Errorf("database list used %s", request.Method)
			}
			if err := request.ParseForm(); err != nil {
				t.Errorf("database list form is invalid: %v", err)
			} else {
				listForm = map[string]string{
					"elecName": request.FormValue("elecName"), "sortType": request.FormValue("sortType"),
					"elecMuTypes": request.FormValue("elecMuTypes"),
				}
			}
			_, _ = fmt.Fprint(writer, `<script>$("#totalCount").text([{"id":1,"elecName":"测试库","elecUrl":"https://db.example","remarks":"<p>说明</p>","companyName":"测试提供商","typeZm":"T","stat":12,"lastDate":"2026-09-12","serviceType":"1","isFree":"0","osPassWord":"private-secret"}].length)</script>`)
		case "/detail":
			if request.URL.Query().Get("id") != "1" {
				t.Errorf("unexpected database id: %s", request.URL.Query().Get("id"))
			}
			_, _ = fmt.Fprint(writer, `<div class="detail-main"><h4>测试库</h4><ul class="detail-list"><li><div class="dt">访问地址：</div><div class="dd"><a onclick="jinru('https://db.example','1')">进入</a></div></li><li><div class="dt">首页访问量：</div><div class="dd">1,234</div></li><li><div class="dt">首字母：</div><div class="dd">T</div></li><li><div class="dt">资源类型：</div><div class="dd">期刊, 论文</div></li><li><div class="dt">学科：</div><div class="dd">工学, 理学</div></li><li><div class="dt">语种：</div><div class="dd">中文, 英文</div></li><li><div class="dt">资源简介：</div><div class="dd"><p>详情说明</p></div></li></ul></div>`)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "library.cookies.txt"))

	list := runIssueJSON(t, "library-remote", "databases", "--keyword", "知网", "--sort", "visits")
	if list["total"] != float64(1) || strings.Contains(string(mustMarshalIssue(list)), "private-secret") {
		t.Fatalf("database list model/redaction failed: %#v", list)
	}
	item := list["data"].([]any)[0].(map[string]any)
	if item["name"] != "测试库" || item["description"] != "说明" || item["visits"] != float64(12) {
		t.Fatalf("database list fields failed: %#v", item)
	}
	if listForm["elecName"] != "知网" || listForm["sortType"] != "0" || listForm["elecMuTypes"] != "" {
		t.Fatalf("database list form mapping failed: %#v", listForm)
	}

	subjectList := runIssueJSON(t, "library-remote", "databases", "--subject", "工学")
	if listForm["elecMuTypes"] != "ed42cf22f6cd" || subjectList["filters"].(map[string]any)["subject"] != "工学" {
		t.Fatalf("subject filter mapping failed: %#v form=%#v", subjectList, listForm)
	}

	detail := runIssueJSON(t, "library-remote", "database", "--id", "1")
	database := detail["database"].(map[string]any)
	if database["access_url"] != "https://db.example" || database["visits"] != float64(1234) || database["description"] != "详情说明" {
		t.Fatalf("database detail model failed: %#v", database)
	}
	if len(database["subjects"].([]any)) != 2 || database["subjects"].([]any)[0] != "工学" {
		t.Fatalf("database detail subjects failed: %#v", database)
	}
}
