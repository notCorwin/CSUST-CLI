package adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBusinessAdaptersKeepSemanticAndRawData(t *testing.T) {
	catalog := businessCatalogNames("onlinejudge")
	items := catalog["catalog"].([]map[string]any)
	if len(items) != 1 || items[0]["name"] != "onlinejudge" || items[0]["confidence_evidence"] == "" {
		t.Fatalf("unexpected business catalog: %#v", catalog)
	}
	staffCatalog := businessCatalogNames("staff-record-appointment")
	staff := staffCatalog["catalog"].([]map[string]any)
	if len(staff) != 1 || !strings.Contains(staff[0]["url"].(string), "fid=4") || staff[0]["entrypoints"] == nil {
		t.Fatalf("staff appointment catalog lost its two form entrypoints: %#v", staffCatalog)
	}

	result := businessResult(map[string]any{
		"response": map[string]any{"json": map[string]any{
			"error": nil,
			"data":  map[string]any{"total": float64(1), "results": []any{}},
		}},
	}, "onlinejudge", "problems")
	if _, ok := result["response"]; ok {
		t.Fatalf("JSON response was not unwrapped: %#v", result)
	}
	data, ok := result["data"].(map[string]any)
	if !ok || data["total"] != float64(1) {
		t.Fatalf("unexpected unwrapped data: %#v", result)
	}

	decoded, err := decodeJSAssignment(`const data = {"career":[{"career_talk_id":7,"meet_name":"招聘会"}]};`, "const data =")
	if err != nil || len(employmentItems(decoded, "career")) != 1 {
		t.Fatalf("embedded employment data was not decoded: %#v %v", decoded, err)
	}

	article := journalArticle(map[string]any{
		"file_no": "20260112", "title": "题目", "author_name": "作者", "doi": "10/test",
	}, "journal-transport", "jtkxygc")
	if article["id"] != "20260112" || article["title"] != "题目" || article["raw"] == nil {
		t.Fatalf("article lost semantic/raw fields: %#v", article)
	}
	if prefix, service, ok := journalService("science"); !ok || prefix != "cslgdxxbzk" || service != "journal-science" || !strings.Contains(article["links"].(map[string]string)["abstract"], "jtkxygc.csust.edu.cn") {
		t.Fatalf("journal protocol mapping changed unexpectedly: %#v", article)
	}
	if prefix, service, ok := journalService("highway"); !ok || prefix != "zwgl" || service != "journal-highway" {
		t.Fatalf("highway journal protocol mapping missing: %q %q %v", prefix, service, ok)
	}
	science := journalArticle(map[string]any{"file_no": "20250215", "title": "自然科学"}, "journal-science", "cslgdxxbzk")
	if !strings.Contains(science["links"].(map[string]string)["abstract"], "cslgxbzk.csust.edu.cn/cslgdxxbzk") {
		t.Fatalf("journal host mapping lost: %#v", science)
	}

	if got := findID(map[string]any{"data": []any{map[string]any{"submission_id": float64(12)}}}); got != "12" {
		t.Fatalf("unexpected nested id: %q", got)
	}
	if got := recordOriginValue("湖南省"); got != "14" {
		t.Fatalf("unexpected province mapping: %q", got)
	}
	if role := journalRole("审稿人"); role != "reviewer" {
		t.Fatalf("unexpected journal role: %q", role)
	}
	if encrypted, err := journalEncryptedPasswordWithRandom("secret", bytes.NewReader(bytes.Repeat([]byte{1}, 126))); err != nil || len(encrypted) != 252 {
		t.Fatalf("unexpected journal RSA payload: len=%d err=%v", len(encrypted), err)
	}
	if got := strings.ToLower(sm3Hex("abc")); got != "66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a2297da02b8f4ba8e0" {
		t.Fatalf("unexpected SM3 digest: %s", got)
	}
	if state, known := jsonBusinessState(map[string]any{"state": "success"}); !state || !known {
		t.Fatalf("state=success was not recognized: %v %v", state, known)
	}
	if !securityLoginForm(`<form id="loginForm"><input name="user[account]"></form>`) {
		t.Fatal("security login form was not recognized")
	}
	if !cmsLoginPage(`<form id="loginform"><input name="user"></form>`) {
		t.Fatal("CMS login form was not recognized")
	}
	if got := redactSiteJSON(map[string]any{"results": []any{1}, "password": "secret"}); !reflect.DeepEqual(got, map[string]any{"results": []any{1}, "password": "<redacted>"}) {
		t.Fatalf("sensitive redaction changed ordinary result fields: %#v", got)
	}
}

