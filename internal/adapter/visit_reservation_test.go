package adapter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestVisitReservationGuideUsesDiscoveredPublicEntry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		if request.URL.Path == "/" {
			_, _ = fmt.Fprint(writer, `<a href="/engine2/m/0/1914391/2728822?p=376911&t=5944357&currentBranch=0">入馆预约</a>`)
			return
		}
		if request.URL.Path == "/engine2/m/0/1914391/2728822" {
			if request.URL.Query().Get("p") != "376911" || request.URL.Query().Get("t") != "5944357" {
				t.Fatalf("预约说明入口参数未被复用: %s", request.URL.RawQuery)
			}
			_, _ = fmt.Fprint(writer, `<script>var pageId = 376911; var typeId = "5944357"; var engineInstanceId = 2728822; var sign = "test-sign";</script>`)
			return
		}
		if request.URL.Path == "/engine2/general/1914391/more/detail" {
			if request.URL.Query().Get("engineInstanceId") != "2728822" || request.URL.Query().Get("typeId") != "5944357" || request.URL.Query().Get("pageId") != "376911" || request.URL.Query().Get("sign") != "test-sign" {
				t.Fatalf("预约说明详情参数错误: %s", request.URL.RawQuery)
			}
			_, _ = fmt.Fprint(writer, `<script>var detailVue = new Vue({data: {generalDataType: {"introduction":"<main><h1>入馆预约</h1><p>方式1：线上预约</p><p>进入预约小程序填写信息。</p><p>关注学校公众号，进入校内服务。</p><p>方式2：线下预约</p></main>"}, generalData: null}});</script>`)
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	result := runIssueJSON(t, "sqyrjd", "guide")
	methods := result["booking_methods"].([]any)
	if result["confirmed"] != true || result["direct_web_form"] != false || result["offline_booking"] != true || len(methods) != 2 || !strings.Contains(result["instructions"].(string), "线上预约") || result["guide_path"] != "/engine2/m/0/1914391/2728822" {
		t.Fatalf("入馆预约说明未被语义化: %#v", result)
	}
}
