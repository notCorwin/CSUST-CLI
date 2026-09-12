package adapter

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const libraryServicesService = "library-services"

var (
	libraryServicesAppIDPattern          = regexp.MustCompile(`\bappId\s*:\s*(\d+)`)
	libraryServicesEngineInstancePattern = regexp.MustCompile(`\bengineInstance\s*:\s*\{"id"\s*:\s*(\d+)`)
	libraryServicesTypeIDPattern         = regexp.MustCompile(`\bdefaultTypeId\s*:\s*["'](\d+)["']`)
	libraryServicesSignPattern           = regexp.MustCompile(`\bsign\s*:\s*["']([A-Za-z0-9_-]+)["']`)
)

type libraryServicesConfig struct {
	appID            string
	engineInstanceID string
	typeID           string
	sign             string
}

func (a NativeSite) executeLibraryServices(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return businessCatalogFilter(libraryServicesService), nil
	}
	cookie, _, valueErr := businessValue(args, "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "list", "services":
		return a.libraryServicesList(ctx, args[1:], cookie)
	case "service", "detail":
		id, requiredErr := businessRequired(args[1:], "--id", "service detail 必须提供 --id")
		if requiredErr != nil {
			return nil, requiredErr
		}
		result, listErr := a.libraryServicesList(ctx, []string{"--page-size", "100"}, cookie)
		if listErr != nil {
			return nil, listErr
		}
		for _, item := range libraryServicesData(result) {
			if fmt.Sprint(item["id"]) == strings.TrimSpace(id) {
				return map[string]any{
					"ok": true, "submitted": false, "confirmed": true,
					"evidence": "图书馆服务大厅公开服务接口返回服务详情",
					"service":  libraryServicesService, "operation": "service", "data": item,
				}, nil
			}
		}
		return nil, &siteError{Code: "not_found", Message: "图书馆服务大厅未找到服务: " + strings.TrimSpace(id)}
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "library-services 只支持 list、service、catalog"}
	}
}

func (a NativeSite) libraryServicesList(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	keyword, _, valueErr := businessValue(args, "--keyword")
	if valueErr != nil {
		return nil, valueErr
	}
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	pageSize, pageSizeErr := businessInt(args, "--page-size", 45)
	if pageSizeErr != nil {
		return nil, pageSizeErr
	}

	pageResult, requestErr := a.businessGet(ctx, "library", "/engine2/m/F1E016BEF66C2730", nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	config, configErr := libraryServicesConfigFromPage(businessBody(pageResult))
	if configErr != nil {
		return nil, configErr
	}
	result, requestErr := businessRequest(ctx, "library", "POST", "/engine2/general/"+url.PathEscape(config.appID)+"/type/more-datas", nil, []pair{
		{"sign", config.sign}, {"engineInstanceId", config.engineInstanceID}, {"typeId", config.typeID},
		{"pageNum", strconv.Itoa(page)}, {"pageSize", strconv.Itoa(pageSize)}, {"sw", strings.TrimSpace(keyword)},
	}, nil, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := businessJSONMap(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	if !libraryServicesSuccess(payload["code"]) {
		return nil, &siteError{Code: "business_rejected", Message: "图书馆服务大厅拒绝服务目录查询", Details: map[string]any{"response": redactSiteJSON(payload)}}
	}
	pageData, ok := payload["data"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "图书馆服务大厅响应缺少分页数据"}
	}
	datas, ok := pageData["datas"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "图书馆服务大厅响应缺少服务清单"}
	}
	rawRows, ok := datas["datas"].([]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "图书馆服务大厅服务清单格式无效"}
	}
	items := make([]map[string]any, 0, len(rawRows))
	for _, raw := range rawRows {
		row, ok := raw.(map[string]any)
		if ok {
			items = append(items, libraryServiceRow(row))
		}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "图书馆服务大厅 more-datas 接口返回公开服务清单",
		"service":  libraryServicesService, "operation": "list",
		"page":          libraryServicesInt(datas["pageNum"], page),
		"page_size":     libraryServicesInt(datas["pageSize"], pageSize),
		"total_pages":   libraryServicesInt(datas["totalPage"], 1),
		"total_records": libraryServicesInt(datas["totalRecords"], len(items)),
		"keyword":       strings.TrimSpace(keyword), "data": items,
	}, nil
}

func libraryServicesConfigFromPage(source string) (libraryServicesConfig, *siteError) {
	document, parseErr := parsePage(source)
	if parseErr != nil {
		return libraryServicesConfig{}, &siteError{Code: "parse_error", Message: "图书馆服务大厅页面解析失败: " + parseErr.Error()}
	}
	var script strings.Builder
	for _, node := range document.findAll("script") {
		script.WriteString(node.rawText())
		script.WriteByte('\n')
	}
	source = script.String()
	config := libraryServicesConfig{
		appID:            libraryServicesMatch(libraryServicesAppIDPattern, source),
		engineInstanceID: libraryServicesMatch(libraryServicesEngineInstancePattern, source),
		typeID:           libraryServicesMatch(libraryServicesTypeIDPattern, source),
		sign:             libraryServicesMatch(libraryServicesSignPattern, source),
	}
	if config.appID == "" || config.engineInstanceID == "" || config.typeID == "" || config.sign == "" {
		return libraryServicesConfig{}, &siteError{Code: "parse_error", Message: "图书馆服务大厅页面缺少公开接口配置"}
	}
	return config, nil
}

func libraryServicesMatch(pattern *regexp.Regexp, source string) string {
	match := pattern.FindStringSubmatch(source)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func libraryServicesSuccess(value any) bool {
	switch typed := value.(type) {
	case float64:
		return typed == 1
	case int:
		return typed == 1
	case string:
		return strings.TrimSpace(typed) == "1"
	default:
		return false
	}
}

func libraryServicesInt(value any, fallback int) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func libraryServiceRow(row map[string]any) map[string]any {
	rawURL := libraryServiceString(row["url"])
	return map[string]any{
		"id":           libraryServiceString(row["id"]),
		"name":         libraryServiceFirst(row, "title", "1"),
		"subtitle":     libraryServiceFirst(row, "subtitle", "2"),
		"description":  redactSiteText(libraryServiceFirst(row, "description", "4"), false),
		"icon_url":     safeSiteReference(libraryServiceFirst(row, "icon_url", "0")),
		"page_type":    libraryServicesInt(libraryServiceFirstValue(row, "pageType", "page_type"), 0),
		"url":          safeSiteReference(rawURL),
		"access_count": libraryServiceFirstValue(row, "accessBaseNum"),
		"published_at": libraryServiceFirst(row, "publishTime", "6"),
		"public":       libraryServiceFirstValue(row, "publicUse"),
		"external":     libraryServiceFirstValue(row, "isOuter"),
		"open_target":  libraryServiceFirstValue(row, "openTarget"),
	}
}

func libraryServicesData(result map[string]any) []map[string]any {
	value, ok := result["data"].([]map[string]any)
	if ok {
		return value
	}
	return nil
}

func libraryServiceFirst(row map[string]any, keys ...string) string {
	return libraryServiceString(libraryServiceFirstValue(row, keys...))
}

func libraryServiceFirstValue(row map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := row[key]; ok && value != nil {
			return value
		}
	}
	return ""
}

func libraryServiceString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		return libraryServiceString(typed["value"])
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
