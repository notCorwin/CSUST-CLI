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
