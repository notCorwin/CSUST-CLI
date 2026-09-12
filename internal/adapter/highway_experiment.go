package adapter

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html/charset"
)

const highwayExperimentService = "highway-experiment"

var highwayExperimentCategories = map[string]string{
	"土工": "1", "土工类": "1", "geotechnical": "1",
	"沥青": "2", "沥青类": "2", "asphalt": "2",
	"材料": "3", "材料类": "3", "materials": "3",
	"无机材料": "4", "无机材料类": "4", "inorganic-materials": "4",
	"其他": "5", "other": "5",
}

func (a NativeSite) executeHighwayExperiment(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "status" {
		statusArgs := args
		if len(statusArgs) > 0 {
			statusArgs = statusArgs[1:]
		}
		cookie, _, valueErr := businessValue(statusArgs, "--cookie-file")
		if valueErr != nil {
			return nil, valueErr
		}
		return a.executeServiceStatusWithOptions(ctx, highwayExperimentService, "status", "公路工程实验中心网站", businessRequestOptions{cookieFile: cookie})
	}
	if args[0] == "catalog" {
		result := businessCatalogFilter(highwayExperimentService)
		result["operations"] = []string{"status", "resources", "resource", "booking-info"}
		return result, nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "resources", "list":
		return a.highwayExperimentResources(ctx, args[1:], cookie)
	case "resource", "detail":
		return a.highwayExperimentResource(ctx, args[1:], cookie)
	case "booking-info", "guide":
		return a.highwayExperimentBookingInfo(ctx, cookie)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "highway-experiment 只支持 status、resources、resource、booking-info、catalog"}
	}
}

func highwayExperimentCategory(value string) (string, string, *siteError) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", nil
	}
	if id := highwayExperimentCategories[strings.ToLower(value)]; id != "" {
		return id, value, nil
	}
	return "", "", &siteError{Code: "invalid_argument", Message: "--category 不是已知设备分类", Details: map[string]any{"category": value}}
}

