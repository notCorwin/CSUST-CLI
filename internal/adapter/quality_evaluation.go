package adapter

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

func (a NativeSite) runQualityEvaluation(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) < 2 {
		return nil, &siteError{Code: "invalid_argument", Message: "quality evaluation 缺少子命令"}
	}
	operation := args[1]
	path, batchSelector, courseID, courseName, yes, suggestion, clearSuggestion := "", "", "", "", false, "", false
	answers := []pair{}
	for index := 2; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		if arg == "--json" {
			continue
		}
		if arg == "--yes" {
			yes = true
			continue
		}
		if arg == "--clear-suggestion" {
			clearSuggestion = true
			continue
		}
		if !inline {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return nil, &siteError{Code: "invalid_argument", Message: arg + " 缺少参数值"}
			}
			index++
			value = args[index]
		}
		switch arg {
		case "--path":
			path = value
		case "--batch":
			batchSelector = value
		case "--course-id":
			courseID = value
		case "--course-name":
			courseName = value
		case "--answer":
			item, err := splitPair(value, "--answer")
			if err != nil {
				return nil, err
			}
			answers = append(answers, item)
		case "--suggestion":
			suggestion = value
		default:
			return nil, &siteError{Code: "invalid_argument", Message: "quality evaluation 参数无效: " + arg}
		}
	}
	if operation != "batches" && path == "" {
		var targetErr *siteError
		path, targetErr = a.qualityEvaluationTarget(ctx, operation, batchSelector, courseID, courseName)
		if targetErr != nil {
			return nil, targetErr
		}
	}
	if (operation == "save" || operation == "submit") && !yes {
		return nil, &siteError{Code: "confirmation_required", Message: "保存或提交评价会修改账号数据，请加 --yes"}
	}
	if operation == "batches" {
		return a.qualityEvaluationPage(ctx, "/jsxsd/xspj/xspj_find.do", "batches")
	}
	_, document, pageURL, err := a.qualityEvaluationPageDocument(ctx, path)
	if err != nil {
		return nil, err
	}
	switch operation {
	case "courses":
		return parseQualityEvaluationCourses(document, pageURL), nil
	case "form":
		formResult := parseQualityEvaluationForm(document, pageURL, false)
		if formResult["action"] == "" {
			return nil, &siteError{Code: "parse_error", Message: "未找到课程评价表"}
		}
		return formResult, nil
	case "save", "submit":
		formResult := parseQualityEvaluationForm(document, pageURL, true)
		if formResult["action"] == "" {
			return nil, &siteError{Code: "parse_error", Message: "未找到课程评价表"}
		}
		if formResult["read_only"] == true {
			return nil, &siteError{Code: "already_submitted", Message: "该评价已经提交，不能再修改"}
		}
		data, buildErr := qualityEvaluationFields(document, pageURL, answers, suggestion, clearSuggestion, operation)
		if buildErr != nil {
			return nil, buildErr
		}
		formAction := fmt.Sprint(formResult["action"])
		if parsed, parseErr := url.Parse(formAction); parseErr == nil && parsed.IsAbs() {
			formAction = parsed.EscapedPath()
			if parsed.RawQuery != "" {
				formAction += "?" + parsed.RawQuery
			}
		}
		request := gatewayRequest{path: formAction, method: "POST", data: data, referer: pageURL, yes: true}
		result, submitErr := a.runGatewayRequest(ctx, qualityServiceName, request)
		if submitErr != nil {
			return nil, submitErr
		}
		message := ""
		if page, ok := result["response"].(map[string]any); ok {
			message = fmt.Sprint(page["text"])
		}
		return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "confirmed", "operation": operation, "message": message, "request": map[string]any{"method": "POST", "path": formAction, "fields": fieldNames(data)}}, nil
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "未知评价子命令: " + operation}
	}
}

