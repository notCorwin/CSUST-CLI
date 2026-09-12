package adapter

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	officialArticlePathPattern = regexp.MustCompile(`/info/(\d+/\d+)\.htm$`)
	officialSearchTotalPattern = regexp.MustCompile(`(?:共有|共找到|共|找到)\s*(\d+)\s*条`)
	officialSearchPagesPattern = regexp.MustCompile(`(?:共有|总共|共)\s*(\d+)\s*页`)
)

func (a NativeSite) executeOfficial(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		result := businessCatalogFilter("official")
		result["operations"] = []string{"home", "search", "article"}
		return result, nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "home":
		result, requestErr := a.businessGet(ctx, "official", "/", nil, businessRequestOptions{cookieFile: cookie})
		if requestErr != nil {
			return nil, requestErr
		}
		result = sitePageResult(result)
		result["service"], result["operation"] = "official", "home"
		return result, nil
	case "search":
		return a.officialSearch(ctx, args[1:], cookie)
	case "article":
		return a.officialArticle(ctx, args[1:], cookie)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "official 只支持 home、search、article、catalog"}
	}
}

func (a NativeSite) officialSearch(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	keyword, requiredErr := businessRequired(args, "--keyword", "official search 必须提供 --keyword")
	if requiredErr != nil {
		return nil, requiredErr
	}
	keyword = strings.TrimSpace(keyword)
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	options := businessRequestOptions{cookieFile: cookie}
	initial, requestErr := a.businessGet(ctx, "official", "/", nil, options)
	if requestErr != nil {
		return nil, requestErr
	}
	initialURL := safeResponseURL(initial)
	document, parseErr := parsePage(businessBody(initial))
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "学校官网首页解析失败: " + parseErr.Error()}
	}
	form := officialSearchForm(document)
	if form == nil {
		return nil, &siteError{Code: "parse_error", Message: "学校官网首页缺少全文检索表单"}
	}
	action := resolvePageURL(initialURL, form.attr("action"))
	if action == "" {
		action = resolvePageURL(initialURL, "/sch.jsp?wbtreeid=1020")
	}
	if !officialSameOrigin(initialURL, action) {
		return nil, &siteError{Code: "parse_error", Message: "学校官网全文检索表单地址不是官网同源地址"}
	}
	path, pathErr := officialRequestPath(action)
	if pathErr != nil {
		return nil, pathErr
	}
	fields := pageFormFields(form, nil, document)
	fields = setFormField(fields, "lucenenewssearchkey", keyword)
	fields = setFormField(fields, "showkeycode", keyword)
	result, requestErr := businessRequest(ctx, "official", "POST", path, nil, fields, []pair{{"Referer", initialURL}}, options, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	firstPageDocument, firstPageParseErr := parsePage(businessBody(result))
	if firstPageParseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "学校官网全文检索结果解析失败: " + firstPageParseErr.Error()}
	}
	firstPageTotal := officialSearchNumber(firstPageDocument, officialSearchTotalPattern)
	firstPagePages := officialSearchNumber(firstPageDocument, officialSearchPagesPattern)
	if page > 1 {
		result, requestErr = a.officialSearchPage(ctx, result, page, options)
		if requestErr != nil {
			return nil, requestErr
		}
	}
	pageURL := safeResponseURL(result)
	resultDocument, parseErr := parsePage(businessBody(result))
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "学校官网全文检索结果解析失败: " + parseErr.Error()}
	}
	items := officialSearchItems(resultDocument, pageURL)
	total := officialSearchNumber(resultDocument, officialSearchTotalPattern)
	if total == 0 {
		total = firstPageTotal
	}
	totalPages := officialSearchNumber(resultDocument, officialSearchPagesPattern)
	if totalPages == 0 {
		totalPages = firstPagePages
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "学校官网 sch.jsp 校内全文检索及文章链接", "service": "official", "operation": "search",
		"keyword": keyword, "page": page, "page_size": len(items),
		"total": total, "total_pages": totalPages, "data": items,
	}, nil
}

