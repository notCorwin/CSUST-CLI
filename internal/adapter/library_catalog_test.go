package adapter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestLibraryCatalogSearchAndBook(t *testing.T) {
	var searchQuery string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch request.URL.Path {
		case "/search":
			searchQuery = request.URL.RawQuery
			_, _ = fmt.Fprint(writer, `<div id="search_meta">检索词: 人工智能, 检索到: 2 条结果</div><div class="meneame"><span class="disabled">共 2 页</span></div><div class="bookmeta" bookrecno="42" booktype="1"><span class="bookmetaTitle"><a class="title-link" href="javascript:bookDetail(42,1,0);">测试书</a></span><div>著者: <a class="author-link">测试作者</a></div><div>出版社: <a class="publisher-link">测试出版社</a> 出版日期: 2024</div><div>文献类型: 图书, 索书号: <span class="callnosSpan">TP1/A1</span></div></div>`)
		case "/book/holdingPreviews":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"previews":{"42":[{"bookrecno":42,"callno":"TP1/A1","curlib":"CSLG","curlibName":"长沙理工大学图书馆","curlocal":"0862","curlocalName":"云塘馆","copycount":2,"loanableCount":1,"shelfno":"A1","barcode":"B1"}]}}`)
		case "/book/42":
			_, _ = fmt.Fprint(writer, `<table id="bookInfoTable"><tr><td colspan="2"><h2>测试书</h2></td></tr><tr><td class="leftTD">ISBN:</td><td class="rightTD">978-7-1234-5678-9 价格： CNY50</td></tr><tr><td class="leftTD">主要责任者:</td><td class="rightTD"><a>测试作者</a> 主编</td></tr><tr><td class="leftTD">主题词:</td><td class="rightTD"><a>人工智能</a><a>教材</a></td></tr><tr><td class="leftTD">内容提要:</td><td class="rightTD">测试摘要</td></tr></table>`)
		case "/api/holding/42":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"holdingList":[{"recno":7,"bookrecno":42,"state":2,"barcode":"B1","callno":"TP1/A1","orglib":"CSLG","orglocal":"0862","curlib":"CSLG","curlocal":"0862","cirtype":"0001","shelfno":"A1","totalLoanNum":3,"totalRenewNum":1}],"libcodeMap":{"CSLG":"长沙理工大学图书馆"},"localMap":{"0862":"云塘馆"},"holdStateMap":{"2":{"stateName":"在馆"}},"pBCtypeMap":{"0001":{"name":"中文图书"}}}`)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "library.cookies"))

	search := runIssueJSON(t, "library", "search", "--query", "人工智能", "--field", "title", "--page", "2", "--page-size", "1", "--sort", "title", "--order", "asc", "--in-library")
	if search["total"] != float64(2) || search["total_pages"] != float64(2) || search["in_library"] != true {
		t.Fatalf("library search pagination failed: %#v", search)
	}
	item := search["data"].([]any)[0].(map[string]any)
	if item["id"] != "42" || item["title"] != "测试书" || item["publisher"] != "测试出版社" || item["available_count"] != float64(1) {
		t.Fatalf("library search model failed: %#v", item)
	}
	if searchQuery == "" || !containsAll(searchQuery, "searchWay=title", "rows=1", "sortWay=title_sort", "sortOrder=asc", "hasholding=1") {
		t.Fatalf("library search request mapping failed: %s", searchQuery)
	}

	book := runIssueJSON(t, "library", "book", "--id", "42")
	data := book["book"].(map[string]any)
	if data["title"] != "测试书" || data["isbn"] != "978-7-1234-5678-9" || data["summary"] != "测试摘要" || data["holding_total"] != float64(1) {
		t.Fatalf("library book model failed: %#v", data)
	}
	holding := data["holdings"].([]any)[0].(map[string]any)
	if holding["state_name"] != "在馆" || holding["library"] != "长沙理工大学图书馆" || holding["location"] != "云塘馆" {
		t.Fatalf("library holding model failed: %#v", holding)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
