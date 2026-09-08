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

var academicDayNames = []string{"星期一", "星期二", "星期三", "星期四", "星期五", "星期六", "星期日"}

func (a NativeSite) runAcademicCommand(ctx context.Context, args []string, jsonMode bool) (bool, []byte, []byte, int, error) {
	if len(args) == 0 || !academicCommand(args[0]) || containsHelp(args[1:]) {
		return false, nil, nil, 0, nil
	}
	result, err := a.executeAcademic(ctx, args)
	if err != nil {
		if jsonMode {
			return true, errorJSON(err), nil, 2, nil
		}
		return true, nil, []byte("错误: " + err.Error() + "\n"), 2, nil
	}
	if jsonMode {
		return true, mustJSON(result), nil, 0, nil
	}
	return true, []byte(renderAcademic(result)), nil, 0, nil
}

func academicCommand(value string) bool {
	switch value {
	case "schedule", "timetable", "grades", "scores", "profile", "personal", "exams", "exam", "classrooms", "rooms", "selections", "selection", "course-results", "terms", "semesters", "semester-start", "course-selection", "course-select", "classroom-request", "room-request", "minor", "minor-registration", "evaluation", "evaluate":
		return true
	default:
		return false
	}
}

func (a NativeSite) executeAcademic(ctx context.Context, args []string) (map[string]any, *siteError) {
	switch args[0] {
	case "schedule", "timetable":
		return a.academicSchedule(ctx, args[1:])
	case "grades", "scores":
		return a.academicGrades(ctx, args[1:])
	case "profile", "personal":
		body, pageURL, err := a.academicPage(ctx, "GET", "/jsxsd/grxx/xsxx", nil, nil)
		if err != nil {
			return nil, err
		}
		return academicWrap(parseProfilePage(body, pageURL)), nil
	case "exams", "exam":
		return a.academicExams(ctx, args[1:])
	case "classrooms", "rooms":
		return a.academicClassrooms(ctx, args[1:])
	case "selections", "selection", "course-results":
		return a.academicSelections(ctx, args[1:])
	case "terms", "semesters":
		return a.academicTerms(ctx, args[1:])
	case "semester-start":
		return a.academicSemesterStart(ctx, args[1:])
	case "course-selection", "course-select":
		return a.academicPageSnapshot(ctx, "/jsxsd/xsxk/xklc_list")
	case "classroom-request", "room-request":
		return a.academicPageSnapshot(ctx, "/jsxsd/kbxx/jsjy_query")
	case "minor", "minor-registration":
		return a.academicPageSnapshot(ctx, "/jsxsd/fxgl/fxbmxx_query")
	case "evaluation", "evaluate":
		return a.runAcademicEvaluation(ctx, args[1:])
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "未知教务命令"}
	}
}

