package adapter

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

const (
	academicPreselectionListPath    = "/jsxsd/xkgl/xsyxgl"
	academicPreselectionCoursesPath = "/jsxsd/xkgl/xsyxxk"
	academicPreselectionSelectPath  = "/jsxsd/xkgl/yxxkOper"
	academicPreselectionDropPath    = "/jsxsd/xkgl/yxtkOper"
)

var (
	academicPreselectionTermPattern = regexp.MustCompile(`(?i)comeInXk\s*\(\s*['"]?([^,'")\s]+)`)
	academicPreselectionDropPattern = regexp.MustCompile(`(?i)yxtkOper\s*\(\s*['"]?([^,'")\s]+)`)
)

func (a NativeSite) academicPreselection(ctx context.Context, args []string) (map[string]any, *siteError) {
	operation := "list"
	if len(args) > 0 && !strings.HasPrefix(args[0], "--") {
		operation, args = strings.ToLower(args[0]), args[1:]
	}
	switch operation {
	case "list", "windows":
		return a.academicPreselectionList(ctx, args)
	case "courses", "query":
		return a.academicPreselectionCourses(ctx, args)
	case "select", "add":
		return a.academicPreselectionMutate(ctx, "select", args)
	case "drop", "remove", "unselect":
		return a.academicPreselectionMutate(ctx, "drop", args)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "preselection 只支持 list、courses、select 或 drop"}
	}
}

func (a NativeSite) academicPreselectionList(ctx context.Context, args []string) (map[string]any, *siteError) {
	if err := academicPreselectionArgs(args, map[string]bool{"--keyword": true}); err != nil {
		return nil, err
	}
	items, pageURL, page, err := a.academicPreselectionTermsFromService(ctx, strings.TrimSpace(flagValue(args, "--keyword")))
	if err != nil {
		return nil, err
	}
	return academicWrap(map[string]any{
		"kind": "preselection", "operation": "list", "path": academicPreselectionListPath,
		"keyword": nullableString(strings.TrimSpace(flagValue(args, "--keyword"))),
		"items":   items, "item_count": len(items), "page": page, "page_url": pageURL,
	}), nil
}

func (a NativeSite) academicPreselectionCourses(ctx context.Context, args []string) (map[string]any, *siteError) {
	if err := academicPreselectionArgs(args, map[string]bool{"--term": true}); err != nil {
		return nil, err
	}
	termID, termLabel, _, err := a.academicPreselectionTerm(ctx, flagValue(args, "--term"))
	if err != nil {
		return nil, err
	}
	items, pageURL, page, forms, err := a.academicPreselectionCoursesFromService(ctx, termID)
	if err != nil {
		return nil, err
	}
	return academicWrap(map[string]any{
		"kind": "preselection", "operation": "courses", "path": academicPreselectionCoursesPath,
		"term_id": termID, "term": nullableString(termLabel), "items": items, "item_count": len(items),
		"forms": forms, "page": page, "page_url": pageURL,
	}), nil
}

func (a NativeSite) academicPreselectionMutate(ctx context.Context, operation string, args []string) (map[string]any, *siteError) {
	if err := academicPreselectionArgs(args, map[string]bool{"--term": true, "--course-id": true, "--course-name": true, "--yes": false}); err != nil {
		return nil, err
	}
	if !flagPresent(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "预选课操作会修改账号数据，请加 --yes"}
	}
	selector := strings.TrimSpace(flagValue(args, "--course-id"))
	selectorField := "course_id"
	if selector == "" {
		selector = strings.TrimSpace(flagValue(args, "--course-name"))
		selectorField = "course"
	}
	if selector == "" {
		return nil, &siteError{Code: "invalid_argument", Message: operation + " 必须提供 --course-id 或 --course-name"}
	}
	termID, termLabel, coursePageURL, items, err := a.academicPreselectionTarget(ctx, flagValue(args, "--term"))
	if err != nil {
		return nil, err
	}
	item, selectErr := academicPreselectionItem(items, selector, selectorField)
	if selectErr != nil {
		return nil, selectErr
	}
	selected, _ := item["selected"].(bool)
	if operation == "select" {
		if selected {
			return nil, &siteError{Code: "already_selected", Message: "目标课程已经处于预选状态"}
		}
		selectionValue := strings.TrimSpace(fmt.Sprint(item["selection_value"]))
		if selectionValue == "" {
			return nil, &siteError{Code: "not_found", Message: "目标课程缺少可提交的预选编号"}
		}
		result, submitErr := a.executeAcademicRequestWithRecovery(ctx, siteRequest{
			Service: "academic", Path: academicPreselectionSelectPath, Method: "GET", CookieFile: academicCookiePath(),
			Params:  []pair{{"xnxq01id", termID}, {"kcCheck", selectionValue}},
			Headers: []pair{{"Referer", coursePageURL}}, RequireLogin: true, Yes: true, mutating: true,
		})
		if submitErr != nil {
			return nil, submitErr
		}
		return a.academicPreselectionVerifyMutation(ctx, operation, termID, termLabel, selector, selectorField, item, result)
	}

	dropID := strings.TrimSpace(fmt.Sprint(item["drop_id"]))
	if dropID == "" {
		return nil, &siteError{Code: "not_found", Message: "目标课程缺少可提交的退选编号"}
	}
	if !selected {
		return nil, &siteError{Code: "already_unselected", Message: "目标课程当前未处于已选状态"}
	}
	result, submitErr := a.executeAcademicRequestWithRecovery(ctx, siteRequest{
		Service: "academic", Path: academicPreselectionDropPath, Method: "GET", CookieFile: academicCookiePath(),
		Params:  []pair{{"xnxq01id", termID}, {"jx0505id", dropID}},
		Headers: []pair{{"Referer", coursePageURL}}, RequireLogin: true, Yes: true, mutating: true,
	})
	if submitErr != nil {
		return nil, submitErr
	}
	return a.academicPreselectionVerifyMutation(ctx, operation, termID, termLabel, selector, selectorField, item, result)
}

