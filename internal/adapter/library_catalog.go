package adapter

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const libraryCatalogService = "library-catalog"

var (
	libraryCatalogTotalPattern       = regexp.MustCompile(`检索到:\s*([0-9,]+)\s*条结果`)
	libraryCatalogPagesPattern       = regexp.MustCompile(`共\s*([0-9,]+)\s*页`)
	libraryCatalogPublicationPattern = regexp.MustCompile(`出版日期:\s*([0-9]{4}(?:[-/]\d{1,2}(?:[-/]\d{1,2})?)?)`)
	libraryCatalogLoanPattern        = regexp.MustCompile(`已借\s*([0-9]+)次`)
	libraryCatalogDocumentPattern    = regexp.MustCompile(`文献类型:\s*([^,\s]+)`)
	libraryCatalogISBNPattern        = regexp.MustCompile(`(?i)\b(?:97[89][\d-]{10,}|[\dX]{10,})\b`)
	libraryCatalogPricePattern       = regexp.MustCompile(`价格[：:]\s*([^\s]+)`)
)

func (a NativeSite) executeLibraryCatalog(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return businessCatalogFilter(libraryCatalogService), nil
	}
	cookie, _, valueErr := businessValue(args, "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "status":
		result, statusErr := a.libraryCatalogProfile(ctx, nil, cookie)
		if result != nil {
			result["operation"], result["logged_in"] = "status", true
		}
		return result, statusErr
	case "login":
		return a.libraryCatalogLogin(ctx, args[1:], cookie)
	case "logout":
		return a.libraryCatalogLogout(cookie)
	case "search", "list":
		return a.libraryCatalogSearch(ctx, args[1:], cookie)
	case "book", "detail":
		return a.libraryCatalogBook(ctx, args[1:], cookie)
	case "holdings", "holding":
		return a.libraryCatalogHoldings(ctx, args[1:], cookie)
	case "profile", "reader-profile":
		return a.libraryCatalogProfile(ctx, args[1:], cookie)
	case "loans", "current-loans":
		return a.libraryCatalogReaderPage(ctx, args[1:], cookie, "loans", "/loan/currentLoanList")
	case "special-loans":
		return a.libraryCatalogReaderPage(ctx, args[1:], cookie, "special-loans", "/loan/currentSpecLoanList")
	case "loan-history", "history-loans":
		return a.libraryCatalogReaderPage(ctx, args[1:], cookie, "loan-history", "/loan/historyLoanList")
	case "reservations", "current-reservations":
		return a.libraryCatalogReaderPage(ctx, args[1:], cookie, "reservations", "/reservation/currentReservationList")
	case "reservation-history":
		return a.libraryCatalogReaderPage(ctx, args[1:], cookie, "reservation-history", "/reservation/historyReservationList")
	case "privileges", "reader-privileges":
		return a.libraryCatalogPrivileges(ctx, args[1:], cookie)
	case "finance", "fees":
		return a.libraryCatalogReaderPage(ctx, args[1:], cookie, "finance", "/finance/financeList")
	case "shelf", "book-shelf":
		return a.libraryCatalogReaderPage(ctx, args[1:], cookie, "shelf", "/privateCollection/privateCollectionList")
	case "preloans", "pre-loans":
		return a.libraryCatalogReaderPage(ctx, args[1:], cookie, "preloans", "/prelend/currentPrelendList")
	case "tags":
		return a.libraryCatalogReaderPage(ctx, args[1:], cookie, "tags", "/tag/privateTagList")
	case "booklists", "book-lists":
		return a.libraryCatalogReaderPage(ctx, args[1:], cookie, "booklists", "/booklist/list")
	case "loan-rule", "rule":
		return a.libraryCatalogLoanRule(ctx, args[1:], cookie)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "library 只支持 login、logout、status、search、book、holdings、profile、loans、special-loans、loan-history、reservations、reservation-history、privileges、finance、shelf、preloans、tags、booklists、loan-rule、catalog"}
	}
}

func libraryCatalogReaderTarget(target *url.URL) *url.URL {
	copy := *target
	copy.Scheme, copy.Path, copy.RawQuery, copy.Fragment = "http", "/special/toOpac", "", ""
	return &copy
}

