package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const libraryRemoteService = "library-remote"

var (
	libraryRemoteDataPattern   = regexp.MustCompile(`(?s)\$\("#totalCount"\)\.text\((\[.*\])\.length\)`)
	libraryRemoteAccessPattern = regexp.MustCompile(`(?i)jinru\(\s*['"]([^'"]+)['"]`)
	libraryRemoteLetterPattern = regexp.MustCompile(`^[A-Za-z]$`)
)

func (a NativeSite) executeLibraryRemote(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return businessCatalogFilter(libraryRemoteService), nil
	}
	cookie, _, valueErr := businessValue(args, "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "databases", "list":
		return a.libraryRemoteDatabases(ctx, args[1:], cookie)
	case "database", "detail":
		return a.libraryRemoteDatabaseDetail(ctx, args[1:], cookie)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "library-remote 只支持 databases、database、catalog"}
	}
}

func (a NativeSite) libraryRemoteRequest(ctx context.Context, method, path string, params []pair, data []pair, cookie string) (map[string]any, *siteError) {
	return a.execute(ctx, siteRequest{
		Service: libraryRemoteService, Method: method, Path: path, Params: params, Data: data,
		CookieFile: cookie, AllowBusinessFailure: true, ReadOnly: true, Yes: true, RawJSON: true,
	})
}

func libraryRemoteBody(result map[string]any) (string, *siteError) {
	response, ok := result["response"].(map[string]any)
	if !ok {
		return "", &siteError{Code: "parse_error", Message: "图书馆远程服务响应缺少页面数据"}
	}
	if body, ok := response["body_internal"].(string); ok && body != "" {
		return body, nil
	}
	if body, ok := response["body"].(string); ok && body != "" {
		return body, nil
	}
	return "", &siteError{Code: "parse_error", Message: "图书馆远程服务响应为空"}
}

func (a NativeSite) libraryRemoteDatabases(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	keyword, _, valueErr := businessValue(args, "--keyword")
	if valueErr != nil {
		return nil, valueErr
	}
	letter, _, valueErr := businessValue(args, "--letter")
	if valueErr != nil {
		return nil, valueErr
	}
	subject, _, valueErr := businessValue(args, "--subject")
	if valueErr != nil {
		return nil, valueErr
	}
	sortValue, _, valueErr := businessValue(args, "--sort")
	if valueErr != nil {
		return nil, valueErr
	}
	letter = strings.ToUpper(strings.TrimSpace(letter))
	if letter != "" && !libraryRemoteLetterPattern.MatchString(letter) {
		return nil, &siteError{Code: "invalid_argument", Message: "--letter 必须是 A-Z 的单个字母"}
	}
	sortType, sortName, sortErr := libraryRemoteSort(sortValue)
	if sortErr != nil {
		return nil, sortErr
	}
	subject = strings.TrimSpace(subject)
	subjectCode := ""
	if subject != "" {
		var subjectErr *siteError
		subjectCode, subjectErr = a.libraryRemoteSubjectCode(ctx, subject, cookie)
		if subjectErr != nil {
			return nil, subjectErr
		}
	}
	data := []pair{
		{"elecName", strings.TrimSpace(keyword)}, {"typeZm", letter}, {"sortType", sortType},
		{"elecMuTypes", subjectCode}, {"category", ""}, {"language", ""},
	}
	result, requestErr := a.libraryRemoteRequest(ctx, "POST", "/accessData", nil, data, cookie)
	if requestErr != nil {
		return nil, requestErr
	}
	body, bodyErr := libraryRemoteBody(result)
	if bodyErr != nil {
		return nil, bodyErr
	}
	rows, rowsErr := libraryRemoteRecords(body)
	if rowsErr != nil {
		return nil, rowsErr
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, libraryRemoteDatabase(row))
	}
	response := map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "公开资源导航接口返回嵌入式 JSON 清单", "service": libraryRemoteService,
		"operation": "databases", "data": items, "total": len(items),
		"filters": map[string]any{"keyword": strings.TrimSpace(keyword), "letter": letter, "subject": subject, "sort": sortName},
	}
	return response, nil
}

func libraryRemoteSort(value string) (string, string, *siteError) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "default":
		return "-1", "default", nil
	case "visits", "usage", "popular":
		return "0", "visits", nil
	case "alphabetical", "name":
		return "2", "alphabetical", nil
	case "alphabetical-desc", "name-desc":
		return "3", "alphabetical-desc", nil
	default:
		return "", "", &siteError{Code: "invalid_argument", Message: "--sort 只能是 default、visits、alphabetical 或 alphabetical-desc"}
	}
}

func (a NativeSite) libraryRemoteSubjectCode(ctx context.Context, label, cookie string) (string, *siteError) {
	result, requestErr := a.libraryRemoteRequest(ctx, "GET", "/", nil, nil, cookie)
	if requestErr != nil {
		return "", requestErr
	}
	body, bodyErr := libraryRemoteBody(result)
	if bodyErr != nil {
		return "", bodyErr
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return "", &siteError{Code: "parse_error", Message: "图书馆远程服务筛选项解析失败: " + parseErr.Error()}
	}
	options := make(map[string]string)
	for _, button := range document.findAll("button") {
		value := strings.TrimSpace(button.attr("value"))
		name := strings.TrimSpace(pageDisplayText(button))
		if value == "" || name == "" || name == "全部" || libraryRemoteLetterPattern.MatchString(name) {
			continue
		}
		options[name] = value
	}
	if code := options[label]; code != "" {
		return code, nil
	}
	labels := make([]string, 0, len(options))
	for name := range options {
		labels = append(labels, name)
	}
	sort.Strings(labels)
	return "", &siteError{Code: "invalid_argument", Message: "--subject 未找到图书馆学科筛选项", Details: map[string]any{"subject": label, "available_subjects": labels}}
}

