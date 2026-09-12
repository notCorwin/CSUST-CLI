package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestEmploymentEncryptionMatchesBrowserProtocol(t *testing.T) {
	got, err := employmentEncrypt("hello", "1234567890abcdef")
	if err != nil || got != "OuG/HtvQGRTDeeep6ZO/WQ==" {
		t.Fatalf("unexpected employment AES-ECB payload: %q %v", got, err)
	}
}

func TestEmploymentLoginAndStatus(t *testing.T) {
	const viCode = "1234567890abcdef"
	const account = "account"
	const password = "secret"
	const realIP = "192.0.2.10"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		session, _ := request.Cookie("employment_session")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/login":
			_, _ = fmt.Fprintf(writer, `<html><body><input id="user_name"><input id="password"><script>var real_ip = %q;</script></body></html>`, realIP)
		case request.Method == http.MethodGet && request.URL.Path == "/login/get_vi_code":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(writer, `{"code":1,"data":{"vi_code":%q}}`, viCode)
		case request.Method == http.MethodPost && request.URL.Path == "/login/get_encode_token":
			if err := request.ParseForm(); err != nil || request.Form.Get("code") != mustEmploymentEncryption(t, password, viCode) || request.Form.Get("encrypt") != "1" {
				t.Fatalf("unexpected employment token request: %v", request.Form)
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"code":1,"data":"encode-token"}`))
		case request.Method == http.MethodPost && request.URL.Path == "/login/submit":
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if request.Header.Get("t") != "encode-token" || request.Form.Get("user_name") != mustEmploymentEncryption(t, account, viCode) || request.Form.Get("mm") != mustEmploymentEncryption(t, password, viCode) || request.Form.Get("valid_code") != mustEmploymentEncryption(t, "", viCode) || request.Form.Get("real_address") != mustEmploymentEncryption(t, realIP, viCode) || request.Form.Get("captcha_token") != "captcha-token" || request.Form.Get("captcha") != "captcha-value" {
				t.Fatalf("unexpected employment login request: %v", request.Form)
			}
			http.SetCookie(writer, &http.Cookie{Name: "employment_session", Value: "ok", Path: "/"})
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"code":1,"data":{"info":[]}}`))
		case request.Method == http.MethodGet && request.URL.Path == "/student" && session != nil && session.Value == "ok":
			_, _ = writer.Write([]byte(`<html><body>学生信息平台</body></html>`))
		case request.Method == http.MethodGet && request.URL.Path == "/student":
			_, _ = writer.Write([]byte(`<html><body><input id="user_name"><input id="password"></body></html>`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "employment.cookies.txt"))

	login := runIssueJSON(t, "employment", "login", "--username", account, "--password", password, "--captcha", "captcha-value", "--captcha-token", "captcha-token")
	if login["ok"] != true || login["confirmed"] != true || login["username"] != account {
		t.Fatalf("employment login was not confirmed: %#v", login)
	}
	status := runIssueJSON(t, "employment", "status")
	if status["logged_in"] != true || status["confirmed"] != true {
		t.Fatalf("employment status did not confirm session: %#v", status)
	}
}

func TestEmploymentEmailVerificationFlow(t *testing.T) {
	const (
		viCode    = "1234567890abcdef"
		account   = "account"
		password  = "secret"
		email     = "student@example.com"
		emailCode = "246810"
		realIP    = "192.0.2.11"
	)
	var emailSent, emailVerified bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		session, _ := request.Cookie("employment_session")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/login":
			_, _ = fmt.Fprintf(writer, `<html><body><input id="user_name"><input id="password"><script>var real_ip = %q;</script></body></html>`, realIP)
		case request.Method == http.MethodGet && request.URL.Path == "/login/get_vi_code":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(writer, `{"code":1,"data":{"vi_code":%q}}`, viCode)
		case request.Method == http.MethodPost && request.URL.Path == "/login/get_encode_token":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"code":1,"data":"encode-token"}`))
		case request.Method == http.MethodPost && request.URL.Path == "/login/submit":
			http.SetCookie(writer, &http.Cookie{Name: "employment_session", Value: "pending", Path: "/"})
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"code":1,"data":{"student_login_email":"student@example.com"}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/login/send_mail_code":
			if err := request.ParseForm(); err != nil || request.Form.Get("mail") != mustEmploymentEncryption(t, email, viCode) {
				t.Fatalf("email code request was not encrypted as the browser does: %v", request.Form)
			}
			emailSent = true
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"code":1,"msg":"验证码已发送"}`))
		case request.Method == http.MethodPost && request.URL.Path == "/login/check_mail_login":
			if err := request.ParseForm(); err != nil || request.Form.Get("mail") != email || request.Form.Get("mail_code") != emailCode {
				t.Fatalf("email verification fields were not mapped: %v", request.Form)
			}
			emailVerified = true
			http.SetCookie(writer, &http.Cookie{Name: "employment_session", Value: "ok", Path: "/"})
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"code":1,"msg":"验证成功"}`))
		case request.Method == http.MethodGet && request.URL.Path == "/student" && session != nil && session.Value == "ok":
			_, _ = writer.Write([]byte(`<html><body>学生信息平台</body></html>`))
		case request.Method == http.MethodGet && request.URL.Path == "/student":
			_, _ = writer.Write([]byte(`<html><body><input id="user_name"><input id="password"></body></html>`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "employment-email.cookies.txt"))

	handled, stdout, _, code, err := (NativeSite{}).Run(context.Background(), []string{"employment", "login", "--username", account, "--password", password, "--captcha", "captcha", "--captcha-token", "token", "--json"}, true)
	if err != nil || !handled || code != 2 {
		t.Fatalf("email verification challenge was not returned: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	var challenge map[string]any
	if err := json.Unmarshal(stdout, &challenge); err != nil || challenge["code"] != "email_verification_required" {
		t.Fatalf("unexpected email verification challenge: %s", stdout)
	}

	sent := runIssueJSON(t, "employment", "send-email-code", "--email", email, "--yes")
	if sent["confirmed"] != true || sent["email_hint"] != "s***t@example.com" || !emailSent {
		t.Fatalf("email code send was not confirmed: %#v", sent)
	}
	verified := runIssueJSON(t, "employment", "verify-email", "--email", email, "--email-code", emailCode)
	if verified["confirmed"] != true || verified["logged_in"] != true || !emailVerified {
		t.Fatalf("email verification was not confirmed: %#v", verified)
	}
}

func mustEmploymentEncryption(t *testing.T, message, key string) string {
	t.Helper()
	value, err := employmentEncrypt(message, key)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