func (a NativeSite) highwayExperimentResources(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	category, _, valueErr := businessValue(args, "--category")
	if valueErr != nil {
		return nil, valueErr
	}
	categoryID, categoryName, categoryErr := highwayExperimentCategory(category)
	if categoryErr != nil {
		return nil, categoryErr
	}
	keyword, _, valueErr := businessValue(args, "--keyword")
	if valueErr != nil {
		return nil, valueErr
	}
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	params := []pair{{"classa", "1"}}
	if categoryID != "" {
		params = append(params, pair{"classb", categoryID})
	}
	if page > 1 {
		params = append(params, pair{"page", strconv.Itoa(page)})
	}
	result, requestErr := a.businessGet(ctx, highwayExperimentService, "/pclass/", params, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	document, parseErr := highwayExperimentDocument(result)
	if parseErr != nil {
		return nil, parseErr
	}
	items := highwayExperimentResourceItems(document, safeResponseURL(result))
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		filtered := items[:0]
		for _, item := range items {
			if strings.Contains(strings.ToLower(item["name"].(string)), strings.ToLower(keyword)) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "公路工程实验中心设备目录页面返回设备卡片",
		"service":  highwayExperimentService, "operation": "resources",
		"category": categoryName, "page": page, "total_pages": highwayExperimentTotalPages(document),
		"keyword": keyword, "data": items, "total": len(items),
	}, nil
}

func (a NativeSite) highwayExperimentResource(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	id, requiredErr := businessRequired(args, "--id", "resource 必须提供 --id")
	if requiredErr != nil {
		return nil, requiredErr
	}
	result, requestErr := a.businessGet(ctx, highwayExperimentService, "/pro/", []pair{{"proid", strings.TrimSpace(id)}}, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	document, parseErr := highwayExperimentDocument(result)
	if parseErr != nil {
		return nil, parseErr
	}
	pageURL := safeResponseURL(result)
	container := libraryRemoteNodeWithClass(document, "show_r")
	if container == nil {
		return nil, &siteError{Code: "parse_error", Message: "设备详情页面缺少内容区域"}
	}
	titleNode := libraryRemoteNodeWithClass(container, "show_rt1")
	title := strings.TrimSpace(pageDisplayText(titleNode))
	main := libraryRemoteNodeWithClass(container, "show_rkc")
	description := strings.TrimSpace(pageDisplayText(libraryRemoteNodeWithClass(container, "show_rk")))
	imageURL := ""
	if main != nil {
		if images := main.findAll("img"); len(images) > 0 {
			imageURL = resolvePageURL(pageURL, images[0].attr("src"))
		}
	}
	related := highwayExperimentResourceItems(container, pageURL)
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "公路工程实验中心设备详情页面返回设备信息",
		"service":  highwayExperimentService, "operation": "resource", "id": strings.TrimSpace(id),
		"data": map[string]any{
			"id": id, "name": title, "description": description, "image_url": imageURL,
			"url": pagePath(pageURL, pageURL), "related": related,
		},
	}, nil
}

func (a NativeSite) highwayExperimentBookingInfo(ctx context.Context, cookie string) (map[string]any, *siteError) {
	result, requestErr := a.businessGet(ctx, highwayExperimentService, "/about/", []pair{{"showid", "56"}}, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	document, parseErr := highwayExperimentDocument(result)
	if parseErr != nil {
		return nil, parseErr
	}
	content := strings.TrimSpace(pageDisplayText(libraryRemoteNodeWithClass(document, "show_r")))
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "公路工程实验中心预约须知页面",
		"service":  highwayExperimentService, "operation": "booking-info",
		"data": map[string]any{"title": "预约须知", "content": content, "url": pagePath(safeResponseURL(result), safeResponseURL(result))},
	}, nil
}

func highwayExperimentDocument(result map[string]any) (*pageNode, *siteError) {
	response, _ := result["response"].(map[string]any)
	contentType, _ := response["content_type"].(string)
	body := businessBody(result)
	reader, readerErr := charset.NewReader(bytes.NewReader([]byte(body)), contentType)
	if readerErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "公路工程实验中心页面编码识别失败: " + readerErr.Error()}
	}
	decoded, readErr := io.ReadAll(reader)
	if readErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "公路工程实验中心页面编码转换失败: " + readErr.Error()}
	}
	document, parseErr := parsePage(string(decoded))
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "公路工程实验中心页面解析失败: " + parseErr.Error()}
	}
	return document, nil
}

func highwayExperimentResourceItems(document *pageNode, pageURL string) []map[string]any {
	items := make([]map[string]any, 0)
	seen := map[string]bool{}
	for _, node := range document.findAll("a") {
		target := resolvePageURL(pageURL, node.attr("href"))
		parsed, err := url.Parse(target)
		if err != nil || !strings.EqualFold(parsed.Path, "/pro/") {
			continue
		}
		id := strings.TrimSpace(parsed.Query().Get("proid"))
		if id == "" || seen[id] {
			continue
		}
		name := strings.TrimSpace(pageDisplayText(node))
		if name == "" {
			name = strings.TrimSpace(node.attr("title"))
		}
		if name == "" {
			continue
		}
		item := map[string]any{"id": id, "name": name, "url": pagePath(pageURL, target)}
		if images := node.findAll("img"); len(images) > 0 {
			item["image_url"] = resolvePageURL(pageURL, images[0].attr("src"))
		}
		items = append(items, item)
		seen[id] = true
	}
	sort.Slice(items, func(i, j int) bool { return items[i]["name"].(string) < items[j]["name"].(string) })
	return items
}

func highwayExperimentTotalPages(document *pageNode) int {
	pages := 1
	for _, node := range document.findAll("a") {
		parsed, err := url.Parse(node.attr("href"))
		if err != nil {
			continue
		}
		value, parseErr := strconv.Atoi(parsed.Query().Get("page"))
		if parseErr == nil && value > pages {
			pages = value
		}
	}
	return pages
}