func (a NativeSite) academicPageSnapshot(ctx context.Context, path string) (map[string]any, *siteError) {
	body, pageURL, err := a.academicPage(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	return academicWrap(map[string]any{"page": page}), nil
}

func (a NativeSite) academicPage(ctx context.Context, method, path string, data []pair, headers []pair) (string, string, *siteError) {
	request := siteRequest{Service: "academic", Path: path, Method: method, CookieFile: academicCookiePath(), Data: data, Headers: headers, RequireLogin: true, Yes: true, ReadOnly: true}
	if method == "GET" {
		request.Data = nil
	}
	result, err := a.executeAcademicRequestWithRecovery(ctx, request)
	if err != nil {
		return "", "", err
	}
	response, ok := result["response"].(map[string]any)
	if !ok {
		return "", "", &siteError{Code: "parse_error", Message: "教务响应不是页面或 JSON"}
	}
	body, _ := response["body"].(string)
	pageURL, _ := response["url"].(string)
	if pageURL == "" {
		target, _, resolveErr := resolveSite(request)
		if resolveErr != nil {
			return "", "", resolveErr
		}
		pageURL = target.String()
	}
	return body, pageURL, nil
}

func (a NativeSite) academicSchedule(ctx context.Context, args []string) (map[string]any, *siteError) {
	term := flagValue(args, "--term")
	weekText := flagValue(args, "--week")
	week := 0
	if weekText != "" {
		parsed, err := strconv.Atoi(weekText)
		if err != nil || parsed < 1 {
			return nil, &siteError{Code: "invalid_argument", Message: "--week 必须是正整数"}
		}
		week = parsed
	}
	schemeID := flagValue(args, "--scheme-id")
	data := []pair{{"jx0404id", ""}, {"cj0701id", ""}, {"zc", strconv.Itoa(week)}, {"demo", ""}, {"xnxq01id", term}, {"sfFD", "1"}, {"kbjcmsid", schemeID}}
	body, pageURL, err := a.academicPage(ctx, "POST", "/jsxsd/xskb/xskb_list.do", data, nil)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	if term == "" {
		term = selectedOptionPage(document, "xnxq01id")
	}
	items, dataErr := parseSchedulePage(document, term)
	if dataErr != nil {
		return nil, dataErr
	}
	if week > 0 {
		filtered := make([]map[string]any, 0)
		for _, item := range items {
			if containsInt(item["weeks"], week) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	return academicWrap(map[string]any{"term": term, "week": nullableInt(week), "items": items, "url": safeSiteURL(mustParseURL(pageURL))}), nil
}

func (a NativeSite) academicGrades(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) > 0 && args[0] == "detail" {
		path := flagValue(args[1:], "--path")
		if path == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "grades detail 必须提供 --path"}
		}
		body, pageURL, err := a.academicPage(ctx, "GET", path, nil, nil)
		if err != nil {
			return nil, err
		}
		document, parseErr := parsePage(body)
		if parseErr != nil {
			return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
		}
		detail, detailErr := parseGradeDetailPage(document, pageURL)
		if detailErr != nil {
			return nil, detailErr
		}
		return academicWrap(detail), nil
	}
	term, nature, course, display, study := flagValue(args, "--term"), flagValue(args, "--course-nature"), flagValue(args, "--course-name"), flagValue(args, "--display"), flagValue(args, "--study-mode-id")
	if study == "" {
		study = "2"
	}
	if display == "" {
		display = "all"
	}
	if display != "all" && display != "best" {
		return nil, &siteError{Code: "invalid_argument", Message: "--display 只能是 all 或 best"}
	}
	data := []pair{{"kksj", term}, {"kcxz", nature}, {"kcmc", course}, {"xsfs", map[bool]string{true: "max", false: "all"}[display == "best"]}, {"fxkc", study}}
	body, pageURL, err := a.academicPage(ctx, "POST", "/jsxsd/kscj/cjcx_list", data, []pair{{"Referer", "http://xk.csust.edu.cn/jsxsd/kscj/cjcx_query"}})
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	items, dataErr := parseGradesPage(document, pageURL)
	if dataErr != nil {
		return nil, dataErr
	}
	return academicWrap(map[string]any{"term": nullableString(term), "items": items}), nil
}

func (a NativeSite) academicExams(ctx context.Context, args []string) (map[string]any, *siteError) {
	term := flagValue(args, "--term")
	queryBody, queryURL, err := a.academicPage(ctx, "GET", "/jsxsd/xsks/xsksap_query", nil, nil)
	if err != nil {
		return nil, err
	}
	queryDoc, parseErr := parsePage(queryBody)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	if term == "" {
		term = selectedOptionPage(queryDoc, "xnxqid")
	}
	data := []pair{{"xqlbmc", flagValue(args, "--category")}, {"xnxqid", term}, {"xqlb", flagValue(args, "--category-id")}}
	body, pageURL, err := a.academicPage(ctx, "POST", "/jsxsd/xsks/xsksap_list", data, []pair{{"Referer", queryURL}})
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	items, dataErr := parseExamPage(document, pageURL)
	if dataErr != nil {
		return nil, dataErr
	}
	return academicWrap(map[string]any{"term": term, "items": items, "url": safeSiteURL(mustParseURL(pageURL))}), nil
}