func (a NativeSite) qualityEvaluationTarget(ctx context.Context, operation, batchSelector, courseID, courseName string) (string, *siteError) {
	_, document, pageURL, err := a.qualityEvaluationPageDocument(ctx, "/jsxsd/xspj/xspj_find.do")
	if err != nil {
		return "", err
	}
	batchResult := parseQualityEvaluationBatches(document, pageURL)
	batches, _ := batchResult["items"].([]map[string]any)
	batch, selectErr := selectEvaluationItem(batches, batchSelector, "sequence", "semester", "category", "name")
	if selectErr != nil {
		return "", selectErr
	}
	batchPath, pathErr := evaluationRelativePath(fmt.Sprint(batch["path"]))
	if pathErr != nil {
		return "", pathErr
	}
	if operation == "courses" {
		return batchPath, nil
	}
	_, courseDocument, courseURL, courseErr := a.qualityEvaluationPageDocument(ctx, batchPath)
	if courseErr != nil {
		return "", courseErr
	}
	courseResult := parseQualityEvaluationCourses(courseDocument, courseURL)
	courses, _ := courseResult["items"].([]map[string]any)
	course, courseSelectErr := selectEvaluationItem(courses, firstNonEmpty(courseID, courseName), "course_id", "course", "name")
	if courseSelectErr != nil {
		return "", courseSelectErr
	}
	return evaluationRelativePath(fmt.Sprint(course["path"]))
}

func evaluationRelativePath(value string) (string, *siteError) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil {
		return "", &siteError{Code: "invalid_path", Message: "评价页面地址无效"}
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	if parsed.RawQuery != "" {
		path += "?" + parsed.RawQuery
	}
	return path, nil
}

func (a NativeSite) qualityEvaluationPage(ctx context.Context, path, kind string) (map[string]any, *siteError) {
	result, document, pageURL, err := a.qualityEvaluationPageDocument(ctx, path)
	if err != nil {
		return nil, err
	}
	switch kind {
	case "batches":
		return parseQualityEvaluationBatches(document, pageURL), nil
	default:
		return result, nil
	}
}

