package adapter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
