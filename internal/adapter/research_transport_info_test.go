package adapter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestResearchLoginProtocolAndLogout(t *testing.T) {
	loggedIn := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/":
			writer.Header().Set("Content-Type", "text/html; charset=gb2312")
			if loggedIn {
				_, _ = fmt.Fprint(writer, `<html><body>科研管理首页</body></html>`)
			} else {
				_, _ = fmt.Fprint(writer, `<html><body><form><input type="hidden" name="__VIEWSTATE" value="view"><input type="hidden" name="__EVENTVALIDATION" value="event"><input name="loginbutton" value="登录"><input id="txtLoginName"><input id="txtPassWord"></form></body></html>`)
			}
		case request.Method == http.MethodGet && request.URL.Path == "/AjaxAshx/EncryptString.ashx":
			if request.URL.Query().Get("strInput") != "account|1234" {
				t.Fatalf("research encryption input was not mapped: %s", request.URL.RawQuery)
			}
			writer.Header().Set("Content-Type", "text/plain")
			_, _ = fmt.Fprint(writer, "h1|h3")
		case request.Method == http.MethodPost && request.URL.Path == "/Login.aspx":
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if request.Form.Get("h1") != "h1" || request.Form.Get("h3") != "h3" || request.Form.Get("h2") != md5Hex("secret") || request.Form.Get("hRoleType") != "01" {
				t.Fatalf("research login fields were not mapped: %v", request.Form)
			}
			loggedIn = true
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(writer, `<html><body>科研管理首页</body></html>`)
		case request.Method == http.MethodPost && request.URL.Path == "/AjaxAshx/LoginOut.ashx":
			loggedIn = false
			writer.Header().Set("Content-Type", "text/plain")
			_, _ = fmt.Fprint(writer, "ok")
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	login := runIssueJSON(t, "research", "login", "--role", "researcher", "--username", "account", "--password", "secret", "--captcha", "1234")
	if login["confirmed"] != true || login["role"] != "01" || login["username"] != "account" {
		t.Fatalf("research login was not confirmed: %#v", login)
	}
	logout := runIssueJSON(t, "research", "logout", "--yes")
	if logout["confirmed"] != true || !logout["logged_out"].(bool) || loggedIn {
		t.Fatalf("research logout was not confirmed: %#v", logout)
	}
}

func TestTransportInfoLoginProtocol(t *testing.T) {
	loggedIn := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/Login/Index":
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(writer, `<html><body><h3>系统登录</h3><input id="UserName"><input id="PassWord"></body></html>`)
		case request.Method == http.MethodPost && request.URL.Path == "/Login/CheckLogin":
			if err := request.ParseForm(); err != nil || request.Form.Get("username") != "account" || request.Form.Get("password") != "secret" || request.Form.Get("code") != "1234" {
				t.Fatalf("transport-info login fields were not mapped: %v", request.Form)
			}
			loggedIn = true
			// The live endpoint returns JSON with an HTML content type.
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(writer, `{"state":"success"}`)
		case request.Method == http.MethodGet && request.URL.Path == "/Home/Index":
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			if loggedIn {
				_, _ = fmt.Fprint(writer, `<html><body>综合信息服务首页</body></html>`)
			} else {
				_, _ = fmt.Fprint(writer, `<html><body>系统登录 UserName PassWord</body></html>`)
			}
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	login := runIssueJSON(t, "transport-info", "login", "--username", "account", "--password", "secret", "--captcha", "1234")
	if login["confirmed"] != true || login["username"] != "account" || !strings.Contains(login["evidence"].(string), "CheckLogin") {
		t.Fatalf("transport-info login was not confirmed: %#v", login)
	}
}
