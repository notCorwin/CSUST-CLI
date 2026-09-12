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
	authorized := func(writer http.ResponseWriter, request *http.Request) bool {
		if request.Header.Get("Authorization") == "Bearerquality-token" {
			return true
		}
		writer.WriteHeader(http.StatusUnauthorized)
		return false
	}
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
			if !authorized(writer, request) {
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"code":200,"data":{"data":{"loginname":"1001","realname":"测试"}}}`)
		case "/api/tpk/tpk/getTaskNums", "/api/tpk/tpk/getTkInfoStatictis", "/api/tpk/tpk/getCompleteOfTasksInfo":
			if !authorized(writer, request) || request.Method != http.MethodGet {
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			data := `{"pending":2}`
			if request.URL.Path == "/api/tpk/tpk/getTkInfoStatictis" {
				data = `{"average":91}`
			} else if request.URL.Path == "/api/tpk/tpk/getCompleteOfTasksInfo" {
				data = `{"completed":8}`
			}
			_, _ = fmt.Fprintf(writer, `{"code":200,"data":%s}`, data)
		case "/api/manage/homePage/selectByLoginName", "/api/manage/selectopt/semesters", "/api/manage/selectopt/orgns", "/api/manage/selectopt/courses", "/api/manage/selectopt/teachers", "/api/manage/selectopt/roles":
			if !authorized(writer, request) {
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			if request.URL.Path == "/api/manage/selectopt/courses" || request.URL.Path == "/api/manage/selectopt/teachers" {
				if request.URL.Query().Get("orgCode") != "ORG" || request.URL.Query().Get("search") != "课程" {
					writer.WriteHeader(http.StatusBadRequest)
					return
				}
			}
			_, _ = fmt.Fprint(writer, `{"code":200,"data":{"opts":[{"id":"1","name":"测试项"}],"home":"ok"}}`)
		case "/api/tpk/tpk/getMytpktask", "/api/tpk/tpk/getTtpkListenresultList", "/api/tpk/tpk/getTtpkWaitListencourseList", "/api/tpk/tpk/getTtpkImprovementsList":
			if !authorized(writer, request) {
				return
			}
			query := request.URL.Query()
			if query.Get("yeartermcode") != "2026-2027-1" || query.Get("orgcode") != "ORG" || query.Get("searchss") != "课程" || query.Get("page") != "2" || query.Get("limit") != "20" {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"code":200,"data":{"pageData":{"records":[{"id":"42","title":"测试任务"}],"total":1}}}`)
		case "/api/tpk/tpk/getTtpkListenresultListxq":
			if !authorized(writer, request) {
				return
			}
			if request.URL.Query().Get("resultid") != "42" {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"code":200,"data":{"resultid":"42","score":95}}`)
		case "/api/manage/doLogout":
			if !authorized(writer, request) {
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

	dashboard := runIssueJSON(t, "quality-system", "dashboard")
	if dashboard["data"].(map[string]any)["task_counts"].(map[string]any)["pending"] != float64(2) || dashboard["api_code"].(map[string]any)["completion"] != float64(200) {
		t.Fatalf("quality-system dashboard model failed: %#v", dashboard)
	}
	home := runIssueJSON(t, "quality-system", "home")
	if home["data"].(map[string]any)["home"] != "ok" {
		t.Fatalf("quality-system home model failed: %#v", home)
	}
	semesters := runIssueJSON(t, "quality-system", "semesters")
	if semesters["api_code"] != float64(200) {
		t.Fatalf("quality-system semesters failed: %#v", semesters)
	}
	courses := runIssueJSON(t, "quality-system", "courses", "--organization", "ORG", "--keyword", "课程")
	if courses["filters"].(map[string]any)["organization"] != "ORG" {
		t.Fatalf("quality-system course filters failed: %#v", courses)
	}
	teachers := runIssueJSON(t, "quality-system", "teachers", "--organization", "ORG", "--keyword", "课程")
	if teachers["ok"] != true {
		t.Fatalf("quality-system teachers failed: %#v", teachers)
	}
	roles := runIssueJSON(t, "quality-system", "roles")
	if roles["ok"] != true {
		t.Fatalf("quality-system roles failed: %#v", roles)
	}
	for _, operation := range []string{"tasks", "results", "improvements", "waitlist"} {
		result := runIssueJSON(t, "quality-system", operation, "--semester", "2026-2027-1", "--organization", "ORG", "--keyword", "课程", "--page", "2", "--page-size", "20")
		if result["ok"] != true || result["filters"].(map[string]any)["page_size"] != float64(20) {
			t.Fatalf("quality-system %s failed: %#v", operation, result)
		}
	}
	detail := runIssueJSON(t, "quality-system", "result", "--id", "42")
	if detail["data"].(map[string]any)["resultid"] != "42" {
		t.Fatalf("quality-system result detail failed: %#v", detail)
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