func (a NativeSite) qualityEvaluationPageDocument(ctx context.Context, path string) (map[string]any, *pageNode, string, *siteError) {
	request := gatewayRequest{path: path, method: "GET", raw: true}
	result, err := a.runGatewayRequest(ctx, qualityServiceName, request)
	if err != nil {
		return nil, nil, "", err
	}
	source, _ := result["raw_body"].(string)
	pageURL, _ := result["raw_url"].(string)
	if source == "" || pageURL == "" {
		return nil, nil, "", &siteError{Code: "parse_error", Message: "评价页面没有可解析的 HTML 响应"}
	}
	document, parseErr := parsePage(source)
	if parseErr != nil {
		return nil, nil, "", &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	return result, document, pageURL, nil
}

func directPageChildren(node *pageNode, tag string) []*pageNode {
	result := make([]*pageNode, 0)
	if node == nil {
		return result
	}
	for _, child := range node.children {
		if child.tag == tag {
			result = append(result, child)
		}
	}
	return result
}

func evaluationCells(row *pageNode) []*pageNode {
	cells := directPageChildren(row, "td")
	if len(cells) == 0 {
		cells = directPageChildren(row, "th")
	}
	return cells
}

func firstPageLink(row *pageNode, pageURL string) string {
	link := row.first("a", "")
	if link == nil {
		return ""
	}
	href := link.attr("href")
	if strings.HasPrefix(strings.ToLower(href), "javascript:") {
		href = pageQuotedTarget(href, pageURL)
	}
	if href == "" {
		return ""
	}
	return pageSafeValue(resolvePageURL(pageURL, href), pageURL)
}

func parseQualityEvaluationBatches(document *pageNode, pageURL string) map[string]any {
	items := make([]map[string]any, 0)
	for _, table := range document.findAll("table") {
		for _, row := range table.findAll("tr") {
			cells := evaluationCells(row)
			values := make([]string, 0, len(cells))
			for _, cell := range cells {
				values = append(values, strings.TrimSpace(pageDisplayText(cell)))
			}
			path := firstPageLink(row, pageURL)
			if len(values) < 6 || path == "" || values[0] == "序号" {
				continue
			}
			items = append(items, map[string]any{"index": len(items) + 1, "sequence": values[0], "semester": values[1], "category": values[2], "name": values[3], "start_date": values[4], "end_date": values[5], "path": path, "cells": values, "text": strings.TrimSpace(pageDisplayText(row))})
		}
	}
	return map[string]any{"url": safeSiteURL(mustParseURL(pageURL)), "items": items}
}

func parseQualityEvaluationCourses(document *pageNode, pageURL string) map[string]any {
	items := make([]map[string]any, 0)
	table := document.first("table", "dataList")
	if table == nil {
		return map[string]any{"url": safeSiteURL(mustParseURL(pageURL)), "items": items}
	}
	for _, row := range table.findAll("tr") {
		cells := evaluationCells(row)
		values := make([]string, 0, len(cells))
		for _, cell := range cells {
			values = append(values, strings.TrimSpace(pageDisplayText(cell)))
		}
		path := firstPageLink(row, pageURL)
		if len(values) < 9 || path == "" || values[0] == "序号" {
			continue
		}
		items = append(items, map[string]any{"index": len(items) + 1, "sequence": values[0], "course_id": values[1], "course": values[2], "teacher": values[3], "category": values[4], "total_score": values[5], "evaluated": values[6] == "是", "submitted": values[7] == "是", "hours": values[8], "path": path, "cells": values, "text": strings.TrimSpace(pageDisplayText(row))})
	}
	return map[string]any{"url": safeSiteURL(mustParseURL(pageURL)), "items": items}
}

func parseQualityEvaluationForm(document *pageNode, pageURL string, includeSensitive bool) map[string]any {
	var form *pageNode
	for _, candidate := range document.findAll("form") {
		if strings.Contains(candidate.attr("action"), "xspj_save.do") {
			form = candidate
			break
		}
	}
	if form == nil {
		return map[string]any{"url": safeSiteURL(mustParseURL(pageURL)), "read_only": true, "questions": []map[string]any{}}
	}
	hidden := make([]map[string]any, 0)
	for _, node := range form.findAll("input") {
		if strings.ToLower(firstNonEmpty(node.attr("type"), "text")) != "hidden" || node.attr("name") == "" || node.disabled() {
			continue
		}
		value := node.attr("value")
		if !includeSensitive && pageSensitiveField.MatchString(node.attr("name")) {
			value = "<redacted>"
		}
		hidden = append(hidden, map[string]any{"name": node.attr("name"), "value": value})
	}
	questions := make([]map[string]any, 0)
	for _, row := range form.findAll("tr") {
		cells := directPageChildren(row, "td")
		if len(cells) < 2 {
			continue
		}
		var question *pageNode
		for _, input := range cells[0].findAll("input") {
			if input.attr("name") == "pj06xh" && input.attr("value") != "" && !input.disabled() {
				question = input
				break
			}
		}
		if question == nil {
			continue
		}
		options := make([]map[string]any, 0)
		for _, input := range cells[1].findAll("input") {
			if strings.ToLower(input.attr("type")) != "radio" || input.attr("value") == "" || input.disabled() {
				continue
			}
			options = append(options, map[string]any{"id": input.attr("value"), "label": input.attr("value"), "selected": input.has("checked")})
		}
		if len(options) > 0 {
			questions = append(questions, map[string]any{"id": question.attr("value"), "title": strings.TrimSpace(pageDisplayText(cells[0])), "options": options})
		}
	}
	suggestion := ""
	suggestionField := ""
	if textareas := form.findAll("textarea"); len(textareas) > 0 {
		for _, textarea := range textareas {
			if textarea.attr("name") != "" && !textarea.disabled() {
				suggestionField, suggestion = textarea.attr("name"), textarea.rawText()
				break
			}
		}
	}
	action := resolvePageURL(pageURL, form.attr("action"))
	readOnly := true
	for _, node := range form.findAll("") {
		if !node.disabled() && strings.Contains(strings.ToLower(node.attr("onclick")), "savedata") {
			readOnly = false
			break
		}
	}
	return map[string]any{"url": safeSiteURL(mustParseURL(pageURL)), "action": pageSafeValue(action, pageURL), "method": strings.ToUpper(firstNonEmpty(form.attr("method"), "POST")), "course": evaluationHeading(document, "课程名称"), "category": evaluationHeading(document, "评教大类"), "hidden_fields": hidden, "questions": questions, "suggestion_field": suggestionField, "suggestion": suggestion, "read_only": readOnly}
}

func evaluationHeading(document *pageNode, label string) string {
	text := pageDisplayText(document)
	pattern := regexp.MustCompile(regexp.QuoteMeta(label) + `\s*[：:]\s*([^\s]+)`)
	match := pattern.FindStringSubmatch(text)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

func qualityEvaluationFields(document *pageNode, pageURL string, answers []pair, suggestion string, clearSuggestion bool, operation string) ([]pair, *siteError) {
	form := (*pageNode)(nil)
	for _, candidate := range document.findAll("form") {
		if strings.Contains(candidate.attr("action"), "xspj_save.do") {
			form = candidate
			break
		}
	}
	if form == nil {
		return nil, &siteError{Code: "parse_error", Message: "未找到课程评价表"}
	}
	questions := map[string]map[string]bool{}
	for _, row := range form.findAll("tr") {
		cells := directPageChildren(row, "td")
		if len(cells) < 2 {
			continue
		}
		id := ""
		for _, input := range cells[0].findAll("input") {
			if input.attr("name") == "pj06xh" {
				id = input.attr("value")
			}
		}
		if id == "" {
			continue
		}
		options := map[string]bool{}
		for _, input := range cells[1].findAll("input") {
			if strings.ToLower(input.attr("type")) == "radio" && input.attr("value") != "" {
				options[input.attr("value")] = true
			}
		}
		if len(options) > 0 {
			questions[id] = options
		}
	}
	answerMap := map[string]string{}
	for _, answer := range answers {
		answerMap[answer.name] = answer.value
	}
	for id, options := range questions {
		value, ok := answerMap[id]
		if !ok {
			return nil, &siteError{Code: "missing_answer", Message: "缺少评价题目 " + id + " 的答案"}
		}
		if !options[value] {
			return nil, &siteError{Code: "invalid_answer", Message: "评价题目 " + id + " 的选项无效"}
		}
	}
	for id := range answerMap {
		if _, ok := questions[id]; !ok {
			return nil, &siteError{Code: "invalid_answer", Message: "评价题目不存在: " + id}
		}
	}
	fields := make([]pair, 0)
	for _, input := range form.findAll("input") {
		name := input.attr("name")
		if strings.ToLower(firstNonEmpty(input.attr("type"), "text")) == "hidden" && name != "" && name != "issubmit" && name != "sfxyt" && !input.disabled() {
			fields = append(fields, pair{name, input.attr("value")})
		}
	}
	for id, value := range answerMap {
		fields = append(fields, pair{"pj0601id_" + id, value})
	}
	for _, textarea := range form.findAll("textarea") {
		if textarea.attr("name") == "" || textarea.disabled() {
			continue
		}
		if clearSuggestion || suggestion != "" {
			if clearSuggestion {
				suggestion = ""
			}
			fields = append(fields, pair{textarea.attr("name"), suggestion})
		}
		break
	}
	fields = append(fields, pair{"issubmit", map[bool]string{true: "1", false: "0"}[operation == "submit"]}, pair{"sfxyt", "0"})
	_ = pageURL
	return fields, nil
}
