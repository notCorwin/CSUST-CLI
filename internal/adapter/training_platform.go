package adapter

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	peixunArticleIDPattern = regexp.MustCompile(`^\d+$`)
	peixunDatePattern      = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)
	peixunPublishedPattern = regexp.MustCompile(`发布日期\s*[:：]\s*(\d{4}-\d{2}-\d{2})`)
)

var peixunCategories = map[string]string{
	"news": "42", "新闻": "42", "新闻资讯": "42",
	"training-news": "47", "培训动态": "47",
	"policy": "48", "政策动态": "48",
	"faculty": "62", "师资队伍": "62",
	"campus": "53", "校园风光": "53",
	"students": "60", "学员风采": "60",
	"transport": "82", "交通运输": "82",
	"topics": "77", "专题": "77",
	"state-owned": "75", "国有企业": "75",
	"private": "74", "民营企业": "74",
	"bank-insurance": "73", "银行保险": "73",
	"field": "46", "现场教学": "46",
	"degree": "45", "同等学力": "45", "同等学力研修及申硕": "45",
}

func (a NativeSite) executeTrainingPlatform(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		result := businessCatalogFilter("training-platform")
		result["operations"] = []string{"home", "list", "detail"}
		return result, nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "home":
		result, requestErr := a.businessGet(ctx, "peixun", "/", nil, businessRequestOptions{cookieFile: cookie})
		if requestErr != nil {
			return nil, requestErr
		}
		result = sitePageResult(result)
		result["service"], result["operation"] = "peixun", "home"
		return result, nil
	case "list":
		return a.peixunList(ctx, args[1:], cookie)
	case "detail":
		return a.peixunDetail(ctx, args[1:], cookie)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "training-platform 只支持 home、list、detail、catalog"}
	}
}

func (a NativeSite) peixunList(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	category, _, valueErr := businessValue(args, "--category")
	if valueErr != nil {
		return nil, valueErr
	}
	category = strings.TrimSpace(category)
	if category == "" {
		category = "news"
	}
	categoryID, ok := peixunCategories[strings.ToLower(category)]
	if !ok {
		return nil, &siteError{Code: "invalid_argument", Message: "--category 不是已知培训资讯分类，可用 news、training-news、policy、faculty、students、transport、state-owned、private、bank-insurance、field 或 degree"}
	}
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	params := []pair{{"cid", categoryID}}
	if page > 1 {
		params = append(params, pair{"page", strconv.Itoa(page)})
	}
	result, requestErr := (NativeSite{}).businessGet(ctx, "peixun", "/article/list", params, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	pageURL := safeResponseURL(result)
	document, parseErr := parsePage(businessBody(result))
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "干部培训公开资讯列表解析失败: " + parseErr.Error()}
	}
	items := peixunListItems(document, pageURL)
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "peixun article/list 公开培训资讯列表及分页", "service": "peixun", "operation": "list",
		"category": category, "category_id": categoryID, "page": page, "page_size": len(items),
		"total_pages": peixunTotalPages(document, pageURL), "data": items,
	}, nil
}

func (a NativeSite) peixunDetail(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	id, requiredErr := businessRequired(args, "--id", "training-platform detail 必须提供 --id")
	if requiredErr != nil {
		return nil, requiredErr
	}
	id = strings.TrimSpace(id)
	if !peixunArticleIDPattern.MatchString(id) {
		return nil, &siteError{Code: "invalid_argument", Message: "training-platform detail --id 必须是正整数"}
	}
	result, requestErr := (NativeSite{}).businessGet(ctx, "peixun", "/article/detail", []pair{{"id", id}}, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	pageURL := safeResponseURL(result)
	document, parseErr := parsePage(businessBody(result))
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "干部培训公开资讯详情解析失败: " + parseErr.Error()}
	}
	title := strings.TrimSpace(pageDisplayText(document.first("title", "")))
	contentNode := peixunNodeWithClass(document, "padding-top-20")
	if contentNode == nil {
		contentNode = libraryRemoteNodeWithClass(document, "info-content")
	}
	if contentNode == nil || strings.TrimSpace(pageDisplayText(contentNode)) == "" {
		return nil, &siteError{Code: "parse_error", Message: "干部培训公开资讯详情缺少正文"}
	}
	metadata := pageDisplayText(libraryRemoteNodeWithClass(document, "info-content"))
	publishedAt := ""
	if matches := peixunPublishedPattern.FindStringSubmatch(metadata); len(matches) > 1 {
		publishedAt = matches[1]
	}
	links := make([]map[string]any, 0)
	for _, link := range contentNode.findAll("a") {
		if strings.TrimSpace(link.attr("href")) != "" {
			links = append(links, pageLink(link, pageURL))
		}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "peixun article/detail 公开培训资讯详情页面", "service": "peixun", "operation": "detail",
		"id": id, "title": title, "published_at": publishedAt,
		"content": strings.TrimSpace(pageDisplayText(contentNode)), "links": links,
		"url": safeSiteURL(mustParseURL(pageURL)),
	}, nil
}

func peixunListItems(document *pageNode, pageURL string) []map[string]any {
	list := libraryRemoteNodeWithClass(document, "m-newlist")
	if list == nil {
		list = document
	}
	items := make([]map[string]any, 0)
	seen := make(map[string]bool)
	for _, link := range list.findAll("a") {
		targetValue := resolvePageURL(pageURL, link.attr("href"))
		target, err := url.Parse(targetValue)
		if err != nil || target == nil || !officialSameOrigin(pageURL, targetValue) {
			continue
		}
		id := target.Query().Get("id")
		if !peixunArticleIDPattern.MatchString(id) || seen[id] {
			continue
		}
		seen[id] = true
		container := link
		for current := link; current != nil; current = current.parent {
			if current.tag == "li" {
				container = current
				break
			}
		}
		publishedAt := ""
		if matches := peixunDatePattern.FindString(pageDisplayText(container)); matches != "" {
			publishedAt = matches
		}
		items = append(items, map[string]any{
			"id": id, "title": strings.TrimSpace(pageDisplayText(link)), "published_at": publishedAt,
			"url": safeSiteURL(target),
		})
	}
	return items
}

func peixunTotalPages(document *pageNode, pageURL string) int {
	total := 1
	for _, link := range document.findAll("a") {
		targetValue := resolvePageURL(pageURL, link.attr("href"))
		if !officialSameOrigin(pageURL, targetValue) {
			continue
		}
		target, err := url.Parse(targetValue)
		if err != nil || target.Path != "/article/list" {
			continue
		}
		if value, err := strconv.Atoi(target.Query().Get("page")); err == nil && value > total {
			total = value
		}
	}
	return total
}

func peixunNodeWithClass(root *pageNode, className string) *pageNode {
	for _, candidate := range root.findAll("") {
		for _, class := range strings.Fields(candidate.attr("class")) {
			if class == className {
				return candidate
			}
		}
	}
	return nil
}
