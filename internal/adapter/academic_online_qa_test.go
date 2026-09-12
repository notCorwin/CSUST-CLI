package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcademicOnlineQARowsExtractSemanticFieldsAndID(t *testing.T) {
	document, err := parsePage(`<table><tr><th>序号</th><th>发送人</th><th>发送单位</th><th>发送类型</th><th>发送时间</th><th>问题内容</th><th>热点问题</th><th>是否回复</th><th>有效时间</th><th>失效时间</th><th>操作</th></tr><tr><td>1</td><td>学生</td><td>教务处</td><td>咨询</td><td>2026-09-12</td><td>问题内容</td><td>否</td><td>否</td><td>2026-09-12</td><td>2026-09-30</td><td><a href="javascript:sc('QA-1',this)">删除</a></td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	rows := academicOnlineQARows(document, "问题", "http://xk.csust.edu.cn/jsxsd/zxwd/zxwd_opt")
	if len(rows) != 1 || rows[0]["id"] != "QA-1" || rows[0]["sender"] != "学生" || rows[0]["content"] != "问题内容" {
		t.Fatalf("unexpected online QA row: %#v", rows)
	}
}

func TestAcademicOnlineQAAskAndDeleteUseServerFeedbackAndReadback(t *testing.T) {
	var askContent string
	listCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch request.URL.Path {
		case academicOnlineQAAddPath:
			_, _ = writer.Write([]byte(`<form method="post" action="/jsxsd/zxwd/tw_add_save"><textarea name="xmms"></textarea><button>确 认</button></form>`))
		case academicOnlineQASavePath:
			if err := request.ParseForm(); err != nil {
				t.Fatalf("parse ask form: %v", err)
			}
			askContent = request.Form.Get("xmms")
			_, _ = writer.Write([]byte(`<script>alert('保存成功')</script>`))
		case academicOnlineQAPath:
			if request.Method == http.MethodPost {
				if err := request.ParseForm(); err != nil || request.Form.Get("oaid") != "QA-1" {
					t.Fatalf("unexpected delete form: %v %#v", err, request.Form)
				}
				_, _ = writer.Write([]byte(`<script>alert('删除成功')</script>`))
				return
			}
			listCalls++
			if listCalls == 1 {
				_, _ = writer.Write([]byte(`<table><tr><th>序号</th><th>发送人</th><th>发送单位</th><th>发送类型</th><th>发送时间</th><th>问题内容</th><th>热点问题</th><th>是否回复</th><th>有效时间</th><th>失效时间</th><th>操作</th></tr><tr><td>1</td><td>学生</td><td>教务处</td><td>咨询</td><td>2026-09-12</td><td>问题内容</td><td>否</td><td>否</td><td>2026-09-12</td><td>2026-09-30</td><td><a href="javascript:sc('QA-1',this)">删除</a></td></tr></table>`))
			} else {
				_, _ = writer.Write([]byte(`<table><tr><th>序号</th><th>问题内容</th></tr><tr><td>未查询到数据</td></tr></table>`))
			}
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	handled, output, _, code, err := (NativeSite{}).Run(context.Background(), []string{"online-qa", "ask", "--content", "问题内容", "--yes", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("ask: handled=%v code=%d err=%v output=%s", handled, code, err, output)
	}
	var askResult map[string]any
	if err := json.Unmarshal(output, &askResult); err != nil {
		t.Fatal(err)
	}
	if askContent != "问题内容" || askResult["submitted"] != true || askResult["confirmed"] != true || askResult["evidence"] != "server-success-and-list-readback" {
		t.Fatalf("unexpected ask result: %#v content=%q", askResult, askContent)
	}

	listCalls = 0
	handled, output, _, code, err = (NativeSite{}).Run(context.Background(), []string{"online-qa", "delete", "--id", "QA-1", "--yes", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("delete: handled=%v code=%d err=%v output=%s", handled, code, err, output)
	}
	var deleteResult map[string]any
	if err := json.Unmarshal(output, &deleteResult); err != nil {
		t.Fatal(err)
	}
	if deleteResult["id"] != "QA-1" || deleteResult["submitted"] != true || deleteResult["confirmed"] != true || !strings.Contains(string(output), "server-success-and-list-readback") {
		t.Fatalf("unexpected delete result: %#v", deleteResult)
	}
}