func (a NativeSite) academicPreselectionVerifyMutation(ctx context.Context, operation, termID, termLabel, selector, selectorField string, before map[string]any, result map[string]any) (map[string]any, *siteError) {
	after, _, _, _, readbackErr := a.academicPreselectionCoursesFromService(ctx, termID)
	if readbackErr != nil {
		return nil, &siteError{Code: "mutation_unverified", Message: "预选课操作成功反馈已返回，但课程列表回读失败", Details: map[string]any{
			"submitted": true, "confirmed": false, "operation": operation, "cause": readbackErr.Code,
		}}
	}
	item, itemErr := academicPreselectionItem(after, selector, selectorField)
	if operation == "select" {
		if itemErr != nil || item["selected"] != true {
			return nil, &siteError{Code: "mutation_unverified", Message: "预选课提交成功反馈已返回，但回读未确认课程已选", Details: map[string]any{
				"submitted": true, "confirmed": false, "operation": operation, "selector": selector,
			}}
		}
	} else if itemErr == nil && item["selected"] == true {
		return nil, &siteError{Code: "mutation_unverified", Message: "退选成功反馈已返回，但回读仍显示课程已选", Details: map[string]any{
			"submitted": true, "confirmed": false, "operation": operation, "selector": selector,
		}}
	}
	return academicWrap(map[string]any{
		"kind": "preselection", "operation": operation, "path": map[string]string{"select": academicPreselectionSelectPath, "drop": academicPreselectionDropPath}[operation],
		"term_id": termID, "term": nullableString(termLabel), "course": before,
		"response": result["response"], "submitted": true, "confirmed": true,
		"evidence": "server-success-and-course-list-readback",
	}), nil
}

func (a NativeSite) academicPreselectionTerm(ctx context.Context, selector string) (string, string, string, *siteError) {
	items, pageURL, _, err := a.academicPreselectionTermsFromService(ctx, "")
	if err != nil {
		return "", "", "", err
	}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		if len(items) == 1 {
			return fmt.Sprint(items[0]["term_id"]), fmt.Sprint(items[0]["term"]), pageURL, nil
		}
		if len(items) == 0 {
			return "", "", "", &siteError{Code: "not_found", Message: "当前没有可用的预选课阶段"}
		}
		return "", "", "", &siteError{Code: "ambiguous_target", Message: "预选课阶段不唯一，请提供 --term"}
	}
	for _, item := range items {
		if strings.EqualFold(selector, fmt.Sprint(item["term_id"])) || selector == fmt.Sprint(item["term"]) {
			return fmt.Sprint(item["term_id"]), fmt.Sprint(item["term"]), pageURL, nil
		}
	}
	matches := make([]map[string]any, 0)
	for _, item := range items {
		if strings.Contains(strings.ToLower(fmt.Sprint(item["term"])), strings.ToLower(selector)) || strings.Contains(strings.ToLower(fmt.Sprint(item["stage"])), strings.ToLower(selector)) {
			matches = append(matches, item)
		}
	}
	if len(matches) == 1 {
		return fmt.Sprint(matches[0]["term_id"]), fmt.Sprint(matches[0]["term"]), pageURL, nil
	}
	if len(matches) > 1 {
		return "", "", "", &siteError{Code: "ambiguous_target", Message: "--term 匹配多个预选课阶段", Details: map[string]any{"term": selector}}
	}
	return "", "", "", &siteError{Code: "not_found", Message: "找不到预选课阶段: " + selector}
}

