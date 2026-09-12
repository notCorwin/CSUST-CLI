package adapter

import (
	"context"
	"net/url"
	"regexp"
	"strings"
)

const (
	academicLabBookingQueryPath     = "/jsxsd/view/syjx/syyy_find.jsp"
	academicOpenLabBookingQueryPath = "/jsxsd/view/syjx/kfsy_find.jsp"
	academicLabBookingListPath      = "/jsxsd/syjx/findSyYyKc.do"
	academicOpenLabBookingListPath  = "/jsxsd/syjx/findSyKfYyKc.do"
	academicOpenLabSelectedPath     = "/jsxsd/syjx/XsyxSyyy.do"
)

var (
	academicLabBookingIDPattern  = regexp.MustCompile(`(?i)\btoyy\s*\(\s*['"]?([^,'")\s]+)`)
	academicOpenLabCancelPattern = regexp.MustCompile(`(?i)\bdoyy\s*\(\s*['"]?([^,'")\s]+)`)
)

func (a NativeSite) academicLabBooking(ctx context.Context, args []string, open bool) (map[string]any, *siteError) {
	operation := "available"
	if len(args) > 0 && !strings.HasPrefix(args[0], "--") {
		operation, args = strings.ToLower(args[0]), args[1:]
	}
	allowed := map[string]bool{"--term": true, "--keyword": true, "--page": true}
	if open {
		if operation != "available" && operation != "list" && operation != "selected" {
			return nil, &siteError{Code: "invalid_argument", Message: "open-lab-booking 只支持 available 或 selected"}
		}
	} else if operation != "available" && operation != "list" {
		return nil, &siteError{Code: "invalid_argument", Message: "lab-booking 只支持 available"}
	}
	if err := academicLabBookingArgs(args, allowed); err != nil {
		return nil, err
	}

	queryPath := academicLabBookingQueryPath
	if open {
		queryPath = academicOpenLabBookingQueryPath
	}
	queryBody, queryURL, err := a.academicPage(ctx, "GET", queryPath, nil, nil)
	if err != nil {
		return nil, err
	}
	queryDocument, parseErr := parsePage(queryBody)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "实验预约学期页面解析失败: " + parseErr.Error()}
	}
	term := strings.TrimSpace(flagValue(args, "--term"))
	if term == "" {
		term = selectedOptionPage(queryDocument, "xnxq")
	}

	body, pageURL := queryBody, queryURL
	if open && operation == "selected" {
		if term != "" {
			query := url.Values{"xnxq": []string{term}}
			body, pageURL, err = a.academicPage(ctx, "GET", academicOpenLabSelectedPath+"?"+query.Encode(), nil, nil)
		} else {
			body, pageURL, err = a.academicPage(ctx, "GET", academicOpenLabSelectedPath, nil, nil)
		}
	} else {
		listPath := academicLabBookingListPath
		if open {
			listPath = academicOpenLabBookingListPath
		}
		data := []pair{{"xnxq", term}}
		if page := strings.TrimSpace(flagValue(args, "--page")); page != "" {
			data = append(data, pair{"pageIndex", page})
		}
		body, pageURL, err = a.academicPage(ctx, "POST", listPath, data, []pair{{"Referer", queryURL}})
	}
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "实验预约列表解析失败: " + parseErr.Error()}
	}
	items := academicLabBookingRows(document, strings.TrimSpace(flagValue(args, "--keyword")), term, open, operation == "selected")
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	resultOperation, resultPath := "available", academicLabBookingListPath
	if open {
		resultPath = academicOpenLabBookingListPath
		if operation == "selected" {
			resultOperation, resultPath = "selected", academicOpenLabSelectedPath
		}
	}
	return academicWrap(map[string]any{
		"kind": "lab-booking", "operation": resultOperation, "path": resultPath,
		"term": nullableString(term), "items": items, "item_count": len(items), "page": page,
	}), nil
}

