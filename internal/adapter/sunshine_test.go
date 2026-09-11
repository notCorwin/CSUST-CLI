package adapter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSunshinePublicQueriesAndCodeConfirmation(t *testing.T) {
	var issueQuery map[string]any
	var departmentQuery map[string]any
	var phone string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/issues":
			if request.Method != http.MethodPut || json.NewDecoder(request.Body).Decode(&issueQuery) != nil {
				t.Fatalf("unexpected issue query")
			}
			_, _ = writer.Write([]byte(`{"success":true,"data":{"rows":[{"no":7,"status":"受理中"}],"total":1}}`))
		case "/api/issues/issue-1":
			if request.Method != http.MethodPost {
				t.Fatalf("unexpected issue detail method: %s", request.Method)
			}
			_, _ = writer.Write([]byte(`{"success":true,"data":{"_id":"issue-1","status":"已处理"}}`))
		case "/api/departments":
			if request.Method != http.MethodPut || json.NewDecoder(request.Body).Decode(&departmentQuery) != nil {
				t.Fatalf("unexpected department query")
			}
			_, _ = writer.Write([]byte(`{"success":true,"data":[{"_id":"d1","name":"信息化处"}]}`))
		case "/api/issuestat":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"numTotal":3353}}`))
		case "/api/systems":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"issueAttachmentMaxNum":1}}`))
		case "/api/verifys":
			var body map[string]string
			if request.Method != http.MethodPost || json.NewDecoder(request.Body).Decode(&body) != nil {
				t.Fatalf("unexpected verification request")
			}
			phone = body["tel"]
			_, _ = writer.Write([]byte(`{"success":true,"data":[{"_id":"verify-1"}]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	issues := runIssueJSON(t, "sunshine", "issues", "--status", "受理中")
	if issues["service"] != "sunshine" || issues["operation"] != "issues" || issues["data"] == nil {
		t.Fatalf("unexpected sunshine issues result: %#v", issues)
	}
	filter := issueQuery["filter"].(map[string]any)
	if filter["isPublic"] != true || filter["status"].(map[string]any)["$in"].([]any)[0] != "受理中" {
		t.Fatalf("public status filter was not mapped: %#v", issueQuery)
	}

	detail := runIssueJSON(t, "sunshine", "issue", "--id", "issue-1")
	if detail["data"].(map[string]any)["_id"] != "issue-1" {
		t.Fatalf("unexpected sunshine detail: %#v", detail)
	}
	departments := runIssueJSON(t, "sunshine", "departments")
	if departments["data"] == nil || departmentQuery["paginator"].(map[string]any)["needAll"] != true {
		t.Fatalf("unexpected sunshine departments: %#v %#v", departments, departmentQuery)
	}
	if stats := runIssueJSON(t, "sunshine", "stats"); stats["data"].(map[string]any)["numTotal"] != float64(3353) {
		t.Fatalf("unexpected sunshine stats: %#v", stats)
	}
	if config := runIssueJSON(t, "sunshine", "config"); config["data"].(map[string]any)["issueAttachmentMaxNum"] != float64(1) {
		t.Fatalf("unexpected sunshine config: %#v", config)
	}
	if code := runIssueJSON(t, "sunshine", "send-code", "--phone", "13800138000", "--yes"); code["verified_by"] != "response-success" || phone != "13800138000" {
		t.Fatalf("unexpected sunshine code result: %#v phone=%s", code, phone)
	}
}

func TestSunshineSuggestionSubmitsSemanticPayloadAndReadsBack(t *testing.T) {
	var submitted map[string]any
	var detailBody map[string]any
	var uploadedName, uploadedSize string
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		requests++
		switch request.URL.Path {
		case "/api/systems":
			if request.Method != http.MethodGet {
				t.Fatalf("unexpected config request")
			}
			_, _ = writer.Write([]byte(`{"success":true,"data":{"issueAttachmentMaxNum":1,"issueAttachmentMaxSize":10485760}}`))
		case "/api/departments":
			if request.Method != http.MethodPut || json.NewDecoder(request.Body).Decode(&detailBody) != nil {
				t.Fatalf("unexpected department request")
			}
			_, _ = writer.Write([]byte(`{"success":true,"data":[{"_id":"department-1","name":"信息化处","nickname":"信息化处"}]}`))
		case "/api/uploadfiles":
			if request.Method != http.MethodPost || request.ParseMultipartForm(1<<20) != nil {
				t.Fatalf("unexpected attachment request")
			}
			items := request.MultipartForm.File["file"]
			if len(items) != 1 {
				t.Fatalf("unexpected attachment file parts: %#v", request.MultipartForm.File)
			}
			uploadedSize = request.FormValue("size")
			uploadedName = items[0].Filename
			file, openErr := items[0].Open()
			if openErr != nil {
				t.Fatalf("open attachment: %v", openErr)
			}
			content, readErr := io.ReadAll(file)
			_ = file.Close()
			if readErr != nil || string(content) != "payload" {
				t.Fatalf("unexpected attachment content: %q err=%v", content, readErr)
			}
			_, _ = writer.Write([]byte(`{"success":true,"data":{"_id":"attachment-1","size":7,"name":"说明.pdf"}}`))
		case "/api/issues":
			if request.Method != http.MethodPost || json.NewDecoder(request.Body).Decode(&submitted) != nil {
				t.Fatalf("unexpected submit request")
			}
			_, _ = writer.Write([]byte(`{"success":true,"data":{"_id":"issue-1"}}`))
		case "/api/issues/issue-1":
			if request.Method != http.MethodPost || json.NewDecoder(request.Body).Decode(&detailBody) != nil {
				t.Fatalf("unexpected readback request")
			}
			_, _ = writer.Write([]byte(`{"success":true,"data":{"_id":"issue-1","name":"操场分区建议","type":"建议咨询","department":"department-1"}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	attachmentPath := filepath.Join(t.TempDir(), "说明.pdf")
	if err := os.WriteFile(attachmentPath, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	handled, _, _, code, err := (NativeSite{}).Run(context.Background(), []string{"sunshine", "suggestion", "--title", "操场分区建议", "--department", "信息化处", "--content", "请说明校区、具体事由和希望的处理方式，内容足够详细。", "--reporter", "张三", "--phone", "13800138000", "--email", "user@example.com", "--role", "student", "--code", "123456", "--expected-date", "2099-01-02", "--json"}, true)
	if err != nil || !handled || code != 2 || requests != 0 {
		t.Fatalf("submit should require confirmation: handled=%v code=%d err=%v requests=%d", handled, code, err, requests)
	}

	result := runIssueJSON(t, "sunshine", "suggestion", "--title", "操场分区建议", "--department", "信息化处", "--content", "请说明校区、具体事由和希望的处理方式，内容足够详细。", "--reporter", "张三", "--phone", "13800138000", "--email", "user@example.com", "--role", "student", "--code", "123456", "--expected-date", "2099-01-02", "--attachment", attachmentPath, "--yes")
	if result["submitted"] != true || result["confirmed"] != true || result["issue_id"] != "issue-1" || result["evidence"] != "POST /api/issues success and detail readback" {
		t.Fatalf("unexpected sunshine submit result: %#v", result)
	}
	if submitted["name"] != "操场分区建议" || submitted["department"] != "department-1" || submitted["type"] != "建议咨询" || submitted["reporter"] != "张三" || submitted["role"] != "本校学生" || submitted["verifyCode"] != "123456" || submitted["needVerifyCode"] != true || submitted["isPublic"] != true {
		t.Fatalf("semantic submit payload was not mapped: %#v", submitted)
	}
	attachments, ok := submitted["attachments"].([]any)
	if !ok || len(attachments) != 1 || uploadedName != "说明.pdf" || uploadedSize != "7" {
		t.Fatalf("attachment was not uploaded or mapped: attachments=%#v name=%q size=%q", submitted["attachments"], uploadedName, uploadedSize)
	}
	if date, _ := submitted["dateExpected"].(string); func() bool {
		parsed, parseErr := time.Parse(time.RFC3339Nano, date)
		return parseErr != nil || parsed.In(time.Local).Format("2006-01-02") != "2099-01-02"
	}() {
		t.Fatalf("unexpected expected date: %#v", submitted["dateExpected"])
	}
	if attachments, ok := submitted["attachments"].([]any); !ok || len(attachments) != 1 {
		t.Fatalf("unexpected attachment payload: %#v", submitted["attachments"])
	}
	encoded := string(mustJSON(result))
	if encoded == "" || strings.Contains(encoded, "123456") || strings.Contains(encoded, "13800138000") {
		t.Fatalf("submission secrets leaked in result: %#v", result)
	}
}