func (a NativeSite) academicClassrooms(ctx context.Context, args []string) (map[string]any, *siteError) {
	campus := strings.ToLower(flagValue(args, "--campus"))
	campusID := map[string]string{"yuntang": "1", "云塘": "1", "1": "1", "jinpenling": "2", "金盆岭": "2", "2": "2"}[campus]
	if campusID == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "--campus 只能是 yuntang、jinpenling、1 或 2"}
	}
	week, weekday, section := flagInt(args, "--week"), flagInt(args, "--weekday"), flagInt(args, "--section")
	if week < 1 || weekday < 1 || weekday > 7 || section < 1 || section > 5 {
		return nil, &siteError{Code: "invalid_argument", Message: "--week、--weekday、--section 参数范围无效"}
	}
	sectionStart := []string{"", "01", "03", "05", "07", "09"}[section]
	sectionEnd := []string{"", "02", "04", "06", "08", "10"}[section]
	data := []pair{{"xnxqh", flagValue(args, "--term")}, {"skyx", flagValue(args, "--department")}, {"xqid", campusID}, {"jzwid", flagValue(args, "--building")}, {"gnq", flagValue(args, "--area")}, {"skjsid", ""}, {"skjs", ""}, {"zc1", strconv.Itoa(week)}, {"zc2", strconv.Itoa(week)}, {"skxq1", strconv.Itoa(weekday)}, {"skxq2", strconv.Itoa(weekday)}, {"jc1", sectionStart}, {"jc2", sectionEnd}}
	body, pageURL, err := a.academicPage(ctx, "POST", "/jsxsd/kbcx/kbxx_classroom_ifr", data, nil)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	items, dataErr := parseClassroomPage(document, pageURL)
	if dataErr != nil {
		return nil, dataErr
	}
	return academicWrap(map[string]any{"campus": campusID, "week": week, "weekday": weekday, "section": section, "items": items}), nil
}

func (a NativeSite) academicSelections(ctx context.Context, args []string) (map[string]any, *siteError) {
	term := flagValue(args, "--term")
	queryBody, queryURL, err := a.academicPage(ctx, "GET", "/jsxsd/xkgl/xsxkjgcx", nil, nil)
	if err != nil {
		return nil, err
	}
	queryDoc, parseErr := parsePage(queryBody)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	if term == "" {
		term = selectedOptionPage(queryDoc, "xnxqid")
	}
	body, pageURL, err := a.academicPage(ctx, "POST", "/jsxsd/xkgl/loadXsxkjgList", []pair{{"xnxqid", term}}, []pair{{"Referer", queryURL}})
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	items, dataErr := parseSelectionPage(document, pageURL)
	if dataErr != nil {
		return nil, dataErr
	}
	return academicWrap(map[string]any{"term": term, "items": items}), nil
}

func (a NativeSite) academicTerms(ctx context.Context, args []string) (map[string]any, *siteError) {
	scope := flagValue(args, "--scope")
	paths := map[string][2]string{"schedule": {"/jsxsd/xskb/xskb_list.do", "xnxq01id"}, "grades": {"/jsxsd/kscj/cjcx_query", "kksj"}, "exams": {"/jsxsd/xsks/xsksap_query", "xnxqid"}, "selection": {"/jsxsd/xkgl/xsxkjgcx", "xnxqid"}, "semester-start": {"/jsxsd/jxzl/jxzl_query", "xnxq01id"}}
	item, ok := paths[scope]
	if !ok {
		return nil, &siteError{Code: "invalid_argument", Message: "--scope 无效"}
	}
	body, pageURL, err := a.academicPage(ctx, "GET", item[0], nil, nil)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	options := pageOptions(document, item[1])
	return academicWrap(map[string]any{"scope": scope, "url": pageURL, "terms": options, "selected": selectedOptionPage(document, item[1])}), nil
}

