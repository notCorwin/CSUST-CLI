package adapter

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const (
	academicOnlineQAPath     = "/jsxsd/zxwd/zxwd_opt"
	academicOnlineQAAddPath  = "/jsxsd/zxwd/tw_add"
	academicOnlineQASavePath = "/jsxsd/zxwd/tw_add_save"
)

var academicOnlineQAIDPattern = regexp.MustCompile(`(?i)\bsc\s*\(\s*['"]?([^,'")\s]+)`)

func (a NativeSite) academicOnlineQA(ctx context.Context, args []string) (map[string]any, *siteError) {
	operation := "list"
	if len(args) > 0 && !strings.HasPrefix(args[0], "--") {
		operation, args = strings.ToLower(args[0]), args[1:]
	}
	switch operation {
	case "list", "show":
		return a.academicOnlineQAList(ctx, args)
	case "ask", "create":
		return a.academicOnlineQAAsk(ctx, args)
	case "delete", "remove":
		return a.academicOnlineQADelete(ctx, args)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "online-qa 只支持 list、ask 或 delete"}
	}
}

func (a NativeSite) academicOnlineQAList(ctx context.Context, args []string) (map[string]any, *siteError) {
	if err := academicOnlineQAArgs(args, map[string]bool{"--keyword": true}); err != nil {
		return nil, err
	}
	body, pageURL, err := a.academicPage(ctx, "GET", academicOnlineQAPath, nil, nil)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	items := academicOnlineQARows(document, strings.TrimSpace(flagValue(args, "--keyword")), pageURL)
	if len(items) == 0 && !noAcademicData(document) && len(document.findAll("table")) == 0 {
		return nil, &siteError{Code: "parse_error", Message: "在线问答页面未包含可解析表格；当前响应结构尚未被适配器识别"}
	}
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	return academicWrap(map[string]any{
		"kind": "online-qa", "operation": "list", "path": academicOnlineQAPath,
		"keyword": nullableString(strings.TrimSpace(flagValue(args, "--keyword"))),
		"items":   items, "item_count": len(items), "page": page,
	}), nil
}

func (a NativeSite) academicOnlineQAAsk(ctx context.Context, args []string) (map[string]any, *siteError) {
	if err := academicOnlineQAArgs(args, map[string]bool{"--content": true, "--yes": false}); err != nil {
		return nil, err
	}
	content := flagValue(args, "--content")
	if strings.TrimSpace(content) == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "online-qa ask 必须提供 --content"}
	}
	if len([]rune(content)) > 2000 {
		return nil, &siteError{Code: "invalid_argument", Message: "--content 不能超过 2000 个字符"}
	}
	if !flagPresent(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "提交在线问答会修改账号数据，请加 --yes"}
	}

	body, pageURL, err := a.academicPage(ctx, "GET", academicOnlineQAAddPath, nil, nil)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	form := academicOnlineQAForm(document)
	if form == nil {
		return nil, &siteError{Code: "parse_error", Message: "未找到在线问答提问表单"}
	}
	action := firstNonEmpty(form.attr("action"), academicOnlineQASavePath)
	target, targetErr := validatePageActionTarget(action, pageURL, false)
	if targetErr != nil {
		return nil, targetErr
	}
	actionPath := target.EscapedPath()
	if target.RawQuery != "" {
		actionPath += "?" + target.RawQuery
	}
	data := pageFormFields(form, nil, document)
	foundContent := false
	for index := range data {
		if data[index].name == "xmms" {
			data[index].value = content
			foundContent = true
		}
	}
	if !foundContent {
		return nil, &siteError{Code: "parse_error", Message: "提问表单未包含 xmms 内容字段"}
	}
	result, submitErr := a.executeAcademicRequestWithRecovery(ctx, siteRequest{
		Service: "academic", Path: actionPath, Method: "POST", CookieFile: academicCookiePath(), Data: data,
		Headers: []pair{{"Referer", pageURL}}, RequireLogin: true, Yes: true,
	})
	if submitErr != nil {
		return nil, submitErr
	}
	items, _, readbackErr := a.academicOnlineQARowsFromService(ctx, "")
	if readbackErr != nil {
		return nil, &siteError{Code: "mutation_unverified", Message: "在线问答提交成功反馈已返回，但列表回读失败", Details: map[string]any{
			"submitted": true, "confirmed": false, "evidence": "server-success", "cause": readbackErr.Code,
		}}
	}
	var created map[string]any
	for _, item := range items {
		if strings.Contains(fmt.Sprint(item["content"]), content) {
			created = item
			break
		}
	}
	if created == nil {
		return nil, &siteError{Code: "mutation_unverified", Message: "在线问答提交成功反馈已返回，但列表未回读到问题内容", Details: map[string]any{
			"submitted": true, "confirmed": false, "evidence": "server-success-and-list-readback", "content_length": len([]rune(content)),
		}}
	}
	return academicWrap(map[string]any{
		"kind": "online-qa", "operation": "ask", "path": academicOnlineQASavePath,
		"content_length": len([]rune(content)), "item": created, "readback_path": academicOnlineQAPath,
		"request":  map[string]any{"method": "POST", "path": actionPath, "fields": fieldNames(data)},
		"response": result["response"], "submitted": true, "confirmed": true,
		"evidence": "server-success-and-list-readback",
	}), nil
}

