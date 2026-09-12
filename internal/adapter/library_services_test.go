package adapter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestLibraryServicesListAndDetail(t *testing.T) {
	var posted url.Values
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch request.URL.Path {
		case "/engine2/m/F1E016BEF66C2730":
			_, _ = fmt.Fprint(writer, `<script>var contentVue = new Vue({data:{appId:1443467,engineInstance:{"id":2072950},defaultTypeId:"4959738",sign:"test-sign"}})</script>`)
		case "/engine2/general/1443467/type/more-datas":
			if request.Method != http.MethodPost {
				t.Errorf("service list used %s", request.Method)
			}
			if err := request.ParseForm(); err != nil {
				t.Errorf("service list form is invalid: %v", err)
			} else {
				posted = request.PostForm
			}
			writer.Header().Set("Content-Type", "application/json")
			rows := `[{
				"id":8636850,"title":"你选书，我买单","subtitle":{"value":"荐购服务"},"description":{"value":"推荐图书"},"0":{"value":"https://icon.example/service.png"},"pageType":1,
				"url":"/engine2/general-rest/8636850/proxy-detail-url?sign=raw-signed-value-1234567890","accessBaseNum":12,"publishTime":"2026-09-12","publicUse":true,
				"1":"你选书，我买单","2":"荐购服务","4":"推荐图书","6":"2026-09-12"
			}]`
			_, _ = fmt.Fprintf(writer, `{"code":1,"data":{"datas":{"datas":%s,"pageNum":1,"pageSize":45,"totalPage":1,"totalRecords":1}},"message":"请求正确响应"}`, rows)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "library-services.cookies"))

	list := runIssueJSON(t, "library-services", "list", "--keyword", "科技", "--page", "2", "--page-size", "45")
	if list["total_records"] != float64(1) || list["page"] != float64(1) || list["keyword"] != "科技" {
		t.Fatalf("service list pagination failed: %#v", list)
	}
	if posted.Get("engineInstanceId") != "2072950" || posted.Get("typeId") != "4959738" || posted.Get("pageNum") != "2" || posted.Get("pageSize") != "45" || posted.Get("sw") != "科技" {
		t.Fatalf("service list request mapping failed: %#v", posted)
	}
	item := list["data"].([]any)[0].(map[string]any)
	if item["id"] != "8636850" || item["name"] != "你选书，我买单" || item["subtitle"] != "荐购服务" || item["description"] != "推荐图书" || item["icon_url"] != "https://icon.example/service.png" || item["page_type"] != float64(1) {
		t.Fatalf("service list model failed: %#v", item)
	}
	if strings.Contains(string(mustMarshalIssue(list)), "raw-signed-value") || strings.Contains(string(mustMarshalIssue(list)), "test-sign") {
		t.Fatal("service list leaked signed value")
	}

	detail := runIssueJSON(t, "library-services", "service", "--id", "8636850")
	data := detail["data"].(map[string]any)
	if data["id"] != "8636850" || data["subtitle"] != "荐购服务" {
		t.Fatalf("service detail model failed: %#v", data)
	}
}
