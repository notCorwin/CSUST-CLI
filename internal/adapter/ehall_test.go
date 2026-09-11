package adapter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
)

func TestEhallServicesAndDetailUseSemanticProtocol(t *testing.T) {
	var pageQuery url.Values
	var cardRequest map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Header.Get("X-Requested-With") != "XMLHttpRequest" || request.Header.Get("localeLang") != "zh_CN" {
			t.Fatalf("eHall headers were not sent: %#v", request.Header)
		}
		switch request.URL.Path {
		case "/queryFolderAndService":
			if request.Method != http.MethodPost {
				t.Fatalf("favorites method = %s", request.Method)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"errcode": "0", "errmsg": "请求成功", "data": []any{map[string]any{"wid": "folder-1", "folderName": "默认收藏夹"}},
			})
		case "/getLoginUserAndGuest":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"errcode": "0", "errmsg": "请求成功", "data": map[string]any{
					"wid": "user-1", "userAccount": "account-1", "userName": "张三",
					"categoryName": "学生/本专科生", "categoryWid": "category-1",
					"deptName": "计算机学院", "deptWid": "dept-1", "groups": []any{map[string]any{"wid": "group-1"}},
					"orgs": []any{map[string]any{"wid": "org-1"}}, "preferredLanguage": "zh_CN", "portalDefaultLang": "zh_CN",
				},
			})
		case "/getPageView":
			pageQuery = request.URL.Query()
			layout, _ := json.Marshal([]any{map[string]any{"columns": []any{map[string]any{"card": map[string]any{
				"cardId": "SYS_CARD_SERVICEBUS", "cardWid": "card-1",
			}}}}})
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"errcode": "0", "errmsg": "请求成功", "data": map[string]any{
					"siteWid": "-100", "portalName": "融合门户", "pageTitle": "服务门户",
					"portalDomain": "http://example.test",
					"pageContext":  map[string]any{"pageInfoEntity": map[string]any{"cardLayout": string(layout)}},
				},
			})
		case "/execCardMethod/card-1/SYS_CARD_SERVICEBUS":
			if request.Method != http.MethodPost {
				t.Fatalf("card render method = %s", request.Method)
			}
			if err := json.NewDecoder(request.Body).Decode(&cardRequest); err != nil {
				t.Fatalf("decode card request: %v", err)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"errcode": "0", "errmsg": "请求成功", "data": map[string]any{
					"appData":      []any{map[string]any{"serviceId": "svc-1", "serviceName": "教务系统", "permission": true}},
					"classifyData": []any{map[string]any{"typeId": "type-1", "typeName": "本科生单点", "count": 1}},
				},
			})
		case "/queryServiceByWid/svc-1/0":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"errcode": "0", "errmsg": "请求成功", "data": map[string]any{
					"login": true, "isLogin": true, "localLang": "zh_CN",
					"serviceInfo": []any{map[string]any{"wid": "svc-1", "serviceName": "教务系统", "hasPermission": true}},
				},
			})
		case "/serviceShow":
			if request.URL.Query().Get("isMobile") != "0" || request.URL.Query().Get("serviceId") != "svc-1" {
				t.Fatalf("service detail query = %s", request.URL.RawQuery)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"errcode": "0", "errmsg": "请求成功", "data": map[string]any{
					"serviceName": "教务系统", "serviceUrl": "https://example.test/sso", "grantData": nil,
				},
			})
		case "/service/getHealthInfo":
			if request.URL.Query().Get("serviceWid") != "svc-1" {
				t.Fatalf("health query = %s", request.URL.RawQuery)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"errcode": "0", "errmsg": "success", "data": map[string]any{
					"wid": "svc-1", "pcHttpCode": 200, "mobileHttpCode": 200, "serviceLimitVisit": 0,
				},
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	cookie := filepath.Join(t.TempDir(), "ehall.cookies")

	result := runIssueJSON(t, "ehall", "services", "--cookie-file", cookie)
	if pageQuery.Get("pageCode") != "" || pageQuery.Get("lang") != "zh_CN" || pageQuery.Get("originalUrl") == "" {
		t.Fatalf("unexpected page query: %s", pageQuery.Encode())
	}
	if cardRequest["cardId"] != "SYS_CARD_SERVICEBUS" || cardRequest["cardWid"] != "card-1" || cardRequest["method"] != "renderData" {
		t.Fatalf("service bus request was not mapped: %#v", cardRequest)
	}
	services, ok := result["services"].([]any)
	if !ok || len(services) != 1 || services[0].(map[string]any)["serviceName"] != "教务系统" {
		t.Fatalf("service catalog lost semantic data: %#v", result)
	}
	if result["service_count"] != float64(1) || result["scope"] != "available-to-current-user" {
		t.Fatalf("service catalog metadata missing: %#v", result)
	}

	detail := runIssueJSON(t, "ehall", "service", "--id", "svc-1", "--cookie-file", cookie)
	if detail["service_id"] != "svc-1" || detail["access"].(map[string]any)["serviceUrl"] != "https://example.test/sso" {
		t.Fatalf("service detail was not preserved: %#v", detail)
	}
	health := runIssueJSON(t, "ehall", "health", "--id", "svc-1", "--cookie-file", cookie)
	if health["service_id"] != "svc-1" || health["health"].(map[string]any)["pcHttpCode"] != float64(200) {
		t.Fatalf("service health was not preserved: %#v", health)
	}

	me := runIssueJSON(t, "ehall", "me", "--cookie-file", cookie)
	if me["logged_in"] != true || me["user"].(map[string]any)["category"] != "学生/本专科生" {
		t.Fatalf("eHall identity was not normalized: %#v", me)
	}

	favorites := runIssueJSON(t, "ehall", "favorites", "--cookie-file", cookie)
	if favorites["folder_count"] != float64(1) || favorites["folders"].([]any)[0].(map[string]any)["folderName"] != "默认收藏夹" {
		t.Fatalf("eHall favorites were not preserved: %#v", favorites)
	}
}
