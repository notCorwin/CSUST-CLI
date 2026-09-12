package adapter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestHighwayExperimentPublicResourcesAndBookingInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch request.URL.Path {
		case "/pclass/":
			if request.URL.Query().Get("classa") != "1" || request.URL.Query().Get("classb") != "1" || request.URL.Query().Get("page") != "2" {
				t.Fatalf("resource filters were not mapped: %s", request.URL.RawQuery)
			}
			_, _ = fmt.Fprint(writer, `<html><body><div class="show_lk"><a href="../pclass/?classa=1&classb=1">土工类</a></div><div class="show_r"><a href="../pro/?proid=47"><img src="../pic/rock.jpg">多功能岩石三轴仪</a><a href="../pro/?proid=48">慢拉伸腐蚀试验机</a><a href="?page=3&classa=1">3</a></div></body></html>`)
		case "/pro/":
			if request.URL.Query().Get("proid") != "50" {
				t.Fatalf("resource id was not mapped: %s", request.URL.RawQuery)
			}
			_, _ = fmt.Fprint(writer, `<html><body><div class="show_r"><div class="show_rt1">DSC214差示扫描量热仪</div><div class="show_rkc"><img src="../pic/dsc.jpg"><br>差示扫描量热仪</div><div class="show_rk">用于材料热分析。</div><div class="show_rkk"><a href="../pro/?proid=49">材料力学性能表征测量系统</a></div></div></body></html>`)
		case "/about/":
			if request.URL.Query().Get("showid") != "56" {
				t.Fatalf("booking info id was not mapped: %s", request.URL.RawQuery)
			}
			_, _ = fmt.Fprint(writer, `<html><body><div class="show_r"><div class="show_rt1">预约须知</div><div class="show_rk">预约设备前请先联系设备管理员。</div></div></body></html>`)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	resources := runIssueJSON(t, "highway-experiment", "resources", "--category", "土工类", "--keyword", "三轴", "--page", "2")
	items := resources["data"].([]any)
	if resources["total_pages"] != float64(3) || len(items) != 1 || items[0].(map[string]any)["id"] != "47" {
		t.Fatalf("unexpected resources: %#v", resources)
	}

	detail := runIssueJSON(t, "highway-experiment", "resource", "--id", "50")
	data := detail["data"].(map[string]any)
	if data["name"] != "DSC214差示扫描量热仪" || data["description"] != "用于材料热分析。" || !strings.HasSuffix(data["image_url"].(string), "/pic/dsc.jpg") {
		t.Fatalf("unexpected resource detail: %#v", detail)
	}

	guide := runIssueJSON(t, "highway-experiment", "booking-info")
	if !strings.Contains(guide["data"].(map[string]any)["content"].(string), "联系设备管理员") {
		t.Fatalf("booking guidance was not returned: %#v", guide)
	}
}
