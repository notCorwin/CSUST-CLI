package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeLocalLoginPersistsOnlyConfirmedSession(t *testing.T) {
	temp := t.TempDir()
	var encoded, account, password, captcha string
	captchaRequests := 0
	seed := "abcdefghijklmnopqrst#11111111111111111111"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/":
			http.SetCookie(writer, &http.Cookie{Name: "SESSION", Value: "1", Path: "/"})
			_, _ = writer.Write([]byte(`<form id="loginForm" action="/Logon.do?method=logon" method="post"><input name="userAccount"><input name="userPassword" type="password"><input name="RANDOMCODE"><input name="encoded"></form>`))
		case "/verifycode.servlet":
			captchaRequests++
			writer.Header().Set("Content-Type", "image/png")
			_, _ = writer.Write([]byte("captcha"))
		case "/Logon.do":
			_ = request.ParseForm()
			if request.URL.Query().Get("flag") == "sess" {
				_, _ = writer.Write([]byte(seed))
				return
			}
			account, password, captcha, encoded = request.Form.Get("userAccount"), request.Form.Get("userPassword"), request.Form.Get("RANDOMCODE"), request.Form.Get("encoded")
			if captcha == "bad" {
				_, _ = writer.Write([]byte("密码错误"))
				return
			}
			if captcha == "captcha-bad" {
				_, _ = writer.Write([]byte("验证码无效,请重新登录!"))
				return
			}
			if captcha == "page" {
				_, _ = writer.Write([]byte(`<form id="loginForm"><input name="userAccount"><input name="userPassword"></form>`))
				return
			}
			http.SetCookie(writer, &http.Cookie{Name: "AUTH", Value: "1", Path: "/"})
			_, _ = writer.Write([]byte("登录成功"))
		case academicProbePath:
			if _, err := request.Cookie("AUTH"); err != nil {
				http.Error(writer, "unauthorized", http.StatusUnauthorized)
				return
			}
			writer.Header().Set("Content-Type", "text/html")
			_, _ = writer.Write([]byte(`<table id="kbtable"><tr><th>星期一</th></tr></table>`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	cookieFile := filepath.Join(temp, "cookies.txt")
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", cookieFile)
	t.Setenv("CSUST_USERNAME", "student")
	t.Setenv("CSUST_PASSWORD", "password")
	if _, err := (NativeSite{}).loginAcademic(context.Background(), loginOptions{auth: "local"}); err == nil || err.Code != "captcha_required" {
		t.Fatalf("expected first login to request captcha, got %v", err)
	}
	result, err := (NativeSite{}).loginAcademic(context.Background(), loginOptions{auth: "local", captcha: "good"})
	if err != nil {
		t.Fatal(err)
	}
	if result["auth"] != "local" || result["confirmed"] != true {
		t.Fatalf("unexpected login result: %#v", result)
	}
	wantEncoded, encodedErr := generateEncodedGo("student", "password", seed)
	if encodedErr != nil {
		t.Fatal(encodedErr)
	}
	if account != "" || password != "" || captcha != "good" || encoded != wantEncoded {
		t.Fatalf("unexpected legacy login payload: account=%q password=%q captcha=%q encoded=%q want=%q", account, password, captcha, encoded, wantEncoded)
	}
	if captchaRequests != 1 {
		t.Fatalf("captcha was refreshed during retry: requests=%d", captchaRequests)
	}
	if _, err := (NativeSite{}).loginAcademic(context.Background(), loginOptions{auth: "local", captcha: "bad"}); err == nil || err.Code != "authentication_failed" {
		t.Fatalf("expected invalid credentials to map to authentication_failed, got %v", err)
	}
	if _, err := (NativeSite{}).loginAcademic(context.Background(), loginOptions{auth: "local", captcha: "captcha-bad"}); err == nil || err.Code != "captcha_failed" {
		t.Fatalf("expected current captcha error to map to captcha_failed, got %v", err)
	}
	if _, err := (NativeSite{}).loginAcademic(context.Background(), loginOptions{auth: "local", captcha: "page"}); err == nil || err.Code != "authentication_failed" {
		t.Fatalf("expected login page response to map to authentication_failed, got %v", err)
	}
	if _, statErr := os.Stat(cookieFile); statErr != nil {
		t.Fatal(statErr)
	}
	if mode := os.FileMode(mustFileMode(t, cookieFile)); mode.Perm() != 0o600 {
		t.Fatalf("cookie mode is %o", mode.Perm())
	}
}

func TestCASLoginFieldsExcludeBrowserPasswordInput(t *testing.T) {
	document, err := parsePage(`<form id="pwdFromId"><input name="username"><input name="passwordText" type="password"><input id="saltPassword" name="password" type="hidden"><input name="execution" value="e1s1"></form>`)
	if err != nil {
		t.Fatal(err)
	}
	fields := casLoginFields(document.first("form", "pwdFromId"))
	for _, field := range fields {
		if field.name == "password" || field.name == "passwordText" || field.name == "username" {
			t.Fatalf("browser-controlled credential field leaked into CAS fields: %#v", fields)
		}
	}
	if len(fields) != 1 || fields[0].name != "execution" || fields[0].value != "e1s1" {
		t.Fatalf("unexpected CAS fields: %#v", fields)
	}
}

func TestNativeLogoutVerifiesRemoteSessionAndClearsCookie(t *testing.T) {
	logoutCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/jsxsd/xk/LoginToXk" && request.URL.Query().Get("method") == "exit" {
			logoutCalled = true
			_, _ = writer.Write([]byte("退出成功"))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()
	cookieFile := filepath.Join(t.TempDir(), "cookies.txt")
	if err := os.WriteFile(cookieFile, []byte("# Netscape HTTP Cookie File\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", cookieFile)
	handled, stdout, _, code, err := (NativeSite{}).Run(context.Background(), []string{"logout", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("logout: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	if !logoutCalled || strings.Contains(string(stdout), `"confirmed":false`) {
		t.Fatalf("logout was not verified: called=%v output=%s", logoutCalled, stdout)
	}
	if _, err := os.Stat(cookieFile); !os.IsNotExist(err) {
		t.Fatalf("cookie file was not removed: %v", err)
	}
}

func mustFileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()
}
