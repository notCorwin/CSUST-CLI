package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeVPNAPIAndTeachingGateway(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/enclient/api/users/info" {
			_, _ = writer.Write([]byte(`{"code":200,"data":{"username":"student"}}`))
			return
		}
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(`<!doctype html><html><head><title>教学平台</title></head><body><a href="/meol/course.do">课程</a></body></html>`))
	}))
	defer server.Close()
	t.Setenv("CSUST_VPN_BASE_URL", server.URL)
	t.Setenv("CSUST_TEACHING_PREFIX", "/http/pt.example")
	t.Setenv("CSUST_VPN_COOKIE_FILE", filepath.Join(t.TempDir(), "vpn.cookies"))
	t.Setenv("CSUST_VPN_SESSION_FILE", filepath.Join(t.TempDir(), "vpn.json"))

	handled, stdout, _, code, err := (NativeSite{}).Run(context.Background(), []string{"vpn", "api", "--path", "/api/users/info", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("vpn api: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout, &payload); err != nil {
		t.Fatal(err)
	}
	response := payload["response"].(map[string]any)
	jsonPayload := response["json"].(map[string]any)
	if jsonPayload["code"].(float64) != 200 {
		t.Fatalf("unexpected vpn response: %#v", jsonPayload)
	}

	handled, stdout, _, code, err = (NativeSite{}).Run(context.Background(), []string{"teaching", "get", "--path", "/meol/index.do", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("teaching get: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	payload = map[string]any{}
	if err := json.Unmarshal(stdout, &payload); err != nil {
		t.Fatal(err)
	}
	page := payload["response"].(map[string]any)
	if page["title"] != "教学平台" || page["kind"] != "html" {
		t.Fatalf("unexpected teaching page: %#v", page)
	}

	if _, err := os.Stat(filepath.Dir(os.Getenv("CSUST_VPN_COOKIE_FILE"))); err != nil {
		t.Fatal(err)
	}
}

func TestNativeVPNLocalLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/enclient/api/users/custom/page/login/cfg/select":
			_, _ = writer.Write([]byte(`{"code":200,"data":{"defaultAuthType":"LOCAL"}}`))
		case "/enclient/api/users/client/auth/generateKey":
			_, _ = writer.Write([]byte(`{"code":200,"data":{"enToken":"a-b-1234567890abcdef"}}`))
		case "/enclient/api/users/auth/login":
			http.SetCookie(writer, &http.Cookie{Name: "vpn_session", Value: "ok", Path: "/"})
			_, _ = writer.Write([]byte(`{"code":200,"data":{"token":"access","refreshToken":"refresh","username":"student"}}`))
		case "/enclient/api/users/info":
			if request.Header.Get("Authorization") != "Bearer access" {
				writer.WriteHeader(http.StatusUnauthorized)
				_, _ = writer.Write([]byte(`{"code":3010,"messages":"expired"}`))
				return
			}
			_, _ = writer.Write([]byte(`{"code":200,"data":{"username":"student"}}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	t.Setenv("CSUST_VPN_BASE_URL", server.URL)
	t.Setenv("CSUST_VPN_COOKIE_FILE", filepath.Join(root, "vpn.cookies"))
	t.Setenv("CSUST_VPN_SESSION_FILE", filepath.Join(root, "vpn.json"))
	t.Setenv("CSUST_USERNAME", "student")
	t.Setenv("CSUST_PASSWORD", "password")

	handled, stdout, _, code, err := (NativeSite{}).Run(context.Background(), []string{"vpn", "login", "--auth", "local", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("vpn login: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != true || payload["auth"] != "local" {
		t.Fatalf("unexpected login result: %#v", payload)
	}
	content, err := os.ReadFile(filepath.Join(root, "vpn.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) == "" {
		t.Fatal("empty session")
	}
	info, err := os.Stat(filepath.Join(root, "vpn.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("session permissions: err=%v mode=%v", err, info.Mode().Perm())
	}
}

func TestNativeQualityEvaluationParsingAndSubmit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch request.URL.Path {
		case "/http/quality.example/jsxsd/xspj/xspj_find.do":
			_, _ = writer.Write([]byte(`<table><tr><th>序号</th><th>学期</th><th>类别</th><th>名称</th><th>开始</th><th>结束</th></tr><tr><td>1</td><td>2026-1</td><td>学生</td><td>评教</td><td>2026-01-01</td><td>2026-02-01</td><td><a href="/jsxsd/xspj/xspj_course.do?id=1">进入</a></td></tr></table>`))
		case "/http/quality.example/jsxsd/xspj/xspj_course.do":
			_, _ = writer.Write([]byte(`<form method="post" action="/jsxsd/xspj/xspj_save.do"><input type="hidden" name="execution" value="secret"><table><tr><td><input name="pj06xh" value="q1">教学质量</td><td><input type="radio" name="q1opt" value="A"><input type="radio" name="q1opt" value="B"></td></tr></table><textarea name="suggestion"></textarea><button type="submit" onclick="saveData()">保存</button></form><script>function saveData(){}</script>`))
		case "/http/quality.example/jsxsd/xspj/xspj_save.do":
			_, _ = writer.Write([]byte(`<script>alert('评价成功')</script>`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_VPN_BASE_URL", server.URL)
	t.Setenv("CSUST_QUALITY_PREFIX", "/http/quality.example")
	t.Setenv("CSUST_VPN_COOKIE_FILE", filepath.Join(t.TempDir(), "vpn.cookies"))
	t.Setenv("CSUST_VPN_SESSION_FILE", filepath.Join(t.TempDir(), "vpn.json"))

	handled, stdout, _, code, err := (NativeSite{}).Run(context.Background(), []string{"quality", "evaluation", "batches", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("batches: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload["items"].([]any)) != 1 {
		t.Fatalf("unexpected batches: %#v", payload)
	}

	handled, stdout, _, code, err = (NativeSite{}).Run(context.Background(), []string{"quality", "evaluation", "form", "--path", "/jsxsd/xspj/xspj_course.do", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("form: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	payload = map[string]any{}
	if err := json.Unmarshal(stdout, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload["questions"].([]any)) != 1 || payload["hidden_fields"].([]any)[0].(map[string]any)["value"] != "<redacted>" {
		t.Fatalf("unexpected form: %#v", payload)
	}

	handled, stdout, _, code, err = (NativeSite{}).Run(context.Background(), []string{"quality", "evaluation", "save", "--path", "/jsxsd/xspj/xspj_course.do", "--answer", "q1=A", "--yes", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("save: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
}

func TestNativeQualityLoginWithExplicitCaptcha(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/http/quality.example/":
			writer.Header().Set("Content-Type", "text/html")
			_, _ = writer.Write([]byte(`<html><title>登录</title><body>login</body></html>`))
		case "/http/quality.example/verifycode.servlet":
			writer.Header().Set("Content-Type", "image/png")
			_, _ = writer.Write([]byte("png"))
		case "/http/quality.example/Logon.do":
			writer.Header().Set("Content-Type", "text/plain")
			if request.URL.Query().Get("flag") == "sess" {
				_, _ = writer.Write([]byte("#00000000000000000000"))
				return
			}
			if request.Method == "POST" {
				http.SetCookie(writer, &http.Cookie{Name: "quality_session", Value: "ok", Path: "/"})
				writer.Header().Set("Content-Type", "text/html")
				_, _ = writer.Write([]byte(`<script>alert('登录成功')</script>`))
				return
			}
			_, _ = writer.Write([]byte("#00000000000000000000"))
		case "/http/quality.example/jsxsd/framework/xsMain.jsp":
			writer.Header().Set("Content-Type", "text/html")
			_, _ = writer.Write([]byte(`<html><title>首页</title><body>home</body></html>`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	t.Setenv("CSUST_VPN_BASE_URL", server.URL)
	t.Setenv("CSUST_QUALITY_PREFIX", "/http/quality.example")
	t.Setenv("CSUST_VPN_COOKIE_FILE", filepath.Join(root, "vpn.cookies"))
	t.Setenv("CSUST_VPN_SESSION_FILE", filepath.Join(root, "vpn.json"))
	t.Setenv("CSUST_USERNAME", "student")
	t.Setenv("CSUST_PASSWORD", "password")
	captcha := filepath.Join(root, "captcha.png")
	handled, stdout, _, code, err := (NativeSite{}).Run(context.Background(), []string{"quality", "login", "--captcha", "123", "--captcha-image", captcha, "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("quality login: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != true || payload["service"] != qualityServiceName {
		t.Fatalf("unexpected quality login: %#v", payload)
	}
	if content, err := os.ReadFile(captcha); err != nil || string(content) != "png" {
		t.Fatalf("captcha file: %v %q", err, content)
	}
}