func (a NativeSite) officialSearchPage(ctx context.Context, result map[string]any, page int, options businessRequestOptions) (map[string]any, *siteError) {
	pageURL := safeResponseURL(result)
	document, parseErr := parsePage(businessBody(result))
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "学校官网分页结果解析失败: " + parseErr.Error()}
	}
	target := officialSearchPageTarget(document, pageURL, page)
	if target == nil {
		return nil, &siteError{Code: "parse_error", Message: "学校官网全文检索结果缺少第 " + strconv.Itoa(page) + " 页链接"}
	}
	params := make([]pair, 0, 4)
	for _, name := range []string{"wbtreeid", "searchScope", "currentnum", "newskeycode2"} {
		if value := target.Query().Get(name); value != "" {
			params = append(params, pair{name, value})
		}
	}
	if target.Query().Get("currentnum") == "" {
		params = append(params, pair{"currentnum", strconv.Itoa(page)})
	}
	return (NativeSite{}).businessGet(ctx, "official", target.EscapedPath(), params, options)
}

func (a NativeSite) officialArticle(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	id, requiredErr := businessRequired(args, "--id", "official article 必须提供 --id（格式为频道号/文章号）")
	if requiredErr != nil {
		return nil, requiredErr
	}
	id = strings.TrimSpace(id)
	if !officialArticlePathPattern.MatchString("/info/" + id + ".htm") {
		return nil, &siteError{Code: "invalid_argument", Message: "official article --id 必须是频道号/文章号，例如 1299/22951"}
	}
	result, requestErr := (NativeSite{}).businessGet(ctx, "official", "/info/"+id+".htm", nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	pageURL := safeResponseURL(result)
	document, parseErr := parsePage(businessBody(result))
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "学校官网文章页面解析失败: " + parseErr.Error()}
	}
	titleContainer := libraryRemoteNodeWithClass(document, "show_title")
	title := ""
	if titleContainer != nil {
		title = strings.TrimSpace(pageDisplayText(titleContainer.first("h3", "")))
	}
	if title == "" {
		title = strings.TrimSpace(pageDisplayText(document.first("h1", "")))
	}
	contentNode := libraryRemoteNodeWithClass(document, "v_news_content")
	if contentNode == nil {
		contentNode = libraryRemoteNodeWithClass(document, "tot_content")
	}
	if contentNode == nil {
		return nil, &siteError{Code: "parse_error", Message: "学校官网文章页面缺少正文"}
	}
	metadata := ""
	if titleContainer != nil {
		metadata = pageDisplayText(titleContainer)
	}
	publishedAt, source := officialArticleMetadata(metadata)
	links := make([]map[string]any, 0)
	for _, link := range contentNode.findAll("a") {
		if strings.TrimSpace(link.attr("href")) != "" {
			links = append(links, pageLink(link, pageURL))
		}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "学校官网 info 文章详情页面", "service": "official", "operation": "article",
		"id": id, "title": title, "published_at": publishedAt, "source": source,
		"content": strings.TrimSpace(pageDisplayText(contentNode)), "links": links,
		"url": safeSiteURL(mustParseURL(pageURL)),
	}, nil
}

func officialSearchForm(document *pageNode) *pageNode {
	for _, form := range document.findAll("form") {
		action := strings.ToLower(form.attr("action"))
		for _, input := range form.findAll("input") {
			name := strings.ToLower(input.attr("name"))
			if name == "lucenenewssearchkey" || name == "showkeycode" || strings.Contains(action, "sch.jsp") {
				return form
			}
		}
	}
	return nil
}

