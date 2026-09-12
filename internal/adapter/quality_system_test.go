package adapter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestQualitySystemSession(t *testing.T) {
	var loginQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/manage/config/selectOne":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"code":200,"message":"查询系统配置成功","data":{"config":{"title":"教学质量保障系统","logo":"/system/logo.JPG","pwdLength":10,"pwdRule":"0-9A-Za-z_","isOpenLoginVali":"是","loginErrorNum":5,"loginLockedTime":10,"userLogins":"是","pwd_rule_check":"是"}}}`)
		case "/api/manage/common/makeVeriCode":
			writer.Header().Set("Content-Type", "image/jpeg")
			writer.Header().Set("Serialno", "serial-1")
			_, _ = writer.Write([]byte("jpeg"))
		case "/api/manage/doLogin":
			loginQuery = request.URL.Query()
			salt := loginQuery.Get("salt")
			saltValue, saltErr := strconv.Atoi(salt)
			if request.Method != http.MethodPost || loginQuery.Get("loginname") != "1001" || saltErr != nil || saltValue < 0 || saltValue >= 1000 || loginQuery.Get("pwd") != md5Hex("secret"+salt) || loginQuery.Get("code") != "123" || loginQuery.Get("serialNo") != "serial-1" {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"code":200,"data":{"accessToken":"quality-token","UserContext":{"loginname":"1001","realname":"测试"}}}`)
		case "/api/manage/common/getCurrenUser":
			if request.Header.Get("Authorization") != "Bearerquality-token" {
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"code":200,"data":{"data":{"loginname":"1001","realname":"测试"}}}`)
		case "/api/manage/doLogout":
			if request.Header.Get("Authorization") != "Bearerquality-token" {
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"code":200,"message":"退出成功"}`)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	sessionDir := t.TempDir()
	cookieFile := filepath.Join(sessionDir, "quality.cookies.txt")
	captchaImage := filepath.Join(sessionDir, "captcha.jpg")
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", cookieFile)
	t.Setenv("CSUST_QUALITY_SYSTEM_PASSWORD", "secret")

	config := runIssueJSON(t, "quality-system", "config")
	configData := config["data"].(map[string]any)
	if configData["captcha_enabled"] != true || configData["password_length"] != float64(10) || configData["source_fields_preserved"] != true {
		t.Fatalf("quality-system config model failed: %#v", config)
	}

	login := runIssueJSON(t, "quality-system", "login", "--username", "1001", "--captcha", "123", "--captcha-image", captchaImage)
	if login["ok"] != true || strings.Contains(string(mustMarshalIssue(login)), "quality-token") {
		t.Fatalf("quality-system login result invalid or leaked token: %#v", login)
	}
	if loginQuery == nil || loginQuery.Get("serialNo") != "serial-1" {
		t.Fatalf("captcha serial number was not forwarded: %v", loginQuery)
	}
	salt, saltErr := strconv.Atoi(loginQuery.Get("salt"))
	if saltErr != nil || salt < 0 || salt >= 1000 || loginQuery.Get("pwd") != md5Hex("secret"+loginQuery.Get("salt")) {
		t.Fatalf("salted password protocol was not used: %v", loginQuery)
	}
	if content, readErr := os.ReadFile(captchaImage); readErr != nil || string(content) != "jpeg" {
		t.Fatalf("captcha image was not saved: err=%v content=%q", readErr, content)
	}

	profile := runIssueJSON(t, "quality-system", "profile")
	if profile["data"].(map[string]any)["loginname"] != "1001" {
		t.Fatalf("quality-system profile model failed: %#v", profile)
	}
	status := runIssueJSON(t, "quality-system", "status")
	if status["logged_in"] != true {
		t.Fatalf("quality-system status failed: %#v", status)
	}
	runIssueJSON(t, "quality-system", "logout")
	if _, err := os.Stat(filepath.Join(sessionDir, "quality.token")); !os.IsNotExist(err) {
		t.Fatalf("quality-system token file was not removed: %v", err)
	}
}