func (a NativeSite) libraryCatalogLogin(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	loginArgs := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		if args[index] == "--cookie-file" {
			index++
			continue
		}
		loginArgs = append(loginArgs, args[index])
	}
	options, parseErr := parseLoginOptions(loginArgs)
	if parseErr != nil {
		return nil, parseErr
	}
	if options.auth == "local" {
		return nil, &siteError{Code: "invalid_argument", Message: "library-catalog 只支持 --auth sso"}
	}
	target, cookiePath, resolveErr := resolveSite(siteRequest{Service: libraryCatalogService, CookieFile: cookie})
	if resolveErr != nil {
		return nil, resolveErr
	}
	return a.loginSSOService(ctx, libraryCatalogReaderTarget(target), cookiePath, options)
}

func (a NativeSite) libraryCatalogLogout(cookie string) (map[string]any, *siteError) {
	target, cookiePath, resolveErr := resolveSite(siteRequest{Service: libraryCatalogService, CookieFile: cookie})
	if resolveErr != nil {
		return nil, resolveErr
	}
	if removeErr := removeCookieFile(cookiePath); removeErr != nil {
		return nil, &siteError{Code: "cookie_write_failed", Message: removeErr.Error()}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "local-cookie-removed",
		"service": libraryCatalogService, "operation": "logout", "logged_out": true,
		"cookie_file": cookiePath, "target": safeSiteURL(libraryCatalogReaderTarget(target)),
	}, nil
}

func (a NativeSite) libraryCatalogProfile(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	result, requestErr := a.businessGet(ctx, libraryCatalogService, "/reader/getReaderInfo", []pair{{"return_fmt", "json"}}, businessRequestOptions{cookieFile: cookie, require: true})
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := businessJSONMap(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	reader, ok := payload["reader"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "图书馆读者资料响应缺少 reader"}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "OPAC /reader/getReaderInfo?return_fmt=json 返回",
		"service":  libraryCatalogService, "operation": "profile", "data": redactSiteJSON(reader),
		"reader": redactSiteJSON(reader), "raw": redactSiteJSON(payload),
	}, nil
}

func (a NativeSite) libraryCatalogReaderPage(ctx context.Context, args []string, cookie, operation, path string) (map[string]any, *siteError) {
	params, page, pageSize, paramsErr := libraryCatalogReaderParams(args, operation)
	if paramsErr != nil {
		return nil, paramsErr
	}
	result, requestErr := a.businessGet(ctx, libraryCatalogService, path, params, businessRequestOptions{cookieFile: cookie, require: true})
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := businessJSONMap(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	paginator, _ := payload["paginator"].(map[string]any)
	data, _ := paginator["currentPages"].([]any)
	if data == nil {
		data = []any{}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "OPAC " + path + "?return_fmt=json 返回",
		"service":  libraryCatalogService, "operation": operation,
		"page": page, "page_size": pageSize, "data": redactSiteJSON(data),
		"total": paginator["totalRows"], "pagination": redactSiteJSON(paginator),
		"raw": redactSiteJSON(payload),
	}, nil
}

func libraryCatalogReaderParams(args []string, operation string) ([]pair, int, int, *siteError) {
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, 0, 0, pageErr
	}
	pageSize, pageSizeErr := businessInt(args, "--page-size", 10)
	if pageSizeErr != nil {
		return nil, 0, 0, pageSizeErr
	}
	params := []pair{{"return_fmt", "json"}, {"page", strconv.Itoa(page)}, {"rows", strconv.Itoa(pageSize)}}
	if operation == "loans" || operation == "special-loans" || operation == "loan-history" || operation == "reservations" || operation == "reservation-history" || operation == "privileges" {
		query, found, valueErr := businessValue(args, "--query")
		if valueErr != nil {
			return nil, 0, 0, valueErr
		}
		if !found {
			query, _, valueErr = businessValue(args, "--keyword")
			if valueErr != nil {
				return nil, 0, 0, valueErr
			}
		}
		field, _, valueErr := businessValue(args, "--field")
		if valueErr != nil {
			return nil, 0, 0, valueErr
		}
		field, fieldErr := libraryCatalogReaderField(field)
		if fieldErr != nil {
			return nil, 0, 0, fieldErr
		}
		params = append(params, pair{"searchType", field}, pair{"searchValue", strings.TrimSpace(query)})
	}
	if operation == "loan-history" {
		from, _, valueErr := businessValue(args, "--from")
		if valueErr != nil {
			return nil, 0, 0, valueErr
		}
		to, _, valueErr := businessValue(args, "--to")
		if valueErr != nil {
			return nil, 0, 0, valueErr
		}
		logType, _, valueErr := businessValue(args, "--operation")
		if valueErr != nil {
			return nil, 0, 0, valueErr
		}
		logType, logTypeErr := libraryCatalogLogType(logType)
		if logTypeErr != nil {
			return nil, 0, 0, logTypeErr
		}
		params = append(params, pair{"starttime", strings.TrimSpace(from)}, pair{"endtime", strings.TrimSpace(to)}, pair{"logType", logType})
	}
	return params, page, pageSize, nil
}