func (a NativeSite) academicOnlineQADelete(ctx context.Context, args []string) (map[string]any, *siteError) {
	if err := academicOnlineQAArgs(args, map[string]bool{"--id": true, "--yes": false}); err != nil {
		return nil, err
	}
	id := strings.TrimSpace(flagValue(args, "--id"))
	if id == "" || strings.ContainsAny(id, "/?#&'\"") {
		return nil, &siteError{Code: "invalid_argument", Message: "online-qa delete 必须提供不含路径的 --id"}
	}
	if !flagPresent(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "删除在线问答会修改账号数据，请加 --yes"}
	}
	items, pageURL, listErr := a.academicOnlineQARowsFromService(ctx, "")
	if listErr != nil {
		return nil, listErr
	}
	found := false
	for _, item := range items {
		if fmt.Sprint(item["id"]) == id {
			found = true
			break
		}
	}
	if !found {
		return nil, &siteError{Code: "not_found", Message: "在线问答记录不存在: " + id}
	}
	result, submitErr := a.executeAcademicRequestWithRecovery(ctx, siteRequest{
		Service: "academic", Path: academicOnlineQAPath, Method: "POST", CookieFile: academicCookiePath(),
		Data: []pair{{"oaid", id}}, Headers: []pair{{"Referer", pageURL}}, RequireLogin: true, Yes: true,
	})
	if submitErr != nil {
		return nil, submitErr
	}
	remaining, _, readbackErr := a.academicOnlineQARowsFromService(ctx, "")
	if readbackErr != nil {
		return nil, &siteError{Code: "mutation_unverified", Message: "在线问答删除成功反馈已返回，但列表回读失败", Details: map[string]any{
			"submitted": true, "confirmed": false, "evidence": "server-success", "id": id, "cause": readbackErr.Code,
		}}
	}
	for _, item := range remaining {
		if fmt.Sprint(item["id"]) == id {
			return nil, &siteError{Code: "mutation_unverified", Message: "在线问答删除成功反馈已返回，但记录仍可回读", Details: map[string]any{
				"submitted": true, "confirmed": false, "evidence": "server-success-and-list-readback", "id": id,
			}}
		}
	}
	return academicWrap(map[string]any{
		"kind": "online-qa", "operation": "delete", "path": academicOnlineQAPath,
		"id": id, "request": map[string]any{"method": "POST", "path": academicOnlineQAPath, "fields": []string{"oaid"}},
		"response": result["response"], "submitted": true, "confirmed": true,
		"evidence": "server-success-and-list-readback",
	}), nil
}