func academicLabBookingArgs(args []string, allowed map[string]bool) *siteError {
	for index := 0; index < len(args); index++ {
		arg, _, inline := splitInline(args[index])
		if arg == "--json" {
			continue
		}
		if !strings.HasPrefix(arg, "--") || !allowed[arg] {
			return &siteError{Code: "invalid_argument", Message: "实验预约参数无效: " + arg}
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

func academicLabBookingRows(document *pageNode, keyword, term string, open, selected bool) []map[string]any {
	table := academicLabBookingTable(document, open, selected)
	if table == nil {
		return []map[string]any{}
	}
	rows := directTableRows(table)
	if len(rows) == 0 {
		return []map[string]any{}
	}
	header := rowValues(rows[0])
	headerCount := 0
	for _, title := range header {
		if academicLabBookingField(title) != "" {
			headerCount++
		}
	}
	headerRow := len(directCells(rows[0])) > 0 && (directCells(rows[0])[0].tag == "th" || headerCount > 0)
	if headerRow {
		rows = rows[1:]
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		values := rowValues(row)
		text := strings.TrimSpace(pageDisplayText(row))
		if len(values) == 0 || allEmpty(values) || academicNoDataRow(text) || (keyword != "" && !strings.Contains(strings.ToLower(text), strings.ToLower(keyword))) {
			continue
		}
		item := map[string]any{"index": len(items) + 1, "cells": values, "text": text}
		if headerRow {
			for index, title := range header {
				if name := academicLabBookingField(title); name != "" && index < len(values) {
					item[name] = values[index]
				}
			}
		}
		for _, node := range row.findAll("") {
			source := node.attr("onclick")
			if selected {
				if match := academicOpenLabCancelPattern.FindStringSubmatch(source); len(match) > 1 {
					id := strings.TrimSpace(match[1])
					item["selection_id"] = id
					item["cancel_path"] = academicOpenLabCancelPath(id, term)
					break
				}
				continue
			}
			if match := academicLabBookingIDPattern.FindStringSubmatch(source); len(match) > 1 {
				id := strings.TrimSpace(match[1])
				item["booking_id"] = id
				if open {
					item["booking_path"] = "/jsxsd/syjx/toKfsyyySelect.do?sj0404id=" + url.QueryEscape(id) + "&xnxq=" + url.QueryEscape(term)
				} else {
					item["booking_path"] = "/jsxsd/syjx/toSyYy.do?sj0403id=" + url.QueryEscape(id) + "&xnxq01id="
				}
				break
			}
		}
		items = append(items, item)
	}
	return items
}

func academicOpenLabCancelPath(id, term string) string {
	return "/jsxsd/syjx/toQxXm.do?sj0501id=" + url.QueryEscape(id) + "&xnxq=" + url.QueryEscape(term)
}

func academicLabBookingTable(document *pageNode, open, selected bool) *pageNode {
	var best *pageNode
	score := 0
	for _, table := range document.findAll("table") {
		text := pageDisplayText(table)
		current := len(directTableRows(table))
		if (open && strings.Contains(text, "项目")) || (!open && strings.Contains(text, "课程")) {
			current += 10
		}
		if selected && strings.Contains(text, "批次") {
			current += 5
		}
		if current > score {
			best, score = table, current
		}
	}
	return best
}

func academicLabBookingField(value string) string {
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	if courseField := academicCourseField(value); courseField != "" {
		return courseField
	}
	switch {
	case strings.Contains(value, "项目编号"):
		return "project_id"
	case strings.Contains(value, "项目名称"):
		return "project"
	case strings.Contains(value, "项目类型"):
		return "project_type"
	case strings.Contains(value, "承担实验室"):
		return "laboratory"
	case strings.Contains(value, "计划学时"):
		return "planned_hours"
	case strings.Contains(value, "容量"):
		return "capacity"
	case strings.Contains(value, "余量"):
		return "remaining"
	case strings.Contains(value, "上课时间"):
		return "class_time"
	case strings.Contains(value, "周次节次"):
		return "schedule"
	case strings.Contains(value, "项目性质"):
		return "project_nature"
	case strings.Contains(value, "批次名称"):
		return "batch_name"
	case value == "批次":
		return "batch"
	case strings.Contains(value, "地点"):
		return "location"
	case value == "操作":
		return "action"
	default:
		return ""
	}
}
