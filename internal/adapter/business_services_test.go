package adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
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