func libraryCatalogReaderField(value string) (string, *siteError) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "any", "all", "任意词":
		return "", nil
	case "title", "题名":
		return "title", nil
	case "barcode", "条码号":
		return "barcode", nil
	case "author", "作者":
		return "author", nil
	case "call-number", "callno", "索书号":
		return "callNo", nil
	case "isbn":
		return "isbn", nil
	case "location", "localcode", "馆藏地点":
		return "localcode", nil
	case "circulation-type", "cirtype", "图书流通类型":
		return "cirtype", nil
	default:
		return "", &siteError{Code: "invalid_argument", Message: "--field 只能是 any、title、barcode、author、call-number、isbn、location 或 circulation-type"}
	}
}

func libraryCatalogLogType(value string) (string, *siteError) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "all", "全部":
		return "", nil
	case "borrow", "loan", "借阅", "30001":
		return "30001", nil
	case "return", "归还", "30002":
		return "30002", nil
	default:
		return "", &siteError{Code: "invalid_argument", Message: "--operation 只能是 all、borrow 或 return"}
	}
}

func (a NativeSite) libraryCatalogPrivileges(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	params, _, _, paramsErr := libraryCatalogReaderParams(args, "privileges")
	if paramsErr != nil {
		return nil, paramsErr
	}
	result, requestErr := a.businessGet(ctx, libraryCatalogService, "/reader/readerPrivilegeList", params, businessRequestOptions{cookieFile: cookie, require: true})
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := businessJSONMap(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	data := payload["readerPrivilegeList"]
	if data == nil {
		data = []any{}
	}
	paginator, _ := payload["paginator"].(map[string]any)
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "OPAC /reader/readerPrivilegeList?return_fmt=json 返回",
		"service":  libraryCatalogService, "operation": "privileges",
		"data": redactSiteJSON(data), "policy": redactSiteJSON(payload["prcType"]),
		"total": paginator["totalRows"], "pagination": redactSiteJSON(paginator), "raw": redactSiteJSON(payload),
	}, nil
}

func (a NativeSite) libraryCatalogLoanRule(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	id, requiredErr := businessRequired(args, "--id", "loan-rule 必须提供 --id")
	if requiredErr != nil {
		return nil, requiredErr
	}
	id = strings.TrimSpace(id)
	result, requestErr := a.businessGet(ctx, libraryCatalogService, "/reader/getLoanRule/"+url.PathEscape(id), []pair{{"ruleNo", id}, {"return_fmt", "json"}}, businessRequestOptions{cookieFile: cookie, require: true})
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := businessJSONMap(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	rule, ok := payload["loanRule"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "图书馆借阅规则响应缺少 loanRule"}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "OPAC /reader/getLoanRule?return_fmt=json 返回",
		"service":  libraryCatalogService, "operation": "loan-rule", "id": id,
		"data": redactSiteJSON(rule), "rule": redactSiteJSON(rule), "raw": redactSiteJSON(payload),
	}, nil
}

