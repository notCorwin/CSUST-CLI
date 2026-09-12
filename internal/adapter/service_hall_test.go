package adapter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
)

func TestServiceHallServicesUsesCurrentJSONContract(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/handleHall/getApp" || request.Header.Get("X-Requested-With") != "XMLHttpRequest" {
			t.Fatalf("unexpected service hall request: %s %s %#v", request.Method, request.URL.Path, request.Header)
		}
		if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
			t.Fatalf("invalid service hall body: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"result":1,"data":{"current":2,"pages":3,"total":34,"records":[{"appId":100000125,"name":"校园卡","pcUrl":"http://yktfw.example/ias/prelogin"}]}}`))
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "service-hall.cookies"))

	result := runIssueJSON(t, "service-hall", "services", "--keyword", "校园", "--category-id", "115", "--department-id", "dept-1", "--page", "2", "--page-size", "16", "--favorites")
	if result["operation"] != "services" || result["total"] != float64(34) || result["total_pages"] != float64(3) {
		t.Fatalf("unexpected service hall result: %#v", result)
	}
	if requestBody["classifyId"] != "115" || requestBody["departId"] != "dept-1" || requestBody["page"] != float64(2) || requestBody["row"] != float64(16) || requestBody["keyWord"] != "校园" || requestBody["isCollect"] != float64(1) {
		t.Fatalf("semantic filters were not mapped: %#v", requestBody)
	}
	services := result["services"].([]any)
	if services[0].(map[string]any)["name"] != "校园卡" {
		t.Fatalf("service model lost record: %#v", services[0])
	}
}

func TestServiceHallDictionariesUseSemanticFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("unexpected dictionary method: %s", request.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("invalid dictionary body: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/handleHall/getAppClassify":
			if body["isCollect"] != float64(1) || body["noBusiness"] != float64(1) {
				t.Fatalf("category filters were not mapped: %#v", body)
			}
			_, _ = writer.Write([]byte(`{"result":1,"data":[{"id":115,"name":"信息化服务"}]}`))
		case "/handleHall/getAppDepart":
			if body["classifyId"] != "117" {
				t.Fatalf("department category was not mapped: %#v", body)
			}
			_, _ = writer.Write([]byte(`{"result":1,"data":[{"departId":"dept-1","departName":"教务处"}]}`))
		default:
			t.Fatalf("unexpected dictionary path: %s", request.URL.Path)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "service-hall-dictionaries.cookies"))

	categories := runIssueJSON(t, "service-hall", "categories", "--favorites")
	if categories["total"] != float64(1) || categories["data"].([]any)[0].(map[string]any)["name"] != "信息化服务" {
		t.Fatalf("unexpected categories: %#v", categories)
	}
	departments := runIssueJSON(t, "service-hall", "departments", "--category-id", "117")
	if departments["total"] != float64(1) || departments["data"].([]any)[0].(map[string]any)["departName"] != "教务处" {
		t.Fatalf("unexpected departments: %#v", departments)
	}
}

func TestServiceHallServicesResolveSemanticNames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("unexpected service hall method: %s", request.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("invalid service hall body: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/handleHall/getAppClassify":
			_, _ = writer.Write([]byte(`{"result":1,"data":[{"id":117,"name":"教务教学"}]}`))
		case "/handleHall/getAppDepart":
			if body["classifyId"] != "117" {
				t.Fatalf("department lookup used wrong category: %#v", body)
			}
			_, _ = writer.Write([]byte(`{"result":1,"data":[{"departId":"dept-1","departName":"教务处"}]}`))
		case "/handleHall/getApp":
			if body["classifyId"] != "117" || body["departId"] != "dept-1" {
				t.Fatalf("semantic names were not resolved: %#v", body)
			}
			_, _ = writer.Write([]byte(`{"result":1,"data":{"current":1,"pages":1,"total":0,"records":[]}}`))
		default:
			t.Fatalf("unexpected service hall path: %s", request.URL.Path)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "service-hall-names.cookies"))

	result := runIssueJSON(t, "service-hall", "services", "--category", "教务教学", "--department", "教务处")
	if result["total"] != float64(0) || result["query"].(map[string]any)["category_id"] != "117" || result["query"].(map[string]any)["department_id"] != "dept-1" {
		t.Fatalf("unexpected resolved service hall query: %#v", result)
	}
}

