package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var textbookPaths = []string{"/jsxsd/nxsjc/jccx", "/jsxsd/xsjc/showXsjc", "/jsxsd/nxsjc/jczmxx", "/jsxsd/xsjc/xsjc.do"}

func (a NativeSite) runTextbookCommand(ctx context.Context, args []string, jsonMode bool) (bool, []byte, []byte, int, error) {
	if len(args) == 0 || (args[0] != "textbooks" && args[0] != "textbook" && args[0] != "order" && args[0] != "cancel") || containsHelp(args[1:]) {
		return false, nil, nil, 0, nil
	}
	result, err := a.executeTextbook(ctx, args)
	if err != nil {
		if jsonMode {
			return true, errorJSON(err), nil, 2, nil
		}
		return true, nil, []byte("错误: " + err.Error() + "\n"), 2, nil
	}
	if jsonMode {
		return true, mustJSON(result), nil, 0, nil
	}
	return true, []byte(renderTextbooks(result)), nil, 0, nil
}

func (a NativeSite) executeTextbook(ctx context.Context, args []string) (map[string]any, *siteError) {
	command := args[0]
	if command == "order" {
		command = "subscribe"
	}
	if command == "cancel" {
		command = "unsubscribe"
	}
	if command == "textbooks" || command == "textbook" {
		if len(args) < 2 {
			return nil, &siteError{Code: "invalid_argument", Message: "缺少 textbooks 子命令"}
		}
		command = args[1]
	}
	if command == "list" || command == "account" {
		path := textbookPaths[0]
		if command == "account" {
			path = textbookPaths[2]
		}
		body, pageURL, err := a.findTextbookPage(ctx, path, true)
		if err != nil {
			return nil, err
		}
		document, parseErr := parsePage(body)
		if parseErr != nil {
			return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
		}
		items, dataErr := parseTextbookPage(document, pageURL)
		if dataErr != nil {
			return nil, dataErr
		}
		return academicWrap(map[string]any{"url": safeSiteURL(mustParseURL(pageURL)), "items": items}), nil
	}
	if command != "subscribe" && command != "unsubscribe" {
		return nil, &siteError{Code: "invalid_argument", Message: "未知教材子命令: " + command}
	}
	indexText, match, yes := flagValue(args[1:], "--index"), flagValue(args[1:], "--match"), flagPresent(args[1:], "--yes")
	if !yes {
		return nil, &siteError{Code: "confirmation_required", Message: "远端订退教材会修改账号数据，请加 --yes"}
	}
	if (indexText == "") == (match == "") {
		return nil, &siteError{Code: "invalid_target", Message: "必须且只能提供 --index 或 --match"}
	}
	index := 0
	if indexText != "" {
		parsed, err := strconv.Atoi(indexText)
		if err != nil || parsed < 1 {
			return nil, &siteError{Code: "invalid_target", Message: "--index 必须是正整数"}
		}
		index = parsed
	}
	if strings.TrimSpace(match) == "" && index == 0 {
		return nil, &siteError{Code: "invalid_target", Message: "--match 不能为空"}
	}
	body, pageURL, err := a.findTextbookPage(ctx, textbookPaths[0], true)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	items, dataErr := parseTextbookPage(document, pageURL)
	if dataErr != nil {
		return nil, dataErr
	}
	position, item, selectErr := chooseTextbookPage(items, index, match)
	if selectErr != nil {
		return nil, selectErr
	}
	rows := textbookRowsPage(document)
	if position >= len(rows) {
		return nil, &siteError{Code: "parse_error", Message: "教材条目与页面行数不一致"}
	}
	row := rows[position]
	form := pageFormOwner(row, document)
	node := textbookActionNode(row, command)
	if node == nil {
		return nil, &siteError{Code: "parse_error", Message: "未找到教材操作按钮；为避免误操作，已拒绝提交"}
	}
	method, targetValue := pageActionTarget(node, form, document, pageURL)
	if targetValue == "" {
		return nil, &siteError{Code: "parse_error", Message: "教材操作没有可解析的目标；为避免误操作，已拒绝提交"}
	}
	target, targetErr := validatePageActionTarget(targetValue, pageURL, false)
	if targetErr != nil {
		return nil, targetErr
	}
	if method != "GET" && method != "POST" {
		return nil, &siteError{Code: "invalid_argument", Message: "教材操作不支持 HTTP 方法 " + method}
	}
	fields := textbookFormFields(form, row, node, document)
	submit := siteRequest{Target: target, CookieFile: academicCookiePath(), Method: method, RequireLogin: true, Yes: true, ReadOnly: false}
	if method == "GET" {
		submit.Params = fields
	} else {
		submit.Data = fields
	}
	result, requestErr := a.executeAcademicRequestWithRecovery(ctx, submit)
	if requestErr != nil {
		return nil, requestErr
	}
	message := textbookResponseMessage(result)
	if strings.Contains(strings.ToLower(message), "失败") || strings.Contains(strings.ToLower(message), "错误") || strings.Contains(strings.ToLower(message), "拒绝") {
		return nil, &siteError{Code: "mutation_rejected", Message: "教材" + command + "失败：" + message, Details: map[string]any{"submitted": true, "confirmed": false, "operation": command, "index": position + 1}}
	}
	verified, verifyErr := a.verifyTextbook(ctx, item, command)
	base := map[string]any{"ok": true, "operation": command, "index": position + 1, "item": item, "message": message, "submitted": true, "confirmed": verified, "evidence": "unknown", "verification": "unknown"}
	if verified {
		base["evidence"], base["verification"] = "confirmed", "confirmed"
		return base, nil
	}
	if verifyErr == "" {
		verifyErr = "提交后的教材状态没有显示预期变化"
	}
	return nil, &siteError{Code: "mutation_unverified", Message: "教材操作已提交但未验证：" + verifyErr, Details: base}
}