func (a NativeSite) libraryCatalogSearch(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	query, found, valueErr := businessValue(args, "--query")
	if valueErr != nil {
		return nil, valueErr
	}
	if !found {
		query, _, valueErr = businessValue(args, "--keyword")
		if valueErr != nil {
			return nil, valueErr
		}
	}
	field, _, valueErr := businessValue(args, "--field")
	if valueErr != nil {
		return nil, valueErr
	}
	searchWay, fieldName, fieldErr := libraryCatalogField(field)
	if fieldErr != nil {
		return nil, fieldErr
	}
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	pageSize, pageSizeErr := businessInt(args, "--page-size", 10)
	if pageSizeErr != nil {
		return nil, pageSizeErr
	}
	sortWay, sortName, sortErr := libraryCatalogSort(flagValue(args, "--sort"))
	if sortErr != nil {
		return nil, sortErr
	}
	sortOrder, orderName, orderErr := libraryCatalogOrder(flagValue(args, "--order"))
	if orderErr != nil {
		return nil, orderErr
	}

	params := []pair{
		{"q", strings.TrimSpace(query)}, {"searchType", "standard"}, {"isFacet", "true"}, {"view", "standard"},
		{"searchWay", searchWay}, {"rows", strconv.Itoa(pageSize)}, {"sortWay", sortWay}, {"sortOrder", sortOrder},
		{"scWay", "dim"}, {"searchWay0", "marc"}, {"logical0", "AND"}, {"searchSource", "reader"}, {"page", strconv.Itoa(page)},
	}
	if businessBool(args, "--in-library") {
		params = append(params, pair{"hasholding", "1"})
	}
	result, requestErr := a.businessGet(ctx, libraryCatalogService, "/search", params, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	body, bodyErr := libraryRemoteBody(result)
	if bodyErr != nil {
		return nil, bodyErr
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "图书馆馆藏检索结果解析失败: " + parseErr.Error()}
	}
	items := libraryCatalogSearchItems(document)
	total := libraryCatalogSearchNumber(document, libraryCatalogTotalPattern)
	if total < 0 {
		total = len(items)
	}
	totalPages := libraryCatalogSearchNumber(document, libraryCatalogPagesPattern)
	if totalPages < 0 && total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	if totalPages < 0 {
		totalPages = 0
	}
	if len(items) > 0 {
		preview, previewErr := a.businessGet(ctx, libraryCatalogService, "/book/holdingPreviews", []pair{
			{"bookrecnos", libraryCatalogItemIDs(items)}, {"curLibcodes", ""}, {"return_fmt", "json"}, {"isCluster", "false"},
		}, businessRequestOptions{cookieFile: cookie})
		if previewErr != nil {
			return nil, previewErr
		}
		previewPayload, payloadErr := businessJSONMap(preview)
		if payloadErr != nil {
			return nil, payloadErr
		}
		libraryCatalogApplyPreviews(items, previewPayload)
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "OPAC /search 检索页面及馆藏预览接口返回",
		"service":  libraryCatalogService, "operation": "search",
		"query": strings.TrimSpace(query), "field": fieldName, "sort": sortName, "order": orderName,
		"in_library": businessBool(args, "--in-library"), "page": page, "page_size": pageSize,
		"total": total, "total_pages": totalPages, "data": items,
	}, nil
}

func libraryCatalogField(value string) (string, string, *siteError) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "any", "all", "keyword", "任意词":
		return "", "any", nil
	case "title", "题名":
		return "title", "title", nil
	case "main-title", "正题名", "title200a":
		return "title200a", "main-title", nil
	case "isbn", "isbn/issn":
		return "isbn", "isbn", nil
	case "author", "著者":
		return "author", "author", nil
	case "subject", "主题词":
		return "subject", "subject", nil
	case "class", "classification", "分类号":
		return "class", "classification", nil
	case "control", "ctrlno", "控制号":
		return "ctrlno", "control", nil
	case "order", "orderno", "订购号":
		return "orderno", "order", nil
	case "publisher", "出版社":
		return "publisher", "publisher", nil
	case "call-number", "callno", "索书号":
		return "callno", "call-number", nil
	default:
		return "", "", &siteError{Code: "invalid_argument", Message: "--field 只能是 any、title、main-title、isbn、author、subject、classification、control、order、publisher 或 call-number"}
	}
}

func libraryCatalogSort(value string) (string, string, *siteError) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "relevance", "score", "匹配度":
		return "score", "relevance", nil
	case "publication-date", "pubdate", "date", "出版日期":
		return "pubdate_sort", "publication-date", nil
	case "subject", "主题词":
		return "subject_sort", "subject", nil
	case "title", "题名":
		return "title_sort", "title", nil
	case "author", "著者":
		return "author_sort", "author", nil
	case "call-number", "callno", "索书号":
		return "callno_sort", "call-number", nil
	case "title-pinyin", "pinyin", "题名拼音":
		return "pinyin_sort", "title-pinyin", nil
	case "loan-count", "borrow-count", "loannum", "借阅次数":
		return "loannum_sort", "loan-count", nil
	case "renew-count", "renewnum", "续借次数":
		return "renewnum_sort", "renew-count", nil
	case "title-weight", "题名权重":
		return "title200Weight", "title-weight", nil
	case "main-title-weight", "正题名权重":
		return "title200aWeight", "main-title-weight", nil
	case "volume", "卷册号":
		return "title200h", "volume", nil
	default:
		return "", "", &siteError{Code: "invalid_argument", Message: "--sort 不支持该排序方式"}
	}
}

