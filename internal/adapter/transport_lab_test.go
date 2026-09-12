package adapter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTransportLabProtocols(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/Login/Index":
			_, _ = fmt.Fprint(writer, `<input id="UserName"><input id="Code"><span>请输入手机号码</span><span>请输入验证码</span>`)
		case request.Method == http.MethodGet && request.URL.Path == "/Login/Forget":
			_, _ = fmt.Fprint(writer, `<input id="MemberName">`)
		case request.Method == http.MethodGet && request.URL.Path == "/Login/Register":
			_, _ = fmt.Fprint(writer, `<input id="MemberType">`)
		case request.Method == http.MethodPost && request.URL.Path == "/Login/CheckLogin":
			if err := request.ParseForm(); err != nil || request.Form.Get("role") != "2" || request.Form.Get("username") != "13800138000" || request.Form.Get("password") != "secret" || request.Form.Get("code") != "1234" {
				t.Fatalf("transport-lab login fields were not mapped: %v", request.Form)
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"state":"success"}`)
		case request.Method == http.MethodGet && request.URL.Path == "/Home/Index":
			_, _ = fmt.Fprint(writer, `<html><body>预约首页</body></html>`)
		case request.Method == http.MethodPost && request.URL.Path == "/Login/ForgetPwdQuestion":
			if err := request.ParseForm(); err != nil || request.Form.Get("keyValue") != "13800138000" {
				t.Fatalf("transport-lab question fields were not mapped: %v", request.Form)
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"state":"success","message":"母亲生日"}`)
		case request.Method == http.MethodPost && request.URL.Path == "/Login/CheckForget":
			if err := request.ParseForm(); err != nil || request.Form.Get("membername") != "13800138000" || request.Form.Get("memberpwdanswer") != "答案" || request.Form.Get("memberpwd") != "NewSecret!1" {
				t.Fatalf("transport-lab reset fields were not mapped: %v", request.Form)
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"state":"success","message":"修改成功"}`)
		case request.Method == http.MethodPost && request.URL.Path == "/Login/CheckRegister":
			if err := request.ParseForm(); err != nil || request.Form.Get("usertype") != "3" || request.Form.Get("fullname") != "测试用户" || request.Form.Get("usersex") != "女" || request.Form.Get("username") != "13800138000" || request.Form.Get("userpwd") != "secret" || !strings.HasPrefix(request.Form.Get("userimage"), "data:image/png;base64,") || !strings.HasPrefix(request.Form.Get("usercard"), "data:image/png;base64,") {
				t.Fatalf("transport-lab register fields were not mapped: %v", request.Form)
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"state":"success","message":"注册成功"}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "photo.png"), []byte("\x89PNG\r\n\x1a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "card.png"), []byte("\x89PNG\r\n\x1a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(root, "cookies.txt"))

	login := runIssueJSON(t, "transport-lab", "login", "--role", "teacher", "--username", "13800138000", "--password", "secret", "--captcha", "1234")
	if login["confirmed"] != true || login["role"] != "2" {
		t.Fatalf("transport-lab login was not confirmed: %#v", login)
	}
	question := runIssueJSON(t, "transport-lab", "getpwdquestion", "--phone", "13800138000")
	if question["question"] != "母亲生日" || question["confirmed"] != true {
		t.Fatalf("transport-lab password question was not returned: %#v", question)
	}
	forgot := runIssueJSON(t, "transport-lab", "forgot", "--yes", "--phone", "13800138000", "--answer", "答案", "--new-password", "NewSecret!1", "--password-confirm", "NewSecret!1")
	if forgot["confirmed"] != true || forgot["password"] != nil {
		t.Fatalf("transport-lab password reset was not confirmed or leaked a password: %#v", forgot)
	}
	register := runIssueJSON(t, "transport-lab", "register", "--yes", "--type", "3", "--name", "测试用户", "--sex", "女", "--phone", "13800138000", "--password", "secret", "--password-confirm", "secret", "--question", "母亲生日", "--answer", "答案", "--photo", filepath.Join(root, "photo.png"), "--card", filepath.Join(root, "card.png"))
	if register["confirmed"] != true || register["password"] != nil {
		t.Fatalf("transport-lab registration was not confirmed or leaked a password: %#v", register)
	}
}