func TestTargetEntriesKeepUnreachableSitesExplicit(t *testing.T) {
	catalog := businessCatalog()["catalog"].([]map[string]any)
	wanted := map[string]string{
		"legacy-portal":       "my.csust.edu.cn",
		"academic-affairs":    "jwc.csust.edu.cn",
		"graduate-management": "yjsgl.csust.edu.cn",
		"admissions-system":   "zs.csust.edu.cn",
		"alumni":              "xy.csust.edu.cn",
		"app":                 "app.csust.edu.cn:8087",
		"training":            "gcxljxgl.csust.edu.cn",
		"srv":                 "srv.csust.edu.cn",
		"icsai2003":           "icsai2003.csust.edu.cn",
		"trx":                 "trx.csust.edu.cn",
		"v":                   "v.csust.edu.cn",
		"live":                "live.csust.edu.cn",
	}
	for name, host := range wanted {
		var found map[string]any
		for _, item := range catalog {
			if item["name"] == name {
				found = item
				break
			}
		}
		if found == nil || found["host"] != host || found["confidence_evidence"] == "" {
			t.Fatalf("target site is missing explicit probe evidence: %s %#v", name, found)
		}
	}
	for _, name := range []string{"legacy-portal", "academic-affairs", "graduate-management", "admissions-system", "alumni", "app", "training", "srv", "icsai2003", "trx", "v", "live"} {
		result := runIssueJSON(t, name, "catalog")
		if result["catalog"] == nil || result["confirmed"] != true {
			t.Fatalf("target site catalog command is not exposed: %s %#v", name, result)
		}
	}
}