func officialSearchPageTarget(document *pageNode, pageURL string, page int) *url.URL {
	wanted := strconv.Itoa(page)
	var fallback *url.URL
	for _, link := range document.findAll("a") {
		targetValue := resolvePageURL(pageURL, link.attr("href"))
		if !officialSameOrigin(pageURL, targetValue) {
			continue
		}
		target, err := url.Parse(targetValue)
		if err != nil || target.Query().Get("newskeycode2") == "" || target.Query().Get("currentnum") == "" {
			continue
		}
		if target.Query().Get("currentnum") == wanted {
			return target
		}
		if fallback == nil {
			fallback = target
		}
	}
	if fallback == nil {
		return nil
	}
	query := fallback.Query()
	query.Set("currentnum", wanted)
	fallback.RawQuery = query.Encode()
	return fallback
}

func officialSearchItems(document *pageNode, pageURL string) []map[string]any {
	items := make([]map[string]any, 0)
	seen := make(map[string]bool)
	for _, table := range document.findAll("table") {
		rows := table.findAll("tr")
		for index, row := range rows {
			var link *pageNode
			var target *url.URL
			for _, candidate := range row.findAll("a") {
				value := resolvePageURL(pageURL, candidate.attr("href"))
				if !officialSameOrigin(pageURL, value) {
					continue
				}
				parsed, err := url.Parse(value)
				if err == nil && officialArticleID(parsed) != "" {
					link, target = candidate, parsed
					break
				}
			}
			if link == nil || target == nil {
				continue
			}
			id := officialArticleID(target)
			if seen[id] {
				continue
			}
			seen[id] = true
			summary, publishedAt := "", ""
			for next := index + 1; next < len(rows) && next <= index+4; next++ {
				text := strings.TrimSpace(pageDisplayText(rows[next]))
				if text == "" {
					continue
				}
				if strings.Contains(text, "发表时间") {
					publishedAt = strings.TrimSpace(strings.TrimLeft(strings.TrimPrefix(text, "发表时间"), " :："))
					break
				}
				if summary == "" {
					summary = text
				}
			}
			items = append(items, map[string]any{
				"id": id, "title": strings.TrimSpace(pageDisplayText(link)), "summary": summary,
				"published_at": publishedAt, "url": safeSiteURL(target),
			})
		}
	}
	return items
}

func officialArticleID(target *url.URL) string {
	if target == nil {
		return ""
	}
	matches := officialArticlePathPattern.FindStringSubmatch(target.Path)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

func officialSearchNumber(document *pageNode, pattern *regexp.Regexp) int {
	matches := pattern.FindStringSubmatch(pageDisplayText(document))
	if len(matches) < 2 {
		return 0
	}
	number, _ := strconv.Atoi(matches[1])
	return number
}

func officialArticleMetadata(value string) (string, string) {
	value = strings.TrimSpace(value)
	index := strings.Index(value, "发布日期")
	if index < 0 {
		return "", ""
	}
	value = strings.TrimLeft(value[index+len("发布日期"):], " :：")
	source := ""
	if sourceIndex := strings.Index(value, "来源"); sourceIndex >= 0 {
		source = strings.TrimLeft(value[sourceIndex+len("来源"):], " :：")
		value = strings.TrimSpace(value[:sourceIndex])
	}
	return strings.TrimSpace(value), strings.TrimSpace(source)
}

func officialSameOrigin(baseValue, targetValue string) bool {
	base, baseErr := url.Parse(baseValue)
	target, targetErr := url.Parse(targetValue)
	return baseErr == nil && targetErr == nil && base.Host != "" && target.Host != "" && strings.EqualFold(base.Host, target.Host) && strings.EqualFold(base.Scheme, target.Scheme)
}

func officialRequestPath(value string) (string, *siteError) {
	target, err := url.Parse(value)
	if err != nil || target.Path == "" {
		return "", &siteError{Code: "parse_error", Message: "学校官网全文检索表单地址无效"}
	}
	path := target.EscapedPath()
	if target.RawQuery != "" {
		path += "?" + target.RawQuery
	}
	return path, nil
}