func (a NativeSite) academicPreselectionTarget(ctx context.Context, selector string) (string, string, string, []map[string]any, *siteError) {
	termID, termLabel, _, termErr := a.academicPreselectionTerm(ctx, selector)
	if termErr != nil {
		return "", "", "", nil, termErr
	}
	items, pageURL, _, _, coursesErr := a.academicPreselectionCoursesFromService(ctx, termID)
	if coursesErr != nil {
		return "", "", "", nil, coursesErr
	}
	return termID, termLabel, pageURL, items, nil
}

func (a NativeSite) academicPreselectionTermsFromService(ctx context.Context, keyword string) ([]map[string]any, string, map[string]any, *siteError) {
	body, pageURL, err := a.academicPage(ctx, "GET", academicPreselectionListPath, nil, nil)
	if err != nil {
		return nil, "", nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, "", nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	items := academicPreselectionTermRows(document, keyword)
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, "", nil, pageErr
	}
	return items, pageURL, page, nil
}

func (a NativeSite) academicPreselectionCoursesFromService(ctx context.Context, termID string) ([]map[string]any, string, map[string]any, []map[string]any, *siteError) {
	body, pageURL, err := a.academicPage(ctx, "POST", academicPreselectionCoursesPath, []pair{{"xnxq01id", termID}}, nil)
	if err != nil {
		return nil, "", nil, nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, "", nil, nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	items := academicPreselectionCourseRows(document)
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, "", nil, nil, pageErr
	}
	forms := academicPreselectionForms(document, pageURL)
	return items, pageURL, page, forms, nil
}

func academicPreselectionForms(document *pageNode, pageURL string) []map[string]any {
	result := make([]map[string]any, 0)
	for index, form := range document.findAll("form") {
		fields := pageFormFields(form, nil, document)
		result = append(result, map[string]any{
			"index": index + 1, "name": form.attr("name"), "action": pageSafeValue(form.attr("action"), pageURL),
			"method": strings.ToUpper(firstNonEmpty(form.attr("method"), "GET")), "fields": fieldNames(fields),
		})
	}
	return result
}

func academicPreselectionTermRows(document *pageNode, keyword string) []map[string]any {
	table := academicPreselectionTable(document, "选课阶段")
	if table == nil {
		return []map[string]any{}
	}
	rows := directTableRows(table)
	if len(rows) == 0 {
		return []map[string]any{}
	}
	header := rowValues(rows[0])
	hasHeader := len(directCells(rows[0])) > 0 && (directCells(rows[0])[0].tag == "th" || preselectionTermHeaderCount(header) > 0)
	if hasHeader {
		rows = rows[1:]
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		values := rowValues(row)
		text := strings.TrimSpace(pageDisplayText(row))
		if len(values) == 0 || allEmpty(values) || academicNoDataRow(text) || keyword != "" && !strings.Contains(strings.ToLower(text), strings.ToLower(keyword)) {
			continue
		}
		item := map[string]any{"index": len(items) + 1, "cells": values, "text": text}
		if hasHeader {
			for index, title := range header {
				if field := academicPreselectionTermField(title); field != "" && index < len(values) {
					item[field] = values[index]
				}
			}
		}
		item["term_id"] = academicPreselectionTermID(row)
		if fmt.Sprint(item["term_id"]) == "" {
			continue
		}
		items = append(items, item)
	}
	return items
}

func academicPreselectionCourseRows(document *pageNode) []map[string]any {
	table := academicPreselectionTable(document, "课程编号")
	if table == nil {
		return []map[string]any{}
	}
	rows := directTableRows(table)
	if len(rows) == 0 {
		return []map[string]any{}
	}
	header := rowValues(rows[0])
	hasHeader := len(directCells(rows[0])) > 0 && (directCells(rows[0])[0].tag == "th" || preselectionCourseHeaderCount(header) > 0)
	if hasHeader {
		rows = rows[1:]
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		values := rowValues(row)
		text := strings.TrimSpace(pageDisplayText(row))
		if len(values) == 0 || allEmpty(values) || academicNoDataRow(text) {
			continue
		}
		item := map[string]any{"index": len(items) + 1, "cells": values, "text": text, "selected": false}
		if hasHeader {
			for index, title := range header {
				if field := academicPreselectionCourseField(title); field != "" && index < len(values) {
					item[field] = values[index]
				}
			}
		}
		for _, input := range row.findAll("input") {
			if input.attr("name") == "kcCheck" {
				item["selection_value"] = input.attr("value")
				item["selected"] = input.has("checked")
			}
		}
		item["drop_id"] = academicPreselectionDropID(row)
		if !item["selected"].(bool) {
			selectedText := strings.TrimSpace(fmt.Sprint(item["selected_status"]))
			item["selected"] = selectedText == "是" || selectedText == "√" || strings.Contains(selectedText, "已选")
		}
		items = append(items, item)
	}
	return items
}

