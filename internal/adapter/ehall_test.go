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
	var pageReferer string
	var cardRequest map[string]any
	var favoriteState bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Header.Get("X-Requested-With") != "XMLHttpRequest" || request.Header.Get("localeLang") != "zh_CN" {
			t.Fatalf("eHall headers were not sent: %#v", request.Header)
		}
		switch request.URL.Path {
		case "/collectService":
			if request.Method != http.MethodGet || request.URL.Query().Get("id") != "svc-1" {
				t.Fatalf("favorite request = %s %s", request.Method, request.URL.RawQuery)
			}
			favoriteState = request.URL.Query().Get("operate") == "1"
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"errcode": "0", "errmsg": "请求成功", "data": map[string]any{"changed": true},
			})
		case "/queryFolderAndService":
			if request.Method != http.MethodPost {
				t.Fatalf("favorites method = %s", request.Method)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"errcode": "0", "errmsg": "请求成功", "data": []any{map[string]any{"wid": "folder-1", "folderName": "默认收藏夹"}},
			})
		case "/getMessageCount":
			if request.Method != http.MethodGet {
				t.Fatalf("message count method = %s", request.Method)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"errcode": "0", "errmsg": "请求成功", "data": "3"})
		case "/userNotify/getNewsNotify":
			if request.Method != http.MethodGet {
				t.Fatalf("notifications method = %s", request.Method)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"errcode": "0", "errmsg": "请求成功", "data": []any{map[string]any{"title": "通知", "read": false}},
			})
		case "/userNotify/getRecommendCycle":
			if request.Method != http.MethodGet {
				t.Fatalf("service cycles method = %s", request.Method)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"errcode": "0", "errmsg": "请求成功", "data": []any{map[string]any{"type": 0, "cycleName": "考试报名", "list": []any{}}},
			})
		case "/execCardMethod/mail-card/CUS_CARD_TENCENTMAIL":
			var body map[string]any
			if request.Method != http.MethodPost || json.NewDecoder(request.Body).Decode(&body) != nil {
				t.Fatalf("mail card request was not JSON POST")
			}
			switch body["method"] {
			case "ifRegister":
				_ = json.NewEncoder(writer).Encode(map[string]any{"errcode": "0", "errmsg": "请求成功", "data": map[string]any{"errcode": "0", "errmsg": "请求成功"}})
			case "unReadMail":
				_ = json.NewEncoder(writer).Encode(map[string]any{"errcode": "0", "errmsg": "请求成功", "data": map[string]any{"errcode": "0", "errmsg": "请求成功", "count": "4"}})
			default:
				t.Fatalf("unexpected mail card method: %v", body["method"])
			}
		case "/execCardMethod/news-card/SYS_CARD_NEWSANNOUNCEMENT":
			var body map[string]any
			if request.Method != http.MethodPost || json.NewDecoder(request.Body).Decode(&body) != nil {
				t.Fatalf("news card request was not JSON POST")
			}
			switch body["method"] {
			case "getNewsConfig":
				_ = json.NewEncoder(writer).Encode(map[string]any{"errcode": "0", "errmsg": "请求成功", "data": map[string]any{"newsTotal": 3}})
			case "getConfiguredAndSubscribedChannel":
				_ = json.NewEncoder(writer).Encode(map[string]any{"errcode": "0", "errmsg": "请求成功", "data": map[string]any{
					"configuredChannel": []any{map[string]any{"id": "configured-1", "name": "教务通知"}}, "subscribedChannel": []any{map[string]any{"wid": "channel-1", "name": "教务通知", "type": 0}},
				}})
			case "getChannelNews":
				param := body["param"].(map[string]any)
				if param["channelIds"] != "channel-1" || param["programIds"] != "" || param["pageNumber"] != float64(2) {
					t.Fatalf("news query params were not semantic: %#v", param)
				}
				_ = json.NewEncoder(writer).Encode(map[string]any{"errcode": "0", "errmsg": "请求成功", "data": map[string]any{
					"datas": map[string]any{"data": []any{map[string]any{"wid": "news-1", "title": "选课通知"}}, "totalSize": 4, "pageSize": 3, "pageNumber": 2},
				}})
			default:
				t.Fatalf("unexpected news card method: %v", body["method"])
			}
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
			pageReferer = request.Header.Get("Referer")
			layout, _ := json.Marshal([]any{map[string]any{"columns": []any{map[string]any{"card": map[string]any{
				"cardId": "SYS_CARD_SERVICEBUS", "cardWid": "card-1",
			}}, map[string]any{"card": map[string]any{
				"cardId": "SYS_CARD_NEWSANNOUNCEMENT", "cardWid": "news-card", "cardName": "通知公告",
			}}, map[string]any{"card": map[string]any{
				"cardId": "CUS_CARD_TENCENTMAIL", "cardWid": "mail-card", "cardName": "企业邮箱",
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
					"appData":      []any{map[string]any{"serviceId": "svc-1", "serviceName": "教务系统", "permission": true, "appFavorite": favoriteState}},
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
	if pageQuery.Get("pageCode") != "" || pageQuery.Get("lang") != "zh_CN" || pageQuery.Get("originalUrl") == "" || pageReferer == "" {
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
	messageCount := runIssueJSON(t, "ehall", "message-count", "--cookie-file", cookie)
	if messageCount["message_count"] != float64(3) || messageCount["data"] != "3" {
		t.Fatalf("eHall message count was not normalized: %#v", messageCount)
	}
	notifications := runIssueJSON(t, "ehall", "notifications", "--cookie-file", cookie)
	if notifications["notification_count"] != float64(1) || notifications["notifications"].([]any)[0].(map[string]any)["title"] != "通知" {
		t.Fatalf("eHall notifications were not preserved: %#v", notifications)
	}
	cycles := runIssueJSON(t, "ehall", "service-cycles", "--cookie-file", cookie)
	if cycles["cycle_count"] != float64(1) || cycles["cycles"].([]any)[0].(map[string]any)["cycleName"] != "考试报名" {
		t.Fatalf("eHall service cycles were not preserved: %#v", cycles)
	}
	mail := runIssueJSON(t, "ehall", "mail-status", "--cookie-file", cookie)
	if mail["registered"] != true || mail["unread_count"] != float64(4) {
		t.Fatalf("eHall mail status was not normalized: %#v", mail)
	}
	if mail["entrypoints"].(map[string]any)["password_reset"] != "http://txyj.csust.edu.cn/Mail/ChangePass" {
		t.Fatalf("eHall mail entrypoints were not preserved: %#v", mail)
	}
	news := runIssueJSON(t, "ehall", "news", "--channel", "教务", "--page", "2", "--cookie-file", cookie)
	if news["item_count"] != float64(1) || news["card_count"] != float64(1) {
		t.Fatalf("eHall news was not normalized: %#v", news)
	}
	newsCard := news["cards"].([]any)[0].(map[string]any)
	if newsCard["title"] != "通知公告" || newsCard["items"].([]any)[0].(map[string]any)["title"] != "选课通知" {
		t.Fatalf("eHall news item was not preserved: %#v", newsCard)
	}
	if newsCard["configured_channels"].([]any)[0].(map[string]any)["id"] != "configured-1" {
		t.Fatalf("eHall configured channels were not preserved: %#v", newsCard)
	}

	added := runIssueJSON(t, "ehall", "favorite", "add", "--service-id", "svc-1", "--yes", "--cookie-file", cookie)
	if added["operation"] != "favorite-add" || added["favorite"] != true || added["evidence"] != "readback" {
		t.Fatalf("favorite add was not read back: %#v", added)
	}
	removed := runIssueJSON(t, "ehall", "favorite", "remove", "--service-id", "svc-1", "--yes", "--cookie-file", cookie)
	if removed["operation"] != "favorite-remove" || removed["favorite"] != false || removed["evidence"] != "readback" {
		t.Fatalf("favorite remove was not read back: %#v", removed)
	}
}
