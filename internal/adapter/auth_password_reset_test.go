package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthPasswordResetPhoneFlowUsesProtocolAndResumeState(t *testing.T) {
	var sendBody, checkBody, resetBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/retrieve-password/generateCaptcha":
			writer.Header().Set("Content-Type", "image/png")
			_, _ = writer.Write([]byte("captcha"))
		case "/tenant/info":
			_, _ = writer.Write([]byte(`{"code":"0","retrieveMethod":"1,2,3"}`))
		case "/retrieve-password/passwordRetrieve/checkUserInfo":
			if err := json.NewDecoder(request.Body).Decode(&sendBody); err != nil || sendBody["loginNo"] != "test-account" || sendBody["captchaId"] != "account-id" || sendBody["captcha"] != "account-code" {
				t.Fatalf("account check body was not mapped: %#v", sendBody)
			}
			_, _ = writer.Write([]byte(`{"code":"0","datas":"{\"accountId\":\"account-1\",\"loginNo\":\"test-account\",\"sign\":\"sign-1\"}"}`))
		case "/retrieve-password/sendCode":
			if err := json.NewDecoder(request.Body).Decode(&sendBody); err != nil {
				t.Fatalf("send-code body: %v", err)
			}
			if sendBody["type"] != "cellphone" || sendBody["captchaId"] != "phone-id" || sendBody["captcha"] != "phone-code" || sendBody["cellphone"] == "13800138000" {
				t.Fatalf("phone verification body was not mapped/encrypted: %#v", sendBody)
			}
			_, _ = writer.Write([]byte(`{"code":"0","datas":"{\"limitTime\":120}"}`))
		case "/retrieve-password/passwordRetrieve/checkCode":
			if err := json.NewDecoder(request.Body).Decode(&checkBody); err != nil {
				t.Fatalf("check-code body: %v", err)
			}
			if checkBody["code"] != "sms-code" || checkBody["cellphone"] == "13800138000" || checkBody["sign"] != "sign-1" {
				t.Fatalf("SMS verification body was not mapped: %#v", checkBody)
			}
			_, _ = writer.Write([]byte(`{"code":"0","datas":"{\"loginNo\":\"test-account\",\"sign\":\"sign-2\"}"}`))
		case "/retrieve-password/passwordRetrieve/resetPassword":
			if err := json.NewDecoder(request.Body).Decode(&resetBody); err != nil {
				t.Fatalf("reset-password body: %v", err)
			}
			if resetBody["password"] == "new-password" || resetBody["confirmPassword"] == "new-password" || resetBody["sign"] != "sign-2" || resetBody["cellphone"] == "13800138000" {
				t.Fatalf("reset body was not encrypted or signed: %#v", resetBody)
			}
			_, _ = writer.Write([]byte(`{"code":"0","message":"密码重置成功"}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	stateFile := filepath.Join(t.TempDir(), "reset-state.json")
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	pending := runIssueJSON(t, "login", "reset-password", "--username", "test-account", "--method", "phone", "--captcha-id", "account-id", "--captcha", "account-code", "--phone", "13800138000", "--phone-captcha-id", "phone-id", "--phone-captcha", "phone-code", "--send-code", "--state-file", stateFile, "--yes")
	if pending["pending"] != true || pending["next_step"] != "code" || pending["submitted"] != true || !strings.Contains(string(mustMarshalIssue(pending)), stateFile) {
		t.Fatalf("unexpected pending result: %#v", pending)
	}
	if mode := mustFileMode(t, stateFile).Perm(); mode != 0o600 {
		t.Fatalf("state file mode is %o", mode)
	}

	result := runIssueJSON(t, "login", "reset-password", "--state-file", stateFile, "--method", "phone", "--code", "sms-code", "--new-password", "new-password", "--password-confirm", "new-password", "--yes")
	if result["reset"] != true || result["confirmed"] != true || result["evidence"] != "authserver_code_0" {
		t.Fatalf("password reset was not confirmed: %#v", result)
	}
	if strings.Contains(string(mustMarshalIssue(result)), "new-password") {
		t.Fatal("password leaked in reset result")
	}
	if _, err := os.Stat(stateFile); err != nil {
		t.Fatal(err)
	}
}

func TestAuthPasswordResetRequiresAccountCaptcha(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/retrieve-password/generateCaptcha" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "image/png")
		_, _ = writer.Write([]byte("captcha"))
	}))
	defer server.Close()
	image := filepath.Join(t.TempDir(), "account.png")
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))
	_, err := (NativeSite{}).executeAuthPasswordReset(context.Background(), []string{"--username", "test-account", "--captcha-image", image})
	if err == nil || err.Code != "captcha_required" || err.Details["captcha_id"] == nil {
		t.Fatalf("captcha requirement was not returned: %v", err)
	}
	if _, statErr := os.Stat(image); statErr != nil {
		t.Fatal(statErr)
	}
}