func (a NativeSite) academicSemesterStart(ctx context.Context, args []string) (map[string]any, *siteError) {
	term := flagValue(args, "--term")
	queryBody, queryURL, err := a.academicPage(ctx, "GET", "/jsxsd/jxzl/jxzl_query", nil, nil)
	if err != nil {
		return nil, err
	}
	queryDoc, parseErr := parsePage(queryBody)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	if term == "" {
		term = selectedOptionPage(queryDoc, "xnxq01id")
	}
	body, pageURL, err := a.academicPage(ctx, "POST", "/jsxsd/jxzl/jxzl_query", []pair{{"xnxq01id", term}}, []pair{{"Referer", queryURL}})
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	result, dataErr := parseSemesterStartPage(document, pageURL)
	if dataErr != nil {
		return nil, dataErr
	}
	result["term"] = term
	return academicWrap(result), nil
}

func academicWrap(value map[string]any) map[string]any {
	result := map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed"}
	for key, item := range value {
		result[key] = item
	}
	return result
}
func mustJSON(value map[string]any) []byte {
	encoded, _ := jsonMarshal(value)
	return append(encoded, '\n')
}
func jsonMarshal(value any) ([]byte, error) { return json.Marshal(value) }
func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}
func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func flagValue(args []string, flag string) string {
	for index, value := range args {
		if value == flag && index+1 < len(args) {
			return args[index+1]
		}
		if strings.HasPrefix(value, flag+"=") {
			return strings.TrimPrefix(value, flag+"=")
		}
	}
	return ""
}
func flagInt(args []string, flag string) int {
	value, _ := strconv.Atoi(flagValue(args, flag))
	return value
}
func containsInt(value any, wanted int) bool {
	values, ok := value.([]int)
	if !ok {
		if list, ok := value.([]any); ok {
			for _, item := range list {
				if number, ok := item.(int); ok && number == wanted {
					return true
				}
			}
		}
		return false
	}
	for _, item := range values {
		if item == wanted {
			return true
		}
	}
	return false
}