func academicOnlineQAArgs(args []string, allowed map[string]bool) *siteError {
	for index := 0; index < len(args); index++ {
		arg, _, inline := splitInline(args[index])
		if arg == "--yes" {
			if inline {
				return &siteError{Code: "invalid_argument", Message: "--yes 不接受 =VALUE"}
			}
			continue
		}
		if !strings.HasPrefix(arg, "--") || !allowed[arg] {
			return &siteError{Code: "invalid_argument", Message: "online-qa 参数无效: " + arg}
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

func (a NativeSite) academicOnlineQARowsFromService(ctx context.Context, keyword string) ([]map[string]any, string, *siteError) {
	body, pageURL, err := a.academicPage(ctx, "GET", academicOnlineQAPath, nil, nil)
	if err != nil {
		return nil, "", err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, "", &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	return academicOnlineQARows(document, keyword, pageURL), pageURL, nil
}

func academicOnlineQAForm(document *pageNode) *pageNode {
	for _, form := range document.findAll("form") {
		action := form.attr("action")
		if strings.HasSuffix(strings.TrimSpace(action), academicOnlineQASavePath) {
			for _, field := range form.findAll("textarea") {
				if field.attr("name") == "xmms" {
					return form
				}
			}
		}
	}
	return nil
}

func academicOnlineQARows(document *pageNode, keyword, pageURL string) []map[string]any {
	var table *pageNode
	score := 0
	for _, candidate := range document.findAll("table") {
		text := pageDisplayText(candidate)
		current := len(directTableRows(candidate))
		if strings.Contains(text, "问题内容") {
			current += 10
		}
		if current > score {
			table, score = candidate, current
		}
	}
	if table == nil {
		return []map[string]any{}
	}
	rows := directTableRows(table)
	if len(rows) == 0 {
		return []map[string]any{}
	}
	header := rowValues(rows[0])
	hasHeader := len(directCells(rows[0])) > 0 && (directCells(rows[0])[0].tag == "th" || onlineQAHeaderCount(header) > 0)
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
				if field := academicOnlineQAField(title); field != "" && index < len(values) {
					item[field] = values[index]
				}
			}
		}
		if id := academicOnlineQAID(row); id != "" {
			item["id"] = id
		}
		if pageURL != "" {
			for _, link := range row.findAll("a") {
				if target := academicLinkTarget(link, pageURL); target != "" {
					item["detail_path"] = academicPathValue(target)
					break
				}
			}
		}
		items = append(items, item)
	}
	return items
}

func academicOnlineQAID(row *pageNode) string {
	for _, node := range row.findAll("") {
		for _, source := range []string{node.attr("onclick"), node.attr("href")} {
			if match := academicOnlineQAIDPattern.FindStringSubmatch(source); len(match) > 1 {
				return strings.TrimSpace(match[1])
			}
		}
	}
	return ""
}

func onlineQAHeaderCount(values []string) int {
	count := 0
	for _, value := range values {
		if academicOnlineQAField(value) != "" {
			count++
		}
	}
	return count
}

func academicOnlineQAField(value string) string {
	value = strings.Join(strings.Fields(value), "")
	switch {
	case strings.Contains(value, "发送人"):
		return "sender"
	case strings.Contains(value, "发送单位"):
		return "sender_unit"
	case strings.Contains(value, "发送类型"):
		return "type"
	case strings.Contains(value, "发送时间"):
		return "sent_at"
	case strings.Contains(value, "问题内容"):
		return "content"
	case strings.Contains(value, "热点问题"):
		return "hot"
	case strings.Contains(value, "是否回复"):
		return "replied"
	case strings.Contains(value, "有效时间"):
		return "valid_from"
	case strings.Contains(value, "失效时间"):
		return "expires_at"
	case value == "操作":
		return "actions"
	default:
		return ""
	}
}

func academicPathValue(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return value
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	if parsed.RawQuery != "" {
		path += "?" + parsed.RawQuery
	}
	return path
}