func libraryRemoteRecords(source string) ([]map[string]any, *siteError) {
	match := libraryRemoteDataPattern.FindStringSubmatch(source)
	if len(match) < 2 {
		return nil, &siteError{Code: "parse_error", Message: "图书馆远程服务响应缺少资源清单"}
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(match[1]), &rows); err != nil {
		return nil, &siteError{Code: "parse_error", Message: "图书馆远程资源清单解析失败: " + err.Error()}
	}
	return rows, nil
}

func libraryRemoteDatabase(row map[string]any) map[string]any {
	status := libraryRemoteText(row, "errorData", "erroIsDown")
	if status == "" {
		status = "available"
	}
	return map[string]any{
		"id":                   libraryRemoteText(row, "id"),
		"name":                 libraryRemoteText(row, "elecName"),
		"access_url":           libraryRemoteText(row, "elecUrl"),
		"alternate_access_url": libraryRemoteText(row, "urlSpare"),
		"initial":              libraryRemoteText(row, "typeZm"),
		"description":          libraryRemoteHTMLText(row["remarks"]),
		"provider":             libraryRemoteText(row, "companyName"),
		"abbreviation":         libraryRemoteText(row, "abbreviation"),
		"visits":               row["stat"],
		"last_checked":         row["lastDate"],
		"service_type":         row["serviceType"],
		"free":                 row["isFree"],
		"status":               status,
	}
}

func (a NativeSite) libraryRemoteDatabaseDetail(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	id, requiredErr := businessRequired(args, "--id", "database detail 必须提供 --id")
	if requiredErr != nil {
		return nil, requiredErr
	}
	id = strings.TrimSpace(id)
	result, requestErr := a.libraryRemoteRequest(ctx, "GET", "/detail", []pair{{"id", id}}, nil, cookie)
	if requestErr != nil {
		return nil, requestErr
	}
	body, bodyErr := libraryRemoteBody(result)
	if bodyErr != nil {
		return nil, bodyErr
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "图书馆远程资源详情解析失败: " + parseErr.Error()}
	}
	detail := libraryRemoteNodeWithClass(document, "detail-main")
	if detail == nil {
		return nil, &siteError{Code: "parse_error", Message: "图书馆远程资源详情缺少 detail-main"}
	}
	title := ""
	for _, heading := range detail.findAll("h4") {
		if value := strings.TrimSpace(pageDisplayText(heading)); value != "" {
			title = value
			break
		}
	}
	if title == "" {
		return nil, &siteError{Code: "parse_error", Message: "图书馆远程资源详情缺少名称"}
	}
	database := map[string]any{"id": id, "name": title}
	list := libraryRemoteNodeWithClass(detail, "detail-list")
	if list == nil {
		return nil, &siteError{Code: "parse_error", Message: "图书馆远程资源详情缺少字段列表"}
	}
	for _, item := range list.findAll("li") {
		labelNode := libraryRemoteNodeWithClass(item, "dt")
		valueNode := libraryRemoteNodeWithClass(item, "dd")
		if labelNode == nil || valueNode == nil {
			continue
		}
		label := strings.TrimRight(strings.TrimSpace(pageDisplayText(labelNode)), " :：")
		value := strings.TrimSpace(pageDisplayText(valueNode))
		switch label {
		case "访问地址":
			for _, link := range valueNode.findAll("a") {
				if match := libraryRemoteAccessPattern.FindStringSubmatch(link.attr("onclick")); len(match) > 1 {
					database["access_url"] = match[1]
					break
				}
			}
		case "首页访问量":
			database["visits"] = libraryRemoteNumber(value)
		case "首字母":
			database["initial"] = value
		case "资源类型":
			database["resource_types"] = libraryRemoteSplit(value)
		case "学科":
			database["subjects"] = libraryRemoteSplit(value)
		case "语种":
			database["languages"] = libraryRemoteSplit(value)
		case "资源简介":
			if paragraphs := valueNode.findAll("p"); len(paragraphs) > 0 {
				value = strings.TrimSpace(pageDisplayText(paragraphs[0]))
			}
			database["description"] = value
		}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "公开资源详情页面返回结构化字段", "service": libraryRemoteService,
		"operation": "database", "id": id, "data": database, "database": database,
	}, nil
}

func libraryRemoteNodeWithClass(node *pageNode, className string) *pageNode {
	for _, candidate := range node.findAll("") {
		for _, class := range strings.Fields(candidate.attr("class")) {
			if class == className {
				return candidate
			}
		}
	}
	return nil
}

func libraryRemoteText(row map[string]any, paths ...string) string {
	for _, path := range paths {
		value, ok := row[path]
		if ok && value != nil {
			if text := strings.TrimSpace(fmt.Sprint(value)); text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

func libraryRemoteHTMLText(value any) string {
	source := strings.TrimSpace(fmt.Sprint(value))
	if source == "" || source == "<nil>" {
		return ""
	}
	document, err := parsePage("<div>" + source + "</div>")
	if err != nil {
		return strings.TrimSpace(source)
	}
	return strings.TrimSpace(pageDisplayText(document))
}

func libraryRemoteNumber(value string) any {
	parsed, err := strconv.ParseInt(strings.ReplaceAll(strings.TrimSpace(value), ",", ""), 10, 64)
	if err == nil {
		return parsed
	}
	return value
}

func libraryRemoteSplit(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}
