package adapter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnionCatalogLoginAndLogout(t *testing.T) {
	var loggedIn bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/front/user.do" && request.URL.Query().Get("dispatch") == "toajaxlogin":
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(writer, `<form><input type="hidden" name="sid" value="2"><input type="hidden" name="login_type" value="0"><input type="hidden" name="proposal_type" value="1"></form>`)
		case request.Method == http.MethodPost && request.URL.Path == "/front/user.do" && request.URL.Query().Get("dispatch") == "ajaxlogin":
			if err := request.ParseForm(); err != nil || request.Form.Get("admin_id") != "account" || request.Form.Get("identifying_code") != "1234" {
				t.Fatalf("union login fields were not mapped: %v", request.Form)
			}
			expected, _ := unionEncryptedPassword("secret")
			if request.Form.Get("admin_pwd") != expected {
				t.Fatalf("union DES password was not mapped: got %q want %q", request.Form.Get("admin_pwd"), expected)
			}
			loggedIn = true
			writer.Header().Set("Content-Type", "text/plain")
			_, _ = fmt.Fprint(writer, "0")
		case request.Method == http.MethodGet && request.URL.Path == "/center/center.do" && request.URL.Query().Get("dispatch") == "getCenterSessionName":
			writer.Header().Set("Content-Type", "text/plain")
			if loggedIn {
				_, _ = fmt.Fprint(writer, "工会会员")
			}
		case request.Method == http.MethodGet && request.URL.Path == "/logout.jsp":
			loggedIn = false
		case request.Method == http.MethodGet && request.URL.Path == "/front/page.do":
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(writer, `<html><body>智慧工会</body></html>`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	catalog := runIssueJSON(t, "union", "modules")
	if len(catalog["modules"].([]any)) != 8 || len(catalog["roles"].([]any)) != 5 {
		t.Fatalf("union catalog was incomplete: %#v", catalog)
	}

	login := runIssueJSON(t, "union", "login", "--role", "representative", "--username", "account", "--password", "secret", "--captcha", "1234")
	if login["confirmed"] != true || login["role"] != "representative" || login["session"] != "工会会员" {
		t.Fatalf("union login evidence was incomplete: %#v", login)
	}
	if strings.Contains(string(mustJSON(login)), "secret") {
		t.Fatal("union password leaked in result")
	}

	logout := runIssueJSON(t, "union", "logout", "--yes")
	if logout["confirmed"] != true || logout["logged_out"] != true || loggedIn {
		t.Fatalf("union logout was not confirmed: %#v", logout)
	}
}

func TestUnionPublicOrganizationDirectoryAndDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		query := request.URL.Query()
		switch {
		case query.Get("dispatch") == "listByType_" && query.Get("ntype_id") == "0903":
			_, _ = fmt.Fprint(writer, `<a class="GH-mian-card1" href="/front/news.do?dispatch=shetuanMain&amp;ntype_id=090302"><div class="text1">土木与环境工程学院</div></a><a class="GH-mian-card1" href="/front/news.do?dispatch=shetuanMain&amp;ntype_id=090301"><div class="text1">交通学院</div></a>`)
		case query.Get("dispatch") == "listByType_" && query.Get("ntype_id") == "0901":
			_, _ = fmt.Fprint(writer, `<a class="GH-mian-card1" href="/front/news.do?dispatch=shetuanMain&amp;ntype_id=090101"><div class="text1">篮球协会</div></a>`)
		case query.Get("dispatch") == "shetuanMain" && query.Get("ntype_id") == "090301":
			_, _ = fmt.Fprint(writer, `<div class="userbox-text1">交通学院</div><div class="pc-user-box2"><div class="info-text1">所辖部门：</div><div class="info-text2">交通学院</div></div><div class="pc-user-box2"><div class="info-text1">分工会主席：</div><div class="info-text2">叶群山</div></div>`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	all := runIssueJSON(t, "union", "organizations")
	if all["total"] != float64(3) {
		t.Fatalf("union organization directory was incomplete: %#v", all)
	}
	branches := runIssueJSON(t, "union", "branches", "--keyword", "交通")
	items := branches["data"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != "090301" {
		t.Fatalf("union branch filter was not mapped: %#v", branches)
	}
	detail := runIssueJSON(t, "union", "organization", "--id", "090301")
	data := detail["data"].(map[string]any)
	if data["name"] != "交通学院" || data["department"] != "交通学院" || data["leader"] != "叶群山" {
		t.Fatalf("union organization detail was not parsed: %#v", detail)
	}
}