func TestOnlineJudgeLoginAndLogoutUseCSRFAndProbeSession(t *testing.T) {
	const username, password, tfaCode, csrf = "alice", "judge-password-secret", "tfa-code-secret", "test-csrf"
	loggedIn := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/":
			http.SetCookie(writer, &http.Cookie{Name: "csrftoken", Value: csrf, Path: "/"})
			_, _ = writer.Write([]byte(`{"ok":true}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/tfa_required":
			var body struct {
				Username string `json:"username"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body.Username != username || request.Header.Get("X-CSRFToken") != csrf {
				t.Fatalf("OnlineJudge TFA request was not mapped correctly: username=%q", body.Username)
			}
			_, _ = writer.Write([]byte(`{"error":null,"data":{"result":true}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/login":
			var body struct {
				Username string `json:"username"`
				Password string `json:"password"`
				TFACode  string `json:"tfa_code"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body.Username != username || body.Password != password || body.TFACode != tfaCode || request.Header.Get("X-CSRFToken") != csrf {
				t.Fatalf("OnlineJudge login request was not mapped correctly: username=%q csrf=%q", body.Username, request.Header.Get("X-CSRFToken"))
			}
			loggedIn = true
			http.SetCookie(writer, &http.Cookie{Name: "sessionid", Value: "session", Path: "/"})
			_, _ = writer.Write([]byte(`{"error":null,"data":{"username":"alice"}}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/profile":
			if !loggedIn {
				_, _ = writer.Write([]byte(`{"error":"login required"}`))
				return
			}
			_, _ = writer.Write([]byte(`{"error":null,"data":{"username":"alice"}}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/logout":
			if _, err := request.Cookie("sessionid"); err != nil || !loggedIn {
				t.Fatalf("OnlineJudge logout request was not authenticated")
			}
			loggedIn = false
			_, _ = writer.Write([]byte(`{"error":null}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	cookieFile := filepath.Join(t.TempDir(), "cookies.txt")
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", cookieFile)

	result := runIssueJSON(t, "onlinejudge", "login", "--username", username, "--password", password, "--tfa-code", tfaCode)
	safeResult := string(mustMarshalIssue(result))
	if result["confirmed"] != true || result["evidence"] != "login-response-and-profile-probe" || strings.Contains(safeResult, password) || strings.Contains(safeResult, tfaCode) || strings.Contains(safeResult, csrf) {
		t.Fatalf("unexpected or unsafe OnlineJudge login result: %#v", result)
	}
	logout := runIssueJSON(t, "onlinejudge", "logout", "--yes")
	if logout["confirmed"] != true || logout["evidence"] != "logout-response-and-local-cookie-removed" || loggedIn {
		t.Fatalf("unexpected OnlineJudge logout result: %#v", logout)
	}
}

func TestOnlineJudgeExtendedReadOperationsMapFrontendContracts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		query := request.URL.Query()
		switch request.URL.Path {
		case "/api/user_rank":
			if query.Get("offset") != "3" || query.Get("limit") != "3" || query.Get("rule") != "OI" {
				t.Fatalf("rank query was not mapped: %v", query)
			}
			_, _ = writer.Write([]byte(`{"error":null,"data":{"results":[],"total":0}}`))
		case "/api/questions":
			if query.Get("offset") != "0" || query.Get("limit") != "4" || query.Get("myself") != "1" || query.Get("problem_id") != "12" {
				t.Fatalf("question query was not mapped: %v", query)
			}
			_, _ = writer.Write([]byte(`{"error":null,"data":{"results":[],"total":0}}`))
		case "/api/contest/problem":
			if query.Get("contest_id") != "7" || query.Get("problem_id") != "A" {
				t.Fatalf("contest problem query was not mapped: %v", query)
			}
			_, _ = writer.Write([]byte(`{"error":null,"data":{"id":"A"}}`))
		case "/api/contest_rank":
			if query.Get("contest_id") != "7" || query.Get("force_refresh") != "1" {
				t.Fatalf("contest rank query was not mapped: %v", query)
			}
			_, _ = writer.Write([]byte(`{"error":null,"data":{"results":[],"total":0}}`))
		case "/api/pickone":
			_, _ = writer.Write([]byte(`{"error":null,"data":"12"}`))
		case "/api/languages":
			_, _ = writer.Write([]byte(`{"error":null,"data":{"languages":[]}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	if result := runIssueJSON(t, "onlinejudge", "oi-rank", "--page", "2", "--limit", "3"); result["operation"] != "rank" {
		t.Fatalf("unexpected rank result: %#v", result)
	}
	if result := runIssueJSON(t, "onlinejudge", "questions", "--limit", "4", "--myself", "--problem-id", "12"); result["operation"] != "questions" {
		t.Fatalf("unexpected question result: %#v", result)
	}
	if result := runIssueJSON(t, "onlinejudge", "contest-problem", "--contest-id", "7", "--problem-id", "A"); result["operation"] != "contest-problem" {
		t.Fatalf("unexpected contest problem result: %#v", result)
	}
	if result := runIssueJSON(t, "onlinejudge", "contest-rank", "--contest-id", "7", "--force-refresh"); result["operation"] != "contest-rank" {
		t.Fatalf("unexpected contest rank result: %#v", result)
	}
	if result := runIssueJSON(t, "onlinejudge", "pick-one"); result["data"] != "12" {
		t.Fatalf("unexpected pick-one result: %#v", result)
	}
	if result := runIssueJSON(t, "onlinejudge", "languages"); result["operation"] != "languages" {
		t.Fatalf("unexpected languages result: %#v", result)
	}
}

func TestOnlineJudgeExtendedMutationsRequireConfirmationAndReadBack(t *testing.T) {
	const registerPassword = "register-password-secret"
	const oldPassword = "old-password-secret"
	const newPassword = "new-password-secret"
	const emailPassword = "email-password-secret"
	var revoked bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		decode := func() map[string]string {
			var body map[string]string
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("decode OnlineJudge JSON body: %v", err)
			}
			return body
		}
		switch request.Method + " " + request.URL.Path {
		case "POST /api/register":
			body := decode()
			if body["username"] != "new-user" || body["email"] != "new@example.com" || body["password"] != registerPassword || body["captcha"] != "1234" {
				t.Fatalf("register body was not mapped: %#v", body)
			}
			_, _ = writer.Write([]byte(`{"error":null,"data":{"username":"new-user"}}`))
		case "PUT /api/profile":
			body := decode()
			if body["real_name"] != "新名字" || body["language"] != "zh-CN" {
				t.Fatalf("profile body was not mapped: %#v", body)
			}
			_, _ = writer.Write([]byte(`{"error":null,"data":{"real_name":"新名字"}}`))
		case "POST /api/change_password":
			body := decode()
			if body["old_password"] != oldPassword || body["new_password"] != newPassword || body["tfa_code"] != "654321" {
				t.Fatalf("password body was not mapped: %#v", body)
			}
			_, _ = writer.Write([]byte(`{"error":null,"data":true}`))
		case "POST /api/change_email":
			body := decode()
			if body["password"] != emailPassword || body["old_email"] != "old@example.com" || body["new_email"] != "next@example.com" {
				t.Fatalf("email body was not mapped: %#v", body)
			}
			_, _ = writer.Write([]byte(`{"error":null,"data":true}`))
		case "GET /api/profile":
			_, _ = writer.Write([]byte(`{"error":null,"data":{"email":"old@example.com"}}`))
		case "POST /api/two_factor_auth", "PUT /api/two_factor_auth":
			body := decode()
			if body["code"] != "654321" {
				t.Fatalf("TFA body was not mapped: %#v", body)
			}
			_, _ = writer.Write([]byte(`{"error":null,"data":true}`))
		case "POST /api/question":
			body := decode()
			if body["problem_id"] != "12" || body["contest_id"] != "7" || body["text"] != "请解释" {
				t.Fatalf("question body was not mapped: %#v", body)
			}
			_, _ = writer.Write([]byte(`{"error":null,"data":{"question_id":"q1"}}`))
		case "PUT /api/question_answer":
			body := decode()
			if body["id"] != "q1" || body["answer"] != "回答" || body["solved"] != "True" {
				t.Fatalf("question answer body was not mapped: %#v", body)
			}
			_, _ = writer.Write([]byte(`{"error":null,"data":200}`))
		case "GET /api/question":
			if request.URL.Query().Get("id") != "q1" {
				t.Fatalf("question readback query was not mapped: %v", request.URL.Query())
			}
			_, _ = writer.Write([]byte(`{"error":null,"data":{"id":"q1","solved":true}}`))
		case "DELETE /api/sessions":
			if request.URL.Query().Get("session_key") != "session-to-revoke" {
				t.Fatalf("session revoke query was not mapped: %v", request.URL.Query())
			}
			revoked = true
			_, _ = writer.Write([]byte(`{"error":null}`))
		case "GET /api/sessions":
			if !revoked {
				t.Fatalf("session readback ran before revoke")
			}
			_, _ = writer.Write([]byte(`{"error":null,"data":[]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	register := runIssueJSON(t, "onlinejudge", "register", "--username", "new-user", "--password", registerPassword, "--email", "new@example.com", "--captcha", "1234", "--yes")
	profile := runIssueJSON(t, "onlinejudge", "profile-update", "--real-name", "新名字", "--language", "zh-CN", "--yes")
	password := runIssueJSON(t, "onlinejudge", "change-password", "--old-password", oldPassword, "--new-password", newPassword, "--tfa-code", "654321", "--yes")
	email := runIssueJSON(t, "onlinejudge", "change-email", "--password", emailPassword, "--new-email", "next@example.com", "--yes")
	tfaEnable := runIssueJSON(t, "onlinejudge", "tfa-enable", "--code", "654321", "--yes")
	tfaDisable := runIssueJSON(t, "onlinejudge", "tfa-disable", "--code", "654321", "--yes")
	question := runIssueJSON(t, "onlinejudge", "ask-question", "--problem-id", "12", "--contest-id", "7", "--content", "请解释", "--yes")
	answer := runIssueJSON(t, "onlinejudge", "answer-question", "--id", "q1", "--answer", "回答", "--yes")
	session := runIssueJSON(t, "onlinejudge", "revoke-session", "--session-key", "session-to-revoke", "--yes")
	if question["question_id"] != "q1" || answer["question_id"] != "q1" || session["evidence"] != "session-revoked-and-read-back" {
		t.Fatalf("extended mutation evidence missing: question=%#v answer=%#v session=%#v", question, answer, session)
	}
	if tfaEnable["operation"] != "tfa-enable" || tfaDisable["operation"] != "tfa-disable" {
		t.Fatalf("TFA evidence missing: enable=%#v disable=%#v", tfaEnable, tfaDisable)
	}
	serialized, _ := json.Marshal([]map[string]any{register, profile, password, email, tfaEnable, tfaDisable, question, answer, session})
	for _, secret := range []string{registerPassword, oldPassword, newPassword, emailPassword, "654321", "session-to-revoke"} {
		if strings.Contains(string(serialized), secret) {
			t.Fatalf("OnlineJudge mutation output leaked secret %q", secret)
		}
	}
}

func TestOnlineJudgeRecoveryAvatarAndDisplayIDContracts(t *testing.T) {
	const resetPassword = "reset-password-secret"
	var avatar []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		decode := func() map[string]string {
			var body map[string]string
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("decode OnlineJudge recovery JSON body: %v", err)
			}
			return body
		}
		switch request.Method + " " + request.URL.Path {
		case "GET /api/captcha":
			_, _ = writer.Write([]byte(`{"error":null,"data":"data:image/png;base64,UE5H"}`))
		case "GET /api/two_factor_auth":
			_, _ = writer.Write([]byte(`{"error":null,"data":"data:image/png;base64,UVI="}`))
		case "POST /api/apply_reset_password":
			body := decode()
			if body["email"] != "user@example.com" || body["captcha"] != "A1B2" {
				t.Fatalf("password reset request body was not mapped: %#v", body)
			}
			_, _ = writer.Write([]byte(`{"error":null,"data":true}`))
		case "POST /api/reset_password":
			body := decode()
			if body["token"] != "mail-token" || body["captcha"] != "C3D4" || body["password"] != resetPassword {
				t.Fatalf("password reset body was not mapped: %#v", body)
			}
			_, _ = writer.Write([]byte(`{"error":null,"data":true}`))
		case "GET /api/profile/fresh_display_id":
			_, _ = writer.Write([]byte(`{"error":null,"data":{"display_id":"new-display"}}`))
		case "POST /api/upload_avatar":
			file, _, err := request.FormFile("image")
			if err != nil {
				t.Fatalf("avatar was not uploaded as image: %v", err)
			}
			avatar, err = io.ReadAll(file)
			_ = file.Close()
			if err != nil {
				t.Fatalf("read avatar: %v", err)
			}
			_, _ = writer.Write([]byte(`{"error":null,"data":true}`))
		case "GET /api/profile":
			_, _ = writer.Write([]byte(`{"error":null,"data":{"avatar":"/public/avatar/new.png"}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(root, "cookies.txt"))

	captchaPath := filepath.Join(root, "captcha.png")
	captcha := runIssueJSON(t, "onlinejudge", "captcha", "--output", captchaPath)
	content, err := os.ReadFile(captchaPath)
	if err != nil || string(content) != "PNG" || captcha["operation"] != "captcha" {
		t.Fatalf("captcha was not decoded and saved: result=%#v err=%v content=%q", captcha, err, content)
	}
	tfaPath := filepath.Join(root, "tfa.png")
	tfaSetup := runIssueJSON(t, "onlinejudge", "tfa-setup", "--output", tfaPath)
	tfaContent, err := os.ReadFile(tfaPath)
	if err != nil || string(tfaContent) != "QR" || tfaSetup["operation"] != "tfa-setup" {
		t.Fatalf("TFA setup QR was not decoded and saved: result=%#v err=%v content=%q", tfaSetup, err, tfaContent)
	}
	requestResult := runIssueJSON(t, "onlinejudge", "password-reset-request", "--email", "user@example.com", "--captcha", "A1B2", "--yes")
	resetResult := runIssueJSON(t, "onlinejudge", "password-reset", "--token", "mail-token", "--captcha", "C3D4", "--new-password", resetPassword, "--password-confirm", resetPassword, "--yes")
	displayID := runIssueJSON(t, "onlinejudge", "refresh-display-id", "--yes")
	avatarPath := filepath.Join(root, "avatar.png")
	if err := os.WriteFile(avatarPath, []byte("avatar-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	avatarResult := runIssueJSON(t, "onlinejudge", "avatar-upload", "--file", avatarPath, "--yes")
	if requestResult["operation"] != "password-reset-request" || resetResult["operation"] != "password-reset" || displayID["operation"] != "refresh-display-id" || avatarResult["evidence"] != "avatar-upload-response-and-profile-readback" || string(avatar) != "avatar-bytes" {
		t.Fatalf("OnlineJudge recovery/avatar evidence missing: request=%#v reset=%#v display=%#v avatar=%#v uploaded=%q", requestResult, resetResult, displayID, avatarResult, avatar)
	}
	serialized, _ := json.Marshal([]map[string]any{requestResult, resetResult, displayID, avatarResult})
	if strings.Contains(string(serialized), resetPassword) {
		t.Fatal("OnlineJudge reset password leaked in result")
	}
}

func TestStaffRecordUnitRequestUploadsAndMapsSemanticFields(t *testing.T) {
	var submitted, uploaded bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		if query.Get("c") == "upload" {
			uploaded = request.Method == http.MethodPost
			if _, _, err := request.FormFile("file"); err != nil {
				t.Fatalf("introduction file was not uploaded: %v", err)
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"state":"success","msg":"uploaded-token"}`))
			return
		}
		if query.Get("c") == "form" && request.Method == http.MethodGet {
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(writer, `<form method="post"><input type="hidden" name="token" value="form-token"><input name="mymyunit"><input name="myintroduce"><input name="myquery_name"><input name="mytel"><textarea name="myname"></textarea><textarea name="mycompany"></textarea><input name="mygoal[]"><textarea name="mymatter"></textarea><textarea name="mycontent"></textarea><input name="mytime"><input name="code"></form>`)
			return
		}
		if query.Get("c") == "form" && request.Method == http.MethodPost {
			if err := request.ParseForm(); err != nil {
				t.Fatalf("parse appointment form: %v", err)
			}
			submitted = request.Form.Get("mymyunit") == "校外单位" &&
				request.Form.Get("myintroduce") == "uploaded-token" &&
				request.Form.Get("myquery_name") == "申请人" &&
				request.Form.Get("mytel") == "13800138000" &&
				request.Form.Get("myname") == "档案对象" &&
				request.Form.Get("mycompany") == "对象单位" &&
				request.Form.Get("mygoal[]") == "查阅" &&
				request.Form.Get("mymatter") == "工作核查" &&
				request.Form.Get("mycontent") == "个人材料" &&
				request.Form.Get("mytime") == "2026-09-15" &&
				request.Form.Get("code") == "1234" &&
				request.Form.Get("token") == "form-token"
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(writer, `<script>alert('预约成功')</script>`)
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()
	root := t.TempDir()
	file := filepath.Join(root, "introduction.txt")
	if err := os.WriteFile(file, []byte("介绍信"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(root, "cookies.txt"))
	result := runIssueJSON(t, "staff-record", "request", "--kind", "unit", "--yes", "--unit", "校外单位", "--applicant-name", "申请人", "--phone", "13800138000", "--subject-name", "档案对象", "--subject-unit", "对象单位", "--usage", "查阅", "--reason", "工作核查", "--content", "个人材料", "--appointment-date", "2026-09-15", "--captcha", "1234", "--introduction-file", file)
	if !uploaded || !submitted {
		t.Fatalf("staff record request was not fully mapped: uploaded=%v submitted=%v result=%#v", uploaded, submitted, result)
	}
	if result["ok"] != true || result["submitted"] != true || result["confirmed"] != true || result["kind"] != "unit" {
		t.Fatalf("appointment success evidence missing: %#v", result)
	}

	personal, err := staffRecordFields([]string{"--subject-name", "本人", "--birth-date", "1980-01-02", "--employee-id", "工号1", "--subject-unit", "本单位", "--applicant-name", "本人", "--phone", "13800138000", "--usage", "开具证明", "--reason", "落户", "--appointment-date", "2026-09-16"}, "personal")
	if err != nil {
		t.Fatal(err)
	}
	fields := make(map[string][]string)
	for _, item := range personal {
		fields[item.name] = append(fields[item.name], item.value)
	}
	if fields["mydate"][0] != "1980-01-02" || fields["mygoal[]"][0] != "开具证明" {
		encoded, _ := json.Marshal(fields)
		t.Fatalf("personal appointment fields were not mapped: %s", encoded)
	}
}

func TestGraduateAdmissionsPasswordResetMapsFormAndSuccess(t *testing.T) {
	var submitted bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		if request.Method == http.MethodGet {
			_, _ = fmt.Fprint(writer, `<form id="Form1" method="post"><input type="hidden" name="__VIEWSTATE" value="view"><input name="txtzjhm"><input name="txtxm"><input name="txtksbh"><input type="submit" name="btnSave" value="重置"></form>`)
			return
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("parse reset form: %v", err)
		}
		submitted = request.Form.Get("__VIEWSTATE") == "view" && request.Form.Get("txtzjhm") == "证件号码" && request.Form.Get("txtxm") == "考生姓名" && request.Form.Get("txtksbh") == "考生编号" && request.Form.Get("btnSave") == "重置"
		_, _ = fmt.Fprint(writer, `<script>alert('密码重置成功!')</script>`)
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))
	result := runIssueJSON(t, "graduate-admissions", "reset-password", "--document-number", "证件号码", "--name", "考生姓名", "--candidate-number", "考生编号", "--yes")
	if !submitted || result["service"] != "graduate-admissions" || result["operation"] != "reset-password" || result["confirmed"] != true {
		t.Fatalf("graduate admissions reset was not mapped: submitted=%v result=%#v", submitted, result)
	}
}