func libraryCatalogOrder(value string) (string, string, *siteError) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "desc", "descending", "降序":
		return "desc", "desc", nil
	case "asc", "ascending", "升序":
		return "asc", "asc", nil
	default:
		return "", "", &siteError{Code: "invalid_argument", Message: "--order 只能是 asc 或 desc"}
	}
}

func libraryCatalogSearchItems(document *pageNode) []map[string]any {
	items := make([]map[string]any, 0)
	for _, meta := range document.findAll("div") {
		if libraryRemoteNodeWithClass(meta, "bookmeta") != meta || strings.TrimSpace(meta.attr("bookrecno")) == "" {
			continue
		}
		id := strings.TrimSpace(meta.attr("bookrecno"))
		text := pageDisplayText(meta)
		title := ""
		if node := libraryRemoteNodeWithClass(meta, "title-link"); node != nil {
			title = strings.TrimSpace(pageDisplayText(node))
		}
		authors := []string{}
		if node := libraryRemoteNodeWithClass(meta, "author-link"); node != nil {
			if value := strings.TrimSpace(pageDisplayText(node)); value != "" {
				authors = append(authors, value)
			}
		}
		item := map[string]any{
			"id": id, "title": title, "authors": authors, "detail_path": "/book/" + url.PathEscape(id),
			"book_type": strings.TrimSpace(meta.attr("booktype")),
		}
		if node := libraryRemoteNodeWithClass(meta, "publisher-link"); node != nil {
			item["publisher"] = strings.TrimSpace(pageDisplayText(node))
		}
		if node := libraryRemoteNodeWithClass(meta, "callnosSpan"); node != nil {
			item["call_number"] = strings.TrimSpace(pageDisplayText(node))
		}
		if match := libraryCatalogPublicationPattern.FindStringSubmatch(text); len(match) > 1 {
			item["publication_year"] = match[1]
		}
		if match := libraryCatalogLoanPattern.FindStringSubmatch(text); len(match) > 1 {
			item["loan_count"] = libraryCatalogParseInt(match[1])
		}
		if match := libraryCatalogDocumentPattern.FindStringSubmatch(text); len(match) > 1 {
			item["document_type"] = match[1]
		}
		items = append(items, item)
	}
	return items
}

func libraryCatalogItemIDs(items []map[string]any) string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if id, ok := item["id"].(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return strings.Join(ids, ",")
}

func libraryCatalogApplyPreviews(items []map[string]any, payload map[string]any) {
	previews, _ := payload["previews"].(map[string]any)
	for _, item := range items {
		id, _ := item["id"].(string)
		rows := libraryCatalogPreviewRows(previews[id])
		copyCount, availableCount := 0, 0
		for _, row := range rows {
			copyCount += libraryCatalogInt(row["copy_count"])
			availableCount += libraryCatalogInt(row["available_count"])
		}
		item["holdings"] = rows
		item["copy_count"] = copyCount
		item["available_count"] = availableCount
	}
}

func libraryCatalogPreviewRows(value any) []map[string]any {
	rawRows, _ := value.([]any)
	rows := make([]map[string]any, 0, len(rawRows))
	for _, raw := range rawRows {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		rows = append(rows, map[string]any{
			"book_id": row["bookrecno"], "call_number": row["callno"], "library_code": row["curlib"],
			"library": row["curlibName"], "location_code": row["curlocal"], "location": row["curlocalName"],
			"copy_count": row["copycount"], "available_count": row["loanableCount"], "shelf": row["shelfno"],
			"barcode": row["barcode"], "state": row["state"], "state_name": row["stateName"],
			"circulation_type": row["pbctypeName"],
		})
	}
	return rows
}

func libraryCatalogSearchNumber(document *pageNode, pattern *regexp.Regexp) int {
	text := pageDisplayText(document)
	if node := document.first("div", "search_meta"); node != nil {
		text = pageDisplayText(node)
	}
	if pattern == libraryCatalogPagesPattern {
		if node := libraryRemoteNodeWithClass(document, "meneame"); node != nil {
			text = pageDisplayText(node)
		}
	}
	match := pattern.FindStringSubmatch(text)
	if len(match) < 2 {
		return -1
	}
	return libraryCatalogParseInt(match[1])
}

