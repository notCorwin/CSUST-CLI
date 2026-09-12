package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestStudentDigitalArchiveQueriesAndMutations(t *testing.T) {
	state := struct {
		note   bool
		labels []string
	}{labels: []string{"系统计算标签"}}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Header.Get("Authentication") != "token" {
			writer.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprint(writer, `{"code":401,"message":"未登录"}`)
			return
		}
		success := func(data any) {
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 2000, "message": "请求成功", "data": data})
		}
		switch request.URL.Path {
		case "/tsa/shc/homepage/personInfo":
			success(map[string]any{"personInfo": map[string]any{"USERID": "student", "USERNAME": "测试用户", "TEL": "13800138000"}, "column": []any{}})
		case "/tsa/shc/learnInfo/learnSituation":
			success(map[string]any{"TOTALUNIT": 160, "GOTTENUNIT": 49.5})
		case "/tsa/shc/learnInfo/scoreCardList":
			if request.URL.Query().Get("_USER") != "" {
				t.Errorf("student id should remain optional: %q", request.URL.Query().Get("_USER"))
			}
			success([]any{map[string]any{"SCHOOLYEAR": "2025-2026", "TERM": "1", "scoreList": []any{map[string]any{"COURSEMC": "课程", "PERCENTSCORE": 90}}}})
		case "/tsa/shc/learnInfo/classScheduleGrid":
			if request.URL.Query().Get("termdm") != "2025-2026-1" || request.URL.Query().Get("page") != "2" {
				t.Errorf("semantic schedule parameters were not mapped: %v", request.URL.Query())
			}
			success(map[string]any{"total": 1, "rowsList": []any{map[string]any{"COURSEMC": "课程"}}})
		case "/tsa/shc/personalInfo/formCatalogList":
			success([]any{map[string]any{"CATALOGID": "A08B3F7F6A621CA4E0537D00A8C0E5FF", "CATALOGNAME": "考试成绩", "children": []any{map[string]any{"formId": "form-1", "FORMNAME": "成绩明细"}}}})
		case "/tsa/shc/personalInfo/getFormListByCatalogId":
			success([]any{map[string]any{"formId": "form-1", "FORMNAME": "成绩明细"}})
		case "/tsa/shc/personalInfo/getCommonFormData/form-1":
			if request.Method != http.MethodPost {
				t.Errorf("records must use POST")
			}
			success(map[string]any{"dataDetail": []any{map[string]any{"XH": "student", "COURSEMC": "课程"}}})
		case "/tsa/pdp/notes/notesList":
			success(map[string]any{"total": map[bool]int{true: 1, false: 0}[state.note], "rowsList": func() []any {
				if !state.note {
					return []any{}
				}
				return []any{map[string]any{"WID": "note-1", "TITLE": "标题", "NOTEDESC": "内容"}}
			}()})
		case "/tsa/pdp/notes/saveNotes":
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body["title"] != "标题" || body["noteDesc"] != "内容" {
				t.Fatalf("note body was not mapped: %#v", body)
			}
			state.note = true
			success(map[string]any{"WID": "note-1"})
		case "/tsa/pdp/notes/deleteNotes":
			state.note = false
			success(nil)
		case "/tsa/shc/homepage/getStuLableList":
			selected := make([]any, len(state.labels))
			for index, label := range state.labels {
				selected[index] = label
			}
			success(map[string]any{"selectedLable": selected, "calculateLable": []any{"系统计算标签"}})
		case "/tsa/shc/homepage/saveSelectedLable":
			var body []string
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			state.labels = body
			success(nil)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_STUDENT_DIGITAL_ARCHIVE_TOKEN", "token")
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	profile := runIssueJSON(t, "student-digital-archive", "profile")
	encoded := string(mustMarshalIssue(profile))
	if strings.Contains(encoded, "13800138000") || strings.Contains(encoded, "测试用户") || strings.Contains(encoded, `"USERID":"student"`) {
		t.Fatalf("digital archive profile leaked private fields: %s", encoded)
	}
	if got := runIssueJSON(t, "student-digital-archive", "credits"); got["operation"] != "credit" {
		t.Fatalf("credit query failed: %#v", got)
	}
	_ = runIssueJSON(t, "student-digital-archive", "grades")
	_ = runIssueJSON(t, "student-digital-archive", "schedule", "--term", "2025-2026-1", "--page", "2")
	_ = runIssueJSON(t, "student-digital-archive", "records", "--category", "exams")
	_ = runIssueJSON(t, "student-digital-archive", "notes")
	if saved := runIssueJSON(t, "student-digital-archive", "save-note", "--title", "标题", "--content", "内容", "--yes"); saved["confirmed"] != true {
		t.Fatalf("note save was not confirmed: %#v", saved)
	}
	if set := runIssueJSON(t, "student-digital-archive", "set-labels", "--label", "自定义标签", "--yes"); set["confirmed"] != true {
		t.Fatalf("label update was not confirmed: %#v", set)
	}
	if deleted := runIssueJSON(t, "student-digital-archive", "delete-note", "--id", "note-1", "--yes"); deleted["confirmed"] != true {
		t.Fatalf("note delete was not confirmed: %#v", deleted)
	}
}