func academicPreselectionTable(document *pageNode, marker string) *pageNode {
	var best *pageNode
	score := 0
	for _, table := range document.findAll("table") {
		text := pageDisplayText(table)
		current := len(directTableRows(table))
		if strings.Contains(text, marker) {
			current += 10
		}
		if current > score {
			best, score = table, current
		}
	}
	return best
}

func academicPreselectionTermID(row *pageNode) string {
	for _, node := range row.findAll("") {
		for _, source := range []string{node.attr("onclick"), node.attr("href")} {
			if match := academicPreselectionTermPattern.FindStringSubmatch(source); len(match) > 1 {
				return strings.TrimSpace(match[1])
			}
		}
	}
	return ""
}

func academicPreselectionDropID(row *pageNode) string {
	for _, node := range row.findAll("") {
		for _, source := range []string{node.attr("onclick"), node.attr("href")} {
			if match := academicPreselectionDropPattern.FindStringSubmatch(source); len(match) > 1 {
				return strings.TrimSpace(match[1])
			}
		}
	}
	return ""
}

func academicPreselectionItem(items []map[string]any, selector, field string) (map[string]any, *siteError) {
	selector = strings.TrimSpace(selector)
	matches := make([]map[string]any, 0, 1)
	for _, item := range items {
		if strings.TrimSpace(fmt.Sprint(item[field])) == selector {
			matches = append(matches, item)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return nil, &siteError{Code: "ambiguous_target", Message: "预选课程匹配多个结果，请使用更具体的课程编号或名称"}
	}
	return nil, &siteError{Code: "not_found", Message: "找不到预选课程: " + selector}
}

func academicPreselectionArgs(args []string, allowed map[string]bool) *siteError {
	for index := 0; index < len(args); index++ {
		arg, _, inline := splitInline(args[index])
		if arg == "--yes" {
			if inline {
				return &siteError{Code: "invalid_argument", Message: "--yes 不接受 =VALUE"}
			}
			continue
		}
		if !strings.HasPrefix(arg, "--") || !allowed[arg] {
			return &siteError{Code: "invalid_argument", Message: "preselection 参数无效: " + arg}
		}
		if !inline {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return &siteError{Code: "invalid_argument", Message: arg + " 缺少参数值"}
			}
			index++
		}
	}
	return nil
}

func preselectionTermHeaderCount(values []string) int {
	count := 0
	for _, value := range values {
		if academicPreselectionTermField(value) != "" {
			count++
		}
	}
	return count
}

func academicPreselectionTermField(value string) string {
	value = strings.Join(strings.Fields(value), "")
	switch {
	case strings.Contains(value, "学年学期") || value == "学期":
		return "term"
	case strings.Contains(value, "选课阶段"):
		return "stage"
	case strings.Contains(value, "开始时间"):
		return "start_at"
	case strings.Contains(value, "结束时间"):
		return "end_at"
	case value == "操作":
		return "actions"
	default:
		return ""
	}
}

func preselectionCourseHeaderCount(values []string) int {
	count := 0
	for _, value := range values {
		if academicPreselectionCourseField(value) != "" {
			count++
		}
	}
	return count
}

func academicPreselectionCourseField(value string) string {
	value = strings.Join(strings.Fields(value), "")
	switch {
	case strings.Contains(value, "课程组名称"):
		return "group"
	case strings.Contains(value, "学分要求"):
		return "credit_requirement"
	case strings.Contains(value, "课程编号") || strings.Contains(value, "课程代码"):
		return "course_id"
	case strings.Contains(value, "课程名称") || value == "课程":
		return "course"
	case strings.Contains(value, "开课单位"):
		return "department"
	case strings.Contains(value, "已选人数"):
		return "selected_count"
	case strings.Contains(value, "总学时") || value == "学时":
		return "hours"
	case value == "学分":
		return "credit"
	case strings.Contains(value, "是否选中"):
		return "selected_status"
	case value == "操作":
		return "actions"
	default:
		return ""
	}
}