func (a NativeSite) libraryCatalogBook(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	id, requiredErr := businessRequired(args, "--id", "book detail 必须提供 --id")
	if requiredErr != nil {
		return nil, requiredErr
	}
	id = strings.TrimSpace(id)
	result, requestErr := a.businessGet(ctx, libraryCatalogService, "/book/"+url.PathEscape(id), nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	body, bodyErr := libraryRemoteBody(result)
	if bodyErr != nil {
		return nil, bodyErr
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "图书馆书目详情解析失败: " + parseErr.Error()}
	}
	book, bookErr := libraryCatalogBookFromPage(document, id)
	if bookErr != nil {
		return nil, bookErr
	}
	holdingData, holdingErr := a.libraryCatalogHoldingsData(ctx, id, cookie)
	if holdingErr != nil {
		return nil, holdingErr
	}
	book["holdings"] = holdingData["holdings"]
	book["holding_total"] = holdingData["total"]
	book["holding_maps"] = map[string]any{
		"libraries": holdingData["libraries"], "locations": holdingData["locations"],
		"states": holdingData["states"], "circulation_types": holdingData["circulation_types"],
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "OPAC 书目详情页面及 /api/holding 返回",
		"service":  libraryCatalogService, "operation": "book", "id": id, "data": book, "book": book,
	}, nil
}

func libraryCatalogBookFromPage(document *pageNode, id string) (map[string]any, *siteError) {
	table := document.first("table", "bookInfoTable")
	if table == nil {
		return nil, &siteError{Code: "parse_error", Message: "图书馆书目详情缺少书目信息表"}
	}
	title := ""
	if heading := table.first("h2", ""); heading != nil {
		title = strings.TrimSpace(pageDisplayText(heading))
	}
	if title == "" {
		return nil, &siteError{Code: "parse_error", Message: "图书馆书目详情缺少题名"}
	}
	fields := make(map[string][]string)
	book := map[string]any{"id": id, "title": title, "authors": []string{}, "subjects": []string{}}
	for _, row := range table.findAll("tr") {
		cells := make([]*pageNode, 0, 2)
		for _, child := range row.children {
			if child.tag == "td" {
				cells = append(cells, child)
			}
		}
		if len(cells) < 2 {
			continue
		}
		label := strings.TrimRight(strings.TrimSpace(pageDisplayText(cells[0])), " :：")
		value := strings.TrimSpace(pageDisplayText(cells[1]))
		if label == "" {
			continue
		}
		fields[label] = append(fields[label], value)
		switch label {
		case "题名/责任者":
			book["responsibility"] = value
		case "ISBN":
			if match := libraryCatalogISBNPattern.FindString(value); match != "" {
				book["isbn"] = match
			}
			if match := libraryCatalogPricePattern.FindStringSubmatch(value); len(match) > 1 {
				book["price"] = match[1]
			}
		case "语种":
			book["language"] = value
		case "载体形态":
			book["physical_description"] = value
		case "出版发行":
			book["publication_statement"] = value
		case "内容提要":
			book["summary"] = value
		case "主题词":
			book["subjects"] = append(book["subjects"].([]string), libraryCatalogLinkTexts(cells[1], value)...)
		case "中图分类法":
			if parts := strings.Fields(value); len(parts) > 0 {
				book["classification"] = parts[0]
			}
		case "主要责任者":
			book["authors"] = append(book["authors"].([]string), libraryCatalogLinkTexts(cells[1], value)...)
		case "附注":
			book["notes"] = value
		case "随书附盘：", "随书附盘":
			book["supplemental_material"] = value
		case "电子图书":
			book["electronic_resource"] = value
		case "相关主题":
			book["related_topics"] = libraryCatalogLinkTexts(cells[1], value)
		}
	}
	book["bibliographic_fields"] = fields
	return book, nil
}

func libraryCatalogLinkTexts(node *pageNode, fallback string) []string {
	values := make([]string, 0)
	for _, link := range node.findAll("a") {
		if value := strings.TrimSpace(pageDisplayText(link)); value != "" {
			values = append(values, value)
		}
	}
	if len(values) > 0 {
		return values
	}
	parts := strings.Fields(strings.ReplaceAll(fallback, "\n", " "))
	if len(parts) == 0 && fallback != "" {
		return []string{fallback}
	}
	return parts
}