func directTableRows(table *pageNode) []*pageNode {
	if table == nil {
		return nil
	}
	result := make([]*pageNode, 0)
	for _, row := range table.findAll("tr") {
		if row.parent != nil && row.parent.tag == "tr" {
			continue
		}
		if len(directCells(row)) > 0 {
			result = append(result, row)
		}
	}
	return result
}
func directCells(row *pageNode) []*pageNode {
	result := make([]*pageNode, 0)
	if row == nil {
		return result
	}
	for _, child := range row.children {
		if child.tag == "td" || child.tag == "th" {
			result = append(result, child)
		}
	}
	return result
}
func academicTable(document *pageNode, ids ...string) *pageNode {
	for _, id := range ids {
		if table := document.first("table", id); table != nil {
			return table
		}
	}
	tables := document.findAll("table")
	if len(tables) == 1 {
		return tables[0]
	}
	for _, table := range tables {
		if len(directTableRows(table)) > 0 {
			return table
		}
	}
	return nil
}
func rowValues(row *pageNode) []string {
	values := make([]string, 0)
	for _, cell := range directCells(row) {
		values = append(values, pageDisplayText(cell))
	}
	return values
}
func noAcademicData(document *pageNode) bool {
	return strings.Contains(pageDisplayText(document), "未查询到数据")
}
func pageOptions(document *pageNode, id string) []map[string]any {
	selectNode := document.first("select", id)
	if selectNode == nil {
		for _, node := range document.findAll("select") {
			if node.attr("name") == id {
				selectNode = node
				break
			}
		}
	}
	if selectNode == nil || selectNode.disabled() {
		return []map[string]any{}
	}
	result := make([]map[string]any, 0)
	for _, option := range selectNode.findAll("option") {
		if pageDisplayText(option) == "" && option.attr("value") == "" {
			continue
		}
		result = append(result, map[string]any{"value": pageOptionValue(option), "label": pageDisplayText(option), "selected": option.has("selected"), "disabled": option.disabled()})
	}
	return result
}
func selectedOptionPage(document *pageNode, id string) string {
	for _, item := range pageOptions(document, id) {
		if item["selected"] == true {
			if value, ok := item["value"].(string); ok {
				return strings.TrimSpace(value)
			}
		}
	}
	options := pageOptions(document, id)
	for _, item := range options {
		if item["disabled"] != true {
			if value, ok := item["value"].(string); ok {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func parseProfilePage(source, pageURL string) map[string]any {
	document, _ := parsePage(source)
	table := academicTable(document, "xjkpTable", "xjxxTable")
	rows := make([][]string, 0)
	fields := make([]map[string]string, 0)
	values := map[string]string{}
	if table == nil {
		return map[string]any{"url": pageURL, "fields": fields, "values": values, "rows": rows}
	}
	for _, row := range directTableRows(table) {
		items := rowValues(row)
		rows = append(rows, items)
		for _, value := range items {
			if name, item, ok := colonField(value); ok {
				fields = append(fields, map[string]string{"name": name, "value": item})
				values[name] = item
			}
		}
		for index := 0; index+1 < len(items); index += 2 {
			label := strings.Trim(strings.TrimSpace(items[index]), "：:")
			if label != "" && len(label) <= 40 && !strings.ContainsAny(label, "：:") && strings.TrimSpace(items[index+1]) != "" {
				fields = append(fields, map[string]string{"name": label, "value": strings.TrimSpace(items[index+1])})
				values[label] = strings.TrimSpace(items[index+1])
			}
		}
	}
	return map[string]any{"url": safeSiteURL(mustParseURL(pageURL)), "fields": fields, "values": values, "rows": rows}
}
func colonField(value string) (string, string, bool) {
	index := strings.IndexAny(value, "：:")
	if index < 1 {
		return "", "", false
	}
	name, item := strings.TrimSpace(value[:index]), strings.TrimSpace(value[index+1:])
	if name == "" {
		return "", "", false
	}
	return name, item, true
}

func parseSchedulePage(document *pageNode, term string) ([]map[string]any, *siteError) {
	table := academicTable(document, "kbtable")
	if table == nil {
		if noAcademicData(document) {
			return []map[string]any{}, nil
		}
		return nil, &siteError{Code: "parse_error", Message: "未找到课表"}
	}
	rows := directTableRows(table)
	days := append([]string(nil), academicDayNames...)
	if len(rows) > 0 {
		found := make([]string, 0)
		for _, cell := range directCells(rows[0]) {
			if containsString(academicDayNames, pageDisplayText(cell)) {
				found = append(found, pageDisplayText(cell))
			}
		}
		if len(found) == 7 {
			days = found
		}
	}
	result := make([]map[string]any, 0)
	seen := map[string]bool{}
	for _, row := range rows[1:] {
		cells := directCells(row)
		if len(cells) == len(days)+1 && len(cells[0].findAll("div")) == 0 {
			cells = cells[1:]
		}
		sectionText := ""
		for _, header := range directCells(row) {
			if header.tag == "th" {
				sectionText = pageDisplayText(header)
				break
			}
		}
		for day, cell := range cells {
			if day >= len(days) {
				break
			}
			for _, item := range parseScheduleCellPage(cell, sectionText) {
				item["term"] = term
				item["weekday"] = days[day]
				key := fmt.Sprintf("%v|%v|%v|%v|%v|%v", item["course"], item["teacher"], item["room"], item["weekday"], item["weeks"], item["sections"])
				if !seen[key] {
					seen[key] = true
					result = append(result, item)
				}
			}
		}
	}
	return result, nil
}
func parseScheduleCellPage(cell *pageNode, fallback string) []map[string]any {
	result := make([]map[string]any, 0)
	for _, content := range cell.findAll("div") {
		class := " " + content.attr("class") + " "
		if !strings.Contains(class, " kbcontent ") && !strings.Contains(class, " kbcontent1 ") {
			continue
		}
		lines := pageBlockLines(content)
		if len(lines) == 0 {
			continue
		}
		teacher, room, spec := "", "", ""
		for _, node := range content.findAll("font") {
			switch node.attr("title") {
			case "老师":
				teacher = pageDisplayText(node)
			case "教室":
				room = pageDisplayText(node)
			case "周次(节次)":
				spec = pageDisplayText(node)
			}
		}
		weeks, sections := parseWeekSpecPage(spec, fallback)
		if len(weeks) == 0 {
			continue
		}
		result = append(result, map[string]any{"course": lines[0], "teacher": teacher, "room": room, "weeks": weeks, "sections": sections})
	}
	return result
}
func pageBlockLines(node *pageNode) []string {
	lines := make([]string, 0)
	var current strings.Builder
	var visit func(*pageNode)
	flush := func() {
		if value := strings.TrimSpace(strings.ReplaceAll(current.String(), "\u00a0", " ")); value != "" {
			lines = append(lines, value)
		}
		current.Reset()
	}
	visit = func(item *pageNode) {
		if item.tag == "#text" {
			current.WriteString(item.text)
			return
		}
		if item.tag == "script" || item.tag == "style" {
			return
		}
		if item.tag == "br" {
			flush()
			return
		}
		for _, child := range item.children {
			visit(child)
		}
		if item.tag == "div" || item.tag == "p" {
			flush()
		}
	}
	visit(node)
	flush()
	return lines
}
func parseWeekSpecPage(value, fallback string) ([]int, []int) {
	value = strings.ReplaceAll(strings.ReplaceAll(value, " ", ""), "，", ",")
	match := regexp.MustCompile(`(.+?)[(（](单周|双周|周)[)）](?:\[([^]]*)\])?`).FindStringSubmatch(value)
	if len(match) == 0 {
		return []int{}, parseIntsPage(fallback)
	}
	weeks := expandWeekRangePage(match[1], match[2])
	sections := parseIntsPage(fallback)
	if match[3] != "" {
		sections = parseIntsPage(match[3])
	}
	return weeks, sections
}
func parseIntsPage(value string) []int {
	numbers := regexp.MustCompile(`\d+`).FindAllString(value, -1)
	result := make([]int, 0)
	for _, item := range numbers {
		parsed, _ := strconv.Atoi(item)
		result = append(result, parsed)
	}
	return result
}
func expandWeekRangePage(value, parity string) []int {
	result := make([]int, 0)
	for _, part := range strings.Split(value, ",") {
		numbers := parseIntsPage(part)
		if len(numbers) == 1 {
			result = append(result, numbers[0])
		}
		if len(numbers) >= 2 {
			for item := numbers[0]; item <= numbers[1]; item++ {
				result = append(result, item)
			}
		}
	}
	filtered := make([]int, 0)
	seen := map[int]bool{}
	for _, item := range result {
		if parity == "单周" && item%2 == 0 || parity == "双周" && item%2 != 0 || seen[item] {
			continue
		}
		seen[item] = true
		filtered = append(filtered, item)
	}
	sort.Ints(filtered)
	return filtered
}
func parseGradesPage(document *pageNode, pageURL string) ([]map[string]any, *siteError) {
	table := academicTable(document, "dataList")
	if table == nil {
		if noAcademicData(document) {
			return []map[string]any{}, nil
		}
		return nil, &siteError{Code: "parse_error", Message: "未找到成绩表"}
	}
	rows := directTableRows(table)
	if len(rows) == 0 {
		return []map[string]any{}, nil
	}
	header := rowValues(rows[0])
	hasHeader := len(directCells(rows[0])) > 0 && (directCells(rows[0])[0].tag == "th" || countGradeHeaders(header) >= 2 || (len(header) > 0 && (header[0] == "序号" || header[0] == "序")))
	result := make([]map[string]any, 0)
	for index, row := range rows {
		if index == 0 && hasHeader {
			continue
		}
		values := rowValues(row)
		if len(values) < 6 || values[0] == "序号" || values[0] == "序" || allEmpty(values) {
			continue
		}
		item := map[string]any{"cells": values}
		if hasHeader {
			for column, title := range header {
				if field := gradeHeaderPage(title); field != "" && column < len(values) {
					item[field] = values[column]
				}
			}
		} else if len(values) >= 20 {
			names := []string{"semester", "course_id", "course", "group", "score", "score_mark", "credit", "hours", "grade_point", "general_elective", "original_score", "description", "note", "retake_semester", "assessment_method", "exam_type", "course_attribute", "course_nature", "course_category"}
			for column, name := range names {
				if column+1 < len(values) {
					item[name] = values[column+1]
				}
			}
		} else {
			item["course"], item["score"] = values[0], values[len(values)-1]
		}
		if anchor := row.first("a", ""); anchor != nil && anchor.attr("href") != "" {
			if target := resolvePageURL(pageURL, anchor.attr("href")); target != "" {
				item["grade_detail_url"] = safeSiteURL(mustParseURL(target))
			}
		}
		result = append(result, item)
	}
	return result, nil
}
func gradeHeaderPage(value string) string {
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	switch {
	case strings.Contains(value, "学期"):
		return "semester"
	case strings.Contains(value, "课程代码") || strings.Contains(value, "课程编号") || strings.Contains(value, "课号"):
		return "course_id"
	case value == "课程" || strings.Contains(value, "课程名称") || strings.Contains(value, "科目名称"):
		return "course"
	case strings.Contains(value, "成绩") || strings.Contains(value, "分数") || strings.Contains(value, "得分"):
		return "score"
	case strings.Contains(value, "修读方式") || strings.Contains(value, "学习方式"):
		return "study_mode"
	case strings.Contains(value, "学分"):
		return "credit"
	case strings.Contains(value, "学时"):
		return "hours"
	case strings.Contains(value, "绩点"):
		return "grade_point"
	case strings.Contains(value, "课程性质"):
		return "course_nature"
	case strings.Contains(value, "课程属性"):
		return "course_attribute"
	case strings.Contains(value, "课程类别"):
		return "course_category"
	case strings.Contains(value, "考核方式") || strings.Contains(value, "考试方式"):
		return "assessment_method"
	case strings.Contains(value, "重修学期") || strings.Contains(value, "补考学期"):
		return "retake_semester"
	}
	return ""
}
func countGradeHeaders(values []string) int {
	count := 0
	for _, value := range values {
		if gradeHeaderPage(value) != "" {
			count++
		}
	}
	return count
}
func parseGradeDetailPage(document *pageNode, pageURL string) (map[string]any, *siteError) {
	table := academicTable(document, "dataList")
	if table == nil {
		return nil, &siteError{Code: "parse_error", Message: "未找到成绩详情表"}
	}
	rows := directTableRows(table)
	if len(rows) < 2 {
		return nil, &siteError{Code: "parse_error", Message: "成绩详情表行数不足"}
	}
	headers, values := rowValues(rows[0]), rowValues(rows[1])
	fields := map[string]string{}
	for index, header := range headers {
		if header != "" && index < len(values) {
			fields[header] = values[index]
		}
	}
	return map[string]any{"url": safeSiteURL(mustParseURL(pageURL)), "fields": fields, "headers": headers, "cells": values, "text": pageDisplayText(rows[1])}, nil
}
func parseExamPage(document *pageNode, pageURL string) ([]map[string]any, *siteError) {
	table := academicTable(document, "dataList")
	if table == nil {
		if noAcademicData(document) {
			return []map[string]any{}, nil
		}
		return nil, &siteError{Code: "parse_error", Message: "未找到考试安排表"}
	}
	result := make([]map[string]any, 0)
	for _, row := range directTableRows(table) {
		values := rowValues(row)
		if len(values) < 10 || values[0] == "序号" || values[0] == "课程代码" || values[0] == "考试安排" {
			continue
		}
		item := map[string]any{"index": len(result) + 1, "sequence": values[0], "campus": valueAt(values, 1), "session": valueAt(values, 2), "course_id": valueAt(values, 3), "course": valueAt(values, 4), "teacher": valueAt(values, 5), "exam_time": valueAt(values, 6), "room": valueAt(values, 7), "seat": valueAt(values, 8), "admission_ticket": valueAt(values, 9), "remarks": valueAt(values, 10), "cells": values, "text": pageDisplayText(row)}
		if match := regexp.MustCompile(`(\d{4}[-/]\d{1,2}[-/]\d{1,2})\s+(\d{1,2}:\d{2})\s*[~～-]\s*(\d{1,2}:\d{2})`).FindStringSubmatch(valueAt(values, 6)); len(match) > 4 {
			item["date"], item["start_time"], item["end_time"] = match[1], match[2], match[3]
		}
		result = append(result, item)
	}
	_ = pageURL
	return result, nil
}
func parseClassroomPage(document *pageNode, pageURL string) ([]map[string]any, *siteError) {
	table := academicTable(document, "kbtable")
	if table == nil {
		if noAcademicData(document) {
			return []map[string]any{}, nil
		}
		return nil, &siteError{Code: "parse_error", Message: "未找到空闲教室表"}
	}
	result := make([]map[string]any, 0)
	for _, row := range directTableRows(table) {
		values := rowValues(row)
		if len(values) < 2 || containsString([]string{"教室", "教学楼", "教室名称"}, values[0]) {
			continue
		}
		occupied := false
		for _, value := range values[1:] {
			if strings.TrimSpace(value) != "" {
				occupied = true
				break
			}
		}
		if !occupied {
			result = append(result, map[string]any{"index": len(result) + 1, "room": values[0], "available": true, "cells": values, "text": pageDisplayText(row)})
		}
	}
	_ = pageURL
	return result, nil
}
func parseSelectionPage(document *pageNode, pageURL string) ([]map[string]any, *siteError) {
	table := academicTable(document, "dataList")
	if table == nil {
		if noAcademicData(document) {
			return []map[string]any{}, nil
		}
		return nil, &siteError{Code: "parse_error", Message: "未找到选课结果表"}
	}
	result := make([]map[string]any, 0)
	for _, row := range directTableRows(table) {
		values := rowValues(row)
		if len(values) < 8 || values[0] == "序号" {
			continue
		}
		result = append(result, map[string]any{"index": len(result) + 1, "sequence": values[0], "course": valueAt(values, 1), "course_id": valueAt(values, 2), "teacher": valueAt(values, 3), "hours": valueAt(values, 4), "credit": valueAt(values, 5), "course_attribute": valueAt(values, 6), "course_nature": valueAt(values, 7), "cells": values, "text": pageDisplayText(row)})
	}
	_ = pageURL
	return result, nil
}
func parseSemesterStartPage(document *pageNode, pageURL string) (map[string]any, *siteError) {
	table := academicTable(document, "kbtable")
	if table == nil {
		return nil, &siteError{Code: "parse_error", Message: "未找到学期起始日表"}
	}
	for _, row := range directTableRows(table) {
		values := rowValues(row)
		start := ""
		cells := directCells(row)
		if len(cells) > 1 {
			start = cells[1].attr("title")
		}
		if start == "" {
			start = regexp.MustCompile(`\d{4}年\d{1,2}月\d{1,2}日?`).FindString(strings.Join(values, " "))
		}
		if start != "" {
			return map[string]any{"url": safeSiteURL(mustParseURL(pageURL)), "start_date": start, "cells": values, "text": pageDisplayText(row)}, nil
		}
	}
	return nil, &siteError{Code: "parse_error", Message: "未找到学期起始日"}
}
func valueAt(values []string, index int) string {
	if index >= 0 && index < len(values) {
		return values[index]
	}
	return ""
}
func allEmpty(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}
func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func renderAcademic(result map[string]any) string {
	if fields, ok := result["fields"].(map[string]string); ok {
		var builder strings.Builder
		for name, value := range fields {
			builder.WriteString(fmt.Sprintf("%s: %s\n", name, value))
		}
		return builder.String()
	}
	if start, ok := result["start_date"].(string); ok {
		return "学期起始日：" + start + "\n"
	}
	if items, ok := result["items"].([]map[string]any); ok {
		var builder strings.Builder
		for _, item := range items {
			label := make([]string, 0)
			for _, key := range []string{"semester", "course", "score", "credit", "grade_point", "exam_time", "room", "name"} {
				if value, ok := item[key].(string); ok && value != "" {
					label = append(label, value)
				}
			}
			builder.WriteString(fmt.Sprintf("[%v] %s\n", item["index"], strings.Join(label, " ")))
		}
		return builder.String()
	}
	encoded, _ := json.Marshal(result)
	return string(encoded) + "\n"
}