func (a NativeSite) findTextbookPage(ctx context.Context, preferred string, tryAlternates bool) (string, string, *siteError) {
	paths := make([]string, 0, len(textbookPaths))
	if preferred != "" {
		paths = append(paths, preferred)
	}
	if tryAlternates {
		for _, path := range textbookPaths {
			if path != preferred {
				paths = append(paths, path)
			}
		}
	}
	for _, path := range paths {
		body, pageURL, err := a.academicPage(ctx, "GET", path, nil, nil)
		if err == nil {
			document, parseErr := parsePage(body)
			if parseErr != nil {
				return "", "", &siteError{Code: "parse_error", Message: parseErr.Error()}
			}
			if textbookTablePage(document) != nil || strings.Contains(pageDisplayText(document), "未查询到数据") || strings.Contains(pageDisplayText(document), "暂无教材") {
				return body, pageURL, nil
			}
			continue
		}
		if !tryAlternates || err.Code != "http_error" || !strings.Contains(err.Message, "404") {
			return "", "", err
		}
	}
	return "", "", &siteError{Code: "parse_error", Message: "未找到教材页面；学校可能暂未开放教材确认或已更换页面"}
}

func textbookTablePage(document *pageNode) *pageNode {
	best := (*pageNode)(nil)
	score := 0
	for _, table := range document.findAll("table") {
		value := pageDisplayText(table)
		current := 0
		if table.attr("id") == "dataList" || table.attr("id") == "jczmxx" || table.attr("id") == "jccx" {
			current += 4
		}
		if strings.Contains(value, "教材") || strings.Contains(value, "书名") || strings.Contains(strings.ToUpper(value), "ISBN") || strings.Contains(value, "课程") {
			current += 3
		}
		if regexp.MustCompile(`选订|订购|退订|取消订|增订`).MatchString(value) {
			current += 4
		}
		if len(directTableRows(table)) > 0 {
			current++
		}
		if current > score {
			best, score = table, current
		}
	}
	return best
}
func textbookRowsPage(document *pageNode) []*pageNode {
	table := textbookTablePage(document)
	if table == nil {
		return nil
	}
	result := make([]*pageNode, 0)
	for _, row := range directTableRows(table) {
		cells := directCells(row)
		values := rowValues(row)
		header := len(values) > 0 && (values[0] == "序号" || countTextHeaders(values) >= 2)
		if len(cells) >= 2 && pageDisplayText(row) != "" && !header {
			result = append(result, row)
		}
	}
	return result
}
func countTextHeaders(values []string) int {
	count := 0
	for _, value := range values {
		if containsString([]string{"序号", "序", "课程", "课程名称", "课程代码", "课号", "教材", "书名", "状态", "操作"}, strings.TrimSpace(value)) {
			count++
		}
	}
	return count
}
func textbookHeaders(document *pageNode) []string {
	table := textbookTablePage(document)
	if table == nil {
		return nil
	}
	for _, row := range directTableRows(table) {
		if headers := directCells(row); len(headers) > 0 && headers[0].tag == "th" {
			values := rowValues(row)
			return values
		}
		values := rowValues(row)
		if len(values) > 0 && (values[0] == "序号" || countTextHeaders(values) >= 2) {
			return values
		}
	}
	return nil
}
func textbookField(headers []string, cells []*pageNode, field string) string {
	aliases := map[string][]string{"course": {"课程", "课程名称", "科目", "科目名称"}, "course_id": {"课程代码", "课程编号", "课号", "科目代码", "科目编号"}, "title": {"教材", "书名", "教材名称", "图书", "图书名称"}, "isbn": {"isbn", "书号"}}[field]
	for index, header := range headers {
		for _, alias := range aliases {
			if strings.EqualFold(strings.ReplaceAll(header, " ", ""), alias) && index < len(cells) {
				return pageDisplayText(cells[index])
			}
		}
	}
	return ""
}
func parseTextbookPage(document *pageNode, pageURL string) ([]map[string]any, *siteError) {
	rows := textbookRowsPage(document)
	headers := textbookHeaders(document)
	result := make([]map[string]any, 0)
	for _, row := range rows {
		cells := directCells(row)
		controls := make([]map[string]any, 0)
		for _, node := range append(append(row.findAll("input"), row.findAll("button")...), row.findAll("a")...) {
			if node.tag == "a" && node.attr("href") == "" && node.attr("onclick") == "" {
				continue
			}
			control := map[string]any{"tag": node.tag, "type": firstNonEmpty(node.attr("type"), node.tag), "name": node.attr("name"), "value": pageElementValue(node), "text": pageDisplayText(node), "href": pageSafeValue(node.attr("href"), pageURL), "onclick": pageEvent(node.attr("onclick")), "formaction": pageSafeValue(node.attr("formaction"), pageURL), "formmethod": node.attr("formmethod"), "checked": node.has("checked"), "disabled": node.disabled()}
			controls = append(controls, control)
		}
		actions := textbookRowActions(row)
		values := rowValues(row)
		course, courseID, title, isbn := textbookField(headers, cells, "course"), textbookField(headers, cells, "course_id"), textbookField(headers, cells, "title"), textbookField(headers, cells, "isbn")
		if headers == nil {
			if course == "" {
				course = valueAt(values, 0)
			}
			if title == "" {
				title = valueAt(values, 1)
			}
		}
		status := ""
		if regexp.MustCompile(`已订|已选|已购|订购成功`).MatchString(pageDisplayText(row)) {
			status = "已订"
		} else if regexp.MustCompile(`未订|未选|可订|待订`).MatchString(pageDisplayText(row)) {
			status = "未订"
		} else if len(actions) == 1 {
			status = map[string]string{"subscribe": "未订", "unsubscribe": "已订"}[actions[0]]
		}
		result = append(result, map[string]any{"index": len(result) + 1, "course": course, "course_id": courseID, "title": title, "isbn": isbn, "status": status, "actions": actions, "cells": values, "text": pageDisplayText(row), "controls": controls, "form": pageFormOwner(row, document) != nil})
	}
	return result, nil
}
func textbookRowActions(row *pageNode) []string {
	result := []string{}
	for _, operation := range []string{"subscribe", "unsubscribe"} {
		for _, node := range append(append(row.findAll("input"), row.findAll("button")...), row.findAll("a")...) {
			text := strings.Join([]string{pageDisplayText(node), node.attr("value"), node.attr("onclick"), node.attr("href")}, " ")
			if operation == "subscribe" && regexp.MustCompile(`选订|订购|增订|订教材|购买`).MatchString(text) && !regexp.MustCompile(`退订|取消订|取消选择`).MatchString(text) {
				result = append(result, operation)
				break
			}
			if operation == "unsubscribe" && regexp.MustCompile(`退订|取消订|不订|退购|取消选择`).MatchString(text) {
				result = append(result, operation)
				break
			}
		}
	}
	return result
}
func chooseTextbookPage(items []map[string]any, index int, match string) (int, map[string]any, *siteError) {
	candidates := []int{}
	if index > 0 {
		if index <= len(items) {
			candidates = []int{index - 1}
		}
	} else {
		needle := strings.ToLower(strings.TrimSpace(match))
		for position, item := range items {
			if strings.Contains(strings.ToLower(fmt.Sprint(item["text"])), needle) {
				candidates = append(candidates, position)
			}
		}
	}
	if len(candidates) == 0 {
		return 0, nil, &siteError{Code: "target_not_found", Message: "没有匹配的教材条目"}
	}
	if len(candidates) > 1 {
		return 0, nil, &siteError{Code: "target_ambiguous", Message: "匹配到多个教材条目，请改用 --index"}
	}
	return candidates[0], items[candidates[0]], nil
}
func textbookActionNode(row *pageNode, operation string) *pageNode {
	for _, node := range append(append(row.findAll("input"), row.findAll("button")...), row.findAll("a")...) {
		text := strings.Join([]string{pageDisplayText(node), node.attr("value"), node.attr("onclick"), node.attr("href")}, " ")
		if operation == "subscribe" && regexp.MustCompile(`选订|订购|增订|订教材|购买`).MatchString(text) && !regexp.MustCompile(`退订|取消订|取消选择`).MatchString(text) {
			return node
		}
		if operation == "unsubscribe" && regexp.MustCompile(`退订|取消订|不订|退购|取消选择`).MatchString(text) {
			return node
		}
	}
	return nil
}
func textbookFormFields(form, row, actionNode, document *pageNode) []pair {
	if form == nil {
		return nil
	}
	result := []pair{}
	for _, node := range pageControls(form, document) {
		if node.disabled() {
			continue
		}
		inTarget, inAnyRow := false, false
		for current := node.parent; current != nil; current = current.parent {
			if current == row {
				inTarget = true
			}
			if current.tag == "tr" {
				inAnyRow = true
			}
		}
		if inAnyRow && !inTarget {
			continue
		}
		if node == actionNode {
			if node.attr("name") != "" {
				result = append(result, pair{node.attr("name"), pageElementValue(node)})
			}
			continue
		}
		switch node.tag {
		case "input":
			kind := strings.ToLower(firstNonEmpty(node.attr("type"), "text"))
			if node.attr("name") == "" || kind == "submit" || kind == "button" || kind == "image" || kind == "reset" || kind == "file" || (kind == "checkbox" || kind == "radio") && !node.has("checked") {
				continue
			}
			result = append(result, pair{node.attr("name"), pageElementValue(node)})
		case "select":
			for _, option := range node.findAll("option") {
				if option.has("selected") && !option.disabled() {
					result = append(result, pair{node.attr("name"), pageOptionValue(option)})
				}
			}
		case "textarea":
			if node.attr("name") != "" {
				result = append(result, pair{node.attr("name"), node.rawText()})
			}
		}
	}
	return result
}
func textbookResponseMessage(result map[string]any) string {
	response, _ := result["response"].(map[string]any)
	if message, ok := response["body"].(string); ok {
		if state, known := pageFeedback(message, fmt.Sprint(response["content_type"])); known && state {
			return "操作成功"
		}
		if len(message) > 500 {
			message = message[:500]
		}
		return pageEvent(message)
	}
	return ""
}
func (a NativeSite) verifyTextbook(ctx context.Context, before map[string]any, operation string) (bool, string) {
	body, pageURL, err := a.findTextbookPage(ctx, textbookPaths[0], true)
	if err != nil {
		return false, err.Message
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return false, parseErr.Error()
	}
	items, _ := parseTextbookPage(document, pageURL)
	candidates := []map[string]any{}
	for _, item := range items {
		if sameTextbookItem(before, item) {
			candidates = append(candidates, item)
		}
	}
	if len(candidates) != 1 {
		return false, "提交后的教材条目无法唯一定位"
	}
	item := candidates[0]
	actions, _ := item["actions"].([]string)
	status := fmt.Sprint(item["status"])
	if operation == "subscribe" && (strings.Contains(status, "已订") || containsString(actions, "unsubscribe")) {
		return true, ""
	}
	if operation == "unsubscribe" && (strings.Contains(status, "未订") || containsString(actions, "subscribe")) {
		return true, ""
	}
	return false, "提交后的教材状态没有显示预期变化"
}
func sameTextbookItem(left, right map[string]any) bool {
	leftText, rightText := strings.TrimSpace(fmt.Sprint(left["text"])), strings.TrimSpace(fmt.Sprint(right["text"]))
	if leftText != "" && leftText == rightText {
		return true
	}
	leftISBN, rightISBN := strings.TrimSpace(fmt.Sprint(left["isbn"])), strings.TrimSpace(fmt.Sprint(right["isbn"]))
	if leftISBN != "" || rightISBN != "" {
		return leftISBN != "" && leftISBN == rightISBN
	}
	return fmt.Sprint(left["course"]) != "" && fmt.Sprint(left["title"]) != "" && fmt.Sprint(left["course"]) == fmt.Sprint(right["course"]) && fmt.Sprint(left["title"]) == fmt.Sprint(right["title"])
}
func renderTextbooks(result map[string]any) string {
	if result["operation"] != nil {
		return fmt.Sprint(result["message"]) + "\n"
	}
	if items, ok := result["items"].([]map[string]any); ok {
		var builder strings.Builder
		for _, item := range items {
			builder.WriteString(fmt.Sprintf("[%v] %v %v %v\n", item["index"], item["course"], item["title"], item["status"]))
		}
		return builder.String()
	}
	encoded, _ := json.Marshal(result)
	return string(encoded) + "\n"
}