func (a NativeSite) libraryCatalogHoldings(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	id, requiredErr := businessRequired(args, "--id", "holdings 必须提供 --id")
	if requiredErr != nil {
		return nil, requiredErr
	}
	data, requestErr := a.libraryCatalogHoldingsData(ctx, strings.TrimSpace(id), cookie)
	if requestErr != nil {
		return nil, requestErr
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "OPAC /api/holding 返回馆藏清单及状态映射",
		"service":  libraryCatalogService, "operation": "holdings", "id": strings.TrimSpace(id),
		"data": data, "holdings": data["holdings"], "total": data["total"],
	}, nil
}

func (a NativeSite) libraryCatalogHoldingsData(ctx context.Context, id, cookie string) (map[string]any, *siteError) {
	result, requestErr := a.businessGet(ctx, libraryCatalogService, "/api/holding/"+url.PathEscape(id), []pair{{"limitLibcodes", ""}, {"isCluster", "false"}}, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := businessJSONMap(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	rawRows, ok := payload["holdingList"].([]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "图书馆馆藏响应缺少 holdingList"}
	}
	holdings := make([]map[string]any, 0, len(rawRows))
	for _, raw := range rawRows {
		if row, ok := raw.(map[string]any); ok {
			holdings = append(holdings, libraryCatalogHolding(row, payload))
		}
	}
	data := map[string]any{"holdings": holdings, "total": len(holdings)}
	for key, output := range map[string]string{
		"libcodeMap": "libraries", "localMap": "locations", "holdStateMap": "states", "pBCtypeMap": "circulation_types",
	} {
		if value, exists := payload[key]; exists {
			data[output] = value
		}
	}
	return data, nil
}

func libraryCatalogHolding(row, payload map[string]any) map[string]any {
	libraryCode := libraryCatalogText(row["curlib"])
	ownerLibraryCode := libraryCatalogText(row["orglib"])
	locationCode := libraryCatalogText(row["curlocal"])
	ownerLocationCode := libraryCatalogText(row["orglocal"])
	state := row["state"]
	stateName := libraryCatalogStateName(payload["holdStateMap"], state)
	circulationType := libraryCatalogText(row["cirtype"])
	circulationTypeName := libraryCatalogMapText(payload["pBCtypeMap"], circulationType, "name")
	return map[string]any{
		"id": row["recno"], "book_id": row["bookrecno"], "barcode": row["barcode"], "call_number": row["callno"],
		"state": state, "state_name": firstNonEmpty(stateName, libraryCatalogText(row["stateStr"])),
		"library_code": libraryCode, "library": libraryCatalogMapText(payload["libcodeMap"], libraryCode, ""),
		"owner_library_code": ownerLibraryCode, "owner_library": libraryCatalogMapText(payload["libcodeMap"], ownerLibraryCode, ""),
		"location_code": locationCode, "location": libraryCatalogMapText(payload["localMap"], locationCode, ""),
		"owner_location_code": ownerLocationCode, "owner_location": libraryCatalogMapText(payload["localMap"], ownerLocationCode, ""),
		"circulation_type": circulationType, "circulation_type_name": circulationTypeName,
		"shelf": row["shelfno"], "memo": row["memoinfo"], "volume": row["volnum"], "volume_info": row["volInfo"],
		"registered_at": row["regdate"], "cataloged_at": row["indate"], "price": row["singlePrice"],
		"loan_count": row["totalLoanNum"], "renew_count": row["totalRenewNum"], "reservation_count": row["totalResNum"],
	}
}

func libraryCatalogStateName(value any, state any) string {
	return libraryCatalogMapText(value, libraryCatalogText(state), "stateName")
}

func libraryCatalogMapText(value any, key, nestedKey string) string {
	items, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	item, ok := items[key]
	if !ok {
		return ""
	}
	if nestedKey == "" {
		return libraryCatalogText(item)
	}
	if object, ok := item.(map[string]any); ok {
		return libraryCatalogText(object[nestedKey])
	}
	return ""
}

func libraryCatalogText(value any) string {
	if value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "<nil>" {
		return ""
	}
	return text
}

func libraryCatalogParseInt(value string) int {
	parsed, err := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(value), ",", ""))
	if err != nil {
		return 0
	}
	return parsed
}

func libraryCatalogInt(value any) int {
	return libraryCatalogParseInt(libraryCatalogText(value))
}
