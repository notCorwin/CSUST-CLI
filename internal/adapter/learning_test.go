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

func TestLearningAdaptersUseLiveContractsAndSharedModels(t *testing.T) {
	var courseBodies []map[string]any
	var categoryAuth, courseAuth, detailAuth, noticeAuth bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/GetCustomerCategory":
			categoryAuth = request.Header.Get("Authorization") == learningBasicAuthorization
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `[{"CategoryId":1296,"CategoryName":"知识产权类","TypeId":1}]`)
		case "/GetBaseResource":
			courseAuth = request.Header.Get("Authorization") == learningBasicAuthorization
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("professional course body is not JSON: %v", err)
			}
			courseBodies = append(courseBodies, body)
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"Item1":[{"CourseId":101,"YlxCourseId":201,"CourseCode":"P-1","CourseName":"课程一","ExpertName":"老师一","Price":20,"ClassesNum":2,"LearningHours":3,"ClassHours":4}],"Item2":1,"Item3":3}`)
		case "/api/cslgsy/query-resource":
			courseAuth = request.Header.Get("Authorization") == learningBasicAuthorization
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("institutional course body is not JSON: %v", err)
			}
			courseBodies = append(courseBodies, body)
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"List":[{"CourseId":102,"CourseCode":"I-1","CourseName":"课程二","ExpertName":"老师二","LearningHours":2.5}],"TotalCount":1,"TotalLearningHours":2.5}`)
		case "/GetResourceByCode/P-1":
			detailAuth = request.Header.Get("Authorization") == learningBasicAuthorization
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"Id":101,"CourseCode":"P-1","CourseName":"课程一","ExpertName":"老师一","ClassesNum":1,"CourseExtend":{"Description":"%3Cp%3E课程说明%3C/p%3E"},"ClassesList":[{"ClassId":301,"ClassName":"第1课时","Author":"老师一","VideoPath":"https://video.example/signed?key=secret","Timelength":120,"State":1}]}`)
		case "/api/Notice/GetNoticeListPage":
			noticeAuth = request.Header.Get("Authorization") == ""
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"Data":[{"i_id":7,"s_title":"通知一","s_content":"<p>正文</p>","s_createTime":"2026-09-12"}],"StatusCode":0}`)
		case "/api/Notice/GetNoticeInfo":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"Data":{"i_id":7,"s_title":"通知详情","s_content":"<p>完整正文</p>","s_createTime":"2026-09-12"},"StatusCode":0}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	professional := runIssueJSON(t, "professional-learning", "courses", "--kind", "professional", "--category-id", "1296", "--keyword", "课程", "--year", "2026", "--min-hours", "1.5", "--page", "2", "--page-size", "5")
	items := professional["data"].([]any)
	if professional["service"] != "professional-learning" || professional["total"] != float64(1) || len(items) != 1 {
		t.Fatalf("unexpected professional learning result: %#v", professional)
	}
	item := items[0].(map[string]any)
	if item["id"] != "101" || item["code"] != "P-1" || item["learning_hours"] != float64(3) {
		t.Fatalf("professional course model was not mapped: %#v", item)
	}

	categories := runIssueJSON(t, "professional-learning", "categories")
	category := categories["data"].([]any)[0].(map[string]any)
	if category["id"] != "1296" || category["name"] != "知识产权类" || !categoryAuth {
		t.Fatalf("category contract/model failed: %#v auth=%v", categories, categoryAuth)
	}

	notices := runIssueJSON(t, "professional-learning", "notices", "--page", "2", "--page-size", "3")
	if notices["data"].([]any)[0].(map[string]any)["title"] != "通知一" || !noticeAuth {
		t.Fatalf("notice contract/model failed: %#v auth=%v", notices, noticeAuth)
	}
	detailNotice := runIssueJSON(t, "professional-learning", "notice", "--id", "7")
	if detailNotice["notice"].(map[string]any)["content_html"] != "<p>完整正文</p>" {
		t.Fatalf("notice detail was not mapped: %#v", detailNotice)
	}

	detail := runIssueJSON(t, "professional-learning", "course", "--code", "P-1")
	detailCourse := detail["course"].(map[string]any)
	if detailCourse["description"] != "<p>课程说明</p>" || len(detailCourse["classes"].([]any)) != 1 || !detailAuth {
		t.Fatalf("course detail model failed: %#v auth=%v", detail, detailAuth)
	}
	if strings.Contains(string(mustMarshalIssue(detail)), "signed?key=secret") {
		t.Fatal("signed video URL leaked without explicit opt-in")
	}

	video := runIssueJSON(t, "professional-learning", "course", "--id", "P-1", "--include-video-url")
	if !strings.Contains(string(mustMarshalIssue(video)), "signed?key=secret") {
		t.Fatal("explicit video URL opt-in was ignored")
	}

	institutional := runIssueJSON(t, "institutional-learning", "list", "--kind", "public", "--year", "2026")
	institutionalItem := institutional["data"].([]any)[0].(map[string]any)
	if institutional["service"] != "institutional-learning" || institutionalItem["code"] != "I-1" || institutional["total_learning_hours"] != 2.5 {
		t.Fatalf("institutional learning result was not mapped: %#v", institutional)
	}
	if len(courseBodies) != 2 || courseBodies[0]["typeId"] != float64(1296) || courseBodies[0]["minClassHoursLimit"] != 1.5 || courseBodies[1]["typeId"] != float64(2) || courseBodies[1]["maxClassHoursLimit"] != float64(15) {
		t.Fatalf("course filters were not mapped to live contracts: %#v", courseBodies)
	}
	if !courseAuth {
		t.Fatal("course request did not carry the vendor authorization")
	}
}