func TestServiceHallGuideUsesGuideProtocol(t *testing.T) {
	var query url.Values
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/handleHall/guide" {
			t.Fatalf("unexpected guide request: %s %s", request.Method, request.URL.Path)
		}
		query = request.URL.Query()
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(`<html><head><title>网络报修指南</title></head><body><h1>网络报修</h1><table><tr><th>办理方式</th><td>在线办理</td></tr></table></body></html>`))
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "service-hall-guide.cookies"))

	result := runIssueJSON(t, "service-hall", "guide", "--id", "guide-1", "--category-id", "115", "--department-id", "dept-1")
	if result["operation"] != "guide" || result["service_id"] != "guide-1" {
		t.Fatalf("unexpected guide result: %#v", result)
	}
	if query.Get("appId") != "guide-1" || query.Get("classifyId") != "115" || query.Get("departId") != "dept-1" {
		t.Fatalf("guide parameters were not mapped: %s", query.Encode())
	}
	page, ok := result["response"].(map[string]any)
	if !ok || page["title"] != "网络报修指南" {
		t.Fatalf("guide page was not structured: %#v", result["response"])
	}
}

func TestServiceHallFavoriteRequiresReadback(t *testing.T) {
	favorite := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method != http.MethodPost {
			t.Fatalf("unexpected favorite method: %s", request.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("invalid favorite body: %v", err)
		}
		switch request.URL.Path {
		case "/handleHall/collect":
			if body["appId"] != "app-1" {
				t.Fatalf("favorite app id was not mapped: %#v", body)
			}
			favorite = true
			_, _ = writer.Write([]byte(`{"result":1,"data":{"changed":true}}`))
		case "/handleHall/cancleCollect":
			favorite = false
			_, _ = writer.Write([]byte(`{"result":1,"data":{"changed":true}}`))
		case "/handleHall/getApp":
			if body["isCollect"] != float64(1) {
				t.Fatalf("favorite readback did not request favorites: %#v", body)
			}
			records := []any{}
			if favorite {
				records = []any{map[string]any{"appId": "app-1", "name": "服务"}}
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"result": 1, "data": map[string]any{
				"current": 1, "pages": 1, "total": len(records), "records": records,
			}})
		default:
			t.Fatalf("unexpected favorite path: %s", request.URL.Path)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "service-hall-favorite.cookies"))

	added := runIssueJSON(t, "service-hall", "favorite", "--service-id", "app-1", "--yes")
	if added["confirmed"] != true || added["favorite"] != true {
		t.Fatalf("favorite was not confirmed: %#v", added)
	}
	removed := runIssueJSON(t, "service-hall", "unfavorite", "--service-id", "app-1", "--yes")
	if removed["confirmed"] != true || removed["favorite"] != false {
		t.Fatalf("unfavorite was not confirmed: %#v", removed)
	}
}

func TestServiceHallRateUsesCommitAndReadback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("unexpected rating method: %s", request.Method)
		}
		writer.Header().Set("Content-Type", "application/json")
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("invalid rating body: %v", err)
		}
		switch request.URL.Path {
		case "/evaluate/commit":
			if body["appId"] != "app-1" || body["rating"] != 4.5 || body["evaluate"] != "很好" {
				t.Fatalf("rating was not mapped: %#v", body)
			}
			_, _ = writer.Write([]byte(`{"result":1,"data":{"id":"rating-1"}}`))
		case "/evaluate/my":
			_, _ = writer.Write([]byte(`{"result":1,"data":{"rating":4.5,"evaluate":"很好"}}`))
		default:
			t.Fatalf("unexpected rating path: %s", request.URL.Path)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "service-hall-rating.cookies"))

	result := runIssueJSON(t, "service-hall", "rate", "--service-id", "app-1", "--rating", "4.5", "--comment", "很好", "--yes")
	if result["confirmed"] != true || result["verified_rating"] != 4.5 || result["service_id"] != "app-1" {
		t.Fatalf("rating was not confirmed: %#v", result)
	}
}
