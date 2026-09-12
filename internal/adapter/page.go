package adapter

// Static HTML pages are an adapter boundary, not a browser runtime.  This
// file deliberately extracts only the controls and actions needed by the CLI.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

const pageSnapshotSchema = 1

var (
	pageSensitiveField  = regexp.MustCompile(`(?i)pass|password|token|secret|sign|randomcode|ticket|cookie|session|csrf|nonce|execution|flowexecutionkey|ysfzjh|yxm(?:py)?|sfzjh|(?:^|[_-])(?:state|lt)(?:$|[_-])`)
	pageEventAttrs      = []string{"onclick", "onchange", "onsubmit", "ondblclick"}
	pageEndpointLiteral = regexp.MustCompile(`["']((?:/api/|/ajax/|/rest/|/service/|/graphql|/oauth/|/auth/|/v1/|/v2/|/jsxsd/|/meol/|/moocresource/)[^"'\s<>]*)["']`)
	pageFunctionName    = regexp.MustCompile(`(?i)\bfunction\s+([A-Za-z_$][\w$]*)\s*\(`)
	pageSuccessMessage  = regexp.MustCompile(`^(?:密码重置|重置密码|密码修改|重置|邮件发送|操作|提交|保存|更新|删除|发布|评价|报名|选课|缴费|撤销|订购|退订|选订|处理|发送|回复|修改|设置|上传|排序|预约|退出|注销|登出)?(?:成功|完成|已保存|已提交)[！!。.]?$`)
	pageFailureMessage  = regexp.MustCompile(`(?i)(?:不存在|失败|错误|拒绝|无效|异常|未授权|禁止|failed|failure|error|denied|invalid|unauthorized|forbidden)`)
)

type pageNode struct {
	tag      string
	attrs    map[string]string
	children []*pageNode
	parent   *pageNode
	text     string
}

func parsePage(source string) (*pageNode, error) {
	root, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return nil, fmt.Errorf("HTML 解析失败: %w", err)
	}
	return convertHTMLNode(root, nil), nil
}

func convertHTMLNode(node *html.Node, parent *pageNode) *pageNode {
	if node == nil {
		return nil
	}
	converted := &pageNode{parent: parent}
	switch node.Type {
	case html.DocumentNode:
		converted.tag = "#document"
	case html.ElementNode:
		converted.tag = strings.ToLower(node.Data)
		converted.attrs = make(map[string]string, len(node.Attr))
		for _, attr := range node.Attr {
			converted.attrs[strings.ToLower(attr.Key)] = attr.Val
		}
	case html.TextNode:
		converted.tag = "#text"
		converted.text = node.Data
	default:
		converted.tag = "#ignored"
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		item := convertHTMLNode(child, converted)
		if item != nil && item.tag != "#ignored" {
			converted.children = append(converted.children, item)
		}
	}
	return converted
}

func (node *pageNode) attr(name string) string {
	if node == nil {
		return ""
	}
	return node.attrs[strings.ToLower(name)]
}

func (node *pageNode) has(name string) bool {
	_, ok := node.attrs[strings.ToLower(name)]
	return ok
}

func (node *pageNode) disabled() bool {
	return node.has("disabled")
}

func (node *pageNode) findAll(tag string) []*pageNode {
	if node == nil {
		return nil
	}
	result := make([]*pageNode, 0)
	var visit func(*pageNode)
	visit = func(current *pageNode) {
		if current.tag == tag || tag == "" {
			result = append(result, current)
		}
		for _, child := range current.children {
			visit(child)
		}
	}
	visit(node)
	return result
}

func (node *pageNode) first(tag, id string) *pageNode {
	for _, item := range node.findAll(tag) {
		if id == "" || item.attr("id") == id {
			return item
		}
	}
	return nil
}

func (node *pageNode) rawText() string {
	if node == nil {
		return ""
	}
	if node.tag == "#text" {
		return node.text
	}
	var builder strings.Builder
	for _, child := range node.children {
		builder.WriteString(child.rawText())
	}
	return builder.String()
}

func pageSensitiveControl(node *pageNode) bool {
	return node != nil && (pageSensitiveField.MatchString(node.attr("name")) || strings.EqualFold(node.attr("type"), "password") || strings.EqualFold(node.attr("type"), "file"))
}

func pageEvent(value string) string {
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return -1
		}
		return r
	}, value)
	return strings.TrimSpace(safeSiteErrorText(value))
}

func pageDisplayText(node *pageNode) string {
	if node == nil || node.tag == "script" || node.tag == "style" || pageSensitiveControl(node) {
		return ""
	}
	var builder strings.Builder
	var visit func(*pageNode)
	visit = func(current *pageNode) {
		if current == nil || current.tag == "script" || current.tag == "style" || pageSensitiveControl(current) {
			return
		}
		if current.tag == "#text" {
			builder.WriteString(current.text)
			return
		}
		for _, child := range current.children {
			visit(child)
		}
		if current != node && (current.tag == "br" || current.tag == "p" || current.tag == "div" || current.tag == "tr") {
			builder.WriteByte('\n')
		}
	}
	visit(node)
	return pageEvent(strings.Join(strings.Fields(strings.ReplaceAll(builder.String(), "\u00a0", " ")), " "))
}

func pageElementValue(node *pageNode) string {
	if node == nil {
		return ""
	}
	switch node.tag {
	case "textarea":
		return node.rawText()
	case "select":
		options := make([]*pageNode, 0)
		for _, option := range node.findAll("option") {
			if !option.disabled() {
				options = append(options, option)
			}
		}
		for _, option := range options {
			if option.has("selected") {
				return option.attr("value")
			}
		}
		if len(options) > 0 && !node.has("multiple") {
			return options[0].attr("value")
		}
		return ""
	default:
		return node.attr("value")
	}
}

func pageOptionValue(node *pageNode) string {
	if value := node.attr("value"); value != "" {
		return value
	}
	return pageDisplayText(node)
}

func pageFormOwner(node, document *pageNode) *pageNode {
	if node == nil {
		return nil
	}
	if formID := node.attr("form"); formID != "" && document != nil {
		return document.first("form", formID)
	}
	for current := node; current != nil; current = current.parent {
		if current.tag == "form" {
			return current
		}
	}
	return nil
}

func pageControls(form, document *pageNode) []*pageNode {
	if form == nil {
		return nil
	}
	controls := make([]*pageNode, 0)
	for _, node := range document.findAll("") {
		if node.tag != "input" && node.tag != "select" && node.tag != "textarea" && node.tag != "button" {
			continue
		}
		if pageFormOwner(node, document) == form {
			controls = append(controls, node)
		}
	}
	return controls
}

func pageFormFields(form, actionNode, document *pageNode) []pair {
	fields := make([]pair, 0)
	for _, node := range pageControls(form, document) {
		if node.disabled() || (node == actionNode && node.disabled()) {
			continue
		}
		name := node.attr("name")
		switch node.tag {
		case "input":
			kind := strings.ToLower(node.attr("type"))
			if kind == "" {
				kind = "text"
			}
			if kind == "image" && node == actionNode {
				fields = append(fields, pair{name + ".x", "0"}, pair{name + ".y", "0"})
				continue
			}
			if name == "" || (kind == "file") || (kind == "submit" || kind == "button" || kind == "reset" || kind == "image") && node != actionNode {
				continue
			}
			if (kind == "checkbox" || kind == "radio") && !node.has("checked") {
				continue
			}
			fields = append(fields, pair{name, pageElementValue(node)})
		case "select":
			if name == "" {
				continue
			}
			selected := make([]*pageNode, 0)
			options := make([]*pageNode, 0)
			for _, option := range node.findAll("option") {
				if option.disabled() {
					continue
				}
				options = append(options, option)
				if option.has("selected") {
					selected = append(selected, option)
				}
			}
			if len(selected) == 0 && len(options) > 0 && !node.has("multiple") {
				selected = append(selected, options[0])
			}
			for _, option := range selected {
				fields = append(fields, pair{name, pageOptionValue(option)})
			}
		case "textarea":
			if name != "" {
				fields = append(fields, pair{name, node.rawText()})
			}
		case "button":
			if node == actionNode && name != "" {
				fields = append(fields, pair{name, node.attr("value")})
			}
		}
	}
	return fields
}

func pageActionSubmits(node *pageNode) bool {
	if node == nil {
		return false
	}
	if node.tag == "form" || node.has("onsubmit") || strings.Contains(strings.ToLower(strings.Join(pageEventValues(node), " ")), "submit(") {
		return true
	}
	if node.tag == "button" {
		kind := strings.ToLower(node.attr("type"))
		return kind == "" || kind == "submit"
	}
	if node.tag == "input" {
		kind := strings.ToLower(node.attr("type"))
		return kind == "submit" || kind == "image" || kind == "button"
	}
	return false
}

func pageEventValues(node *pageNode) []string {
	values := make([]string, 0, len(pageEventAttrs))
	for _, name := range pageEventAttrs {
		if value := node.attr(name); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func pageActionNodes(document *pageNode) []*pageNode {
	nodes := make([]*pageNode, 0)
	for _, node := range document.findAll("") {
		if node.disabled() {
			continue
		}
		href := strings.TrimSpace(node.attr("href"))
		event := strings.TrimSpace(strings.Join(pageEventValues(node), " "))
		if node.tag == "a" && (href != "" && href != "#" && !strings.EqualFold(href, "javascript:void(0)") || event != "") {
			nodes = append(nodes, node)
			continue
		}
		if (node.tag == "input" || node.tag == "button") && strings.ToLower(node.attr("type")) != "reset" && (event != "" || pageActionSubmits(node)) {
			nodes = append(nodes, node)
			continue
		}
		if event != "" && (node.tag == "form" || node.tag == "select" || node.tag == "option" || node.tag == "textarea" || node.tag == "td" || node.tag == "tr" || node.tag == "div" || node.tag == "span") {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func resolvePageURL(pageURL, value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(strings.ToLower(value), "javascript:") || strings.HasPrefix(strings.ToLower(value), "mailto:") || strings.HasPrefix(strings.ToLower(value), "data:") {
		return ""
	}
	base, err := url.Parse(pageURL)
	if err != nil {
		return ""
	}
	target, err := base.Parse(value)
	if err != nil || target.Scheme != "http" && target.Scheme != "https" || target.Host == "" {
		return ""
	}
	target.Fragment = ""
	return target.String()
}

func pagePath(pageURL, target string) string {
	parsed, err := url.Parse(target)
	base, baseErr := url.Parse(pageURL)
	if err != nil || baseErr != nil || parsed.Scheme != base.Scheme || !strings.EqualFold(parsed.Host, base.Host) {
		return safeSiteURL(parsed)
	}
	safe := redactedURL(parsed)
	path := safe.EscapedPath()
	if path == "" {
		path = "/"
	}
	if safe.RawQuery != "" {
		path += "?" + safe.RawQuery
	}
	if safe.Fragment != "" {
		path += "#" + safe.Fragment
	}
	return path
}

func pageLink(node *pageNode, pageURL string) map[string]any {
	href := node.attr("href")
	if strings.HasPrefix(strings.ToLower(href), "javascript:") {
		if target := pageQuotedTarget(href, pageURL); target != "" {
			href = target
		}
	}
	target := pageURLValue(pageURL, href)
	return map[string]any{
		"text":     pageDisplayText(node),
		"href":     pageSafeValue(href, pageURL),
		"path":     pagePath(pageURL, target),
		"raw_path": target,
		"onclick":  pageEvent(node.attr("onclick")),
	}
}

func pageSafeValue(value, base string) string {
	if value == "" {
		return ""
	}
	if target := pageURLValue(base, value); target != "" {
		return safeSiteURL(mustParseURL(target))
	}
	return pageEvent(value)
}

func pageURLValue(base, value string) string {
	return resolvePageURL(base, value)
}

func mustParseURL(value string) *url.URL {
	parsed, _ := url.Parse(value)
	return parsed
}

func pageQuotedTarget(source, baseURL string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:window\.)?(?:location(?:\.href)?|open|openWindow|showWindow|openView|fetch|towptjbs)\s*\(\s*['"]([^'"]+)['"]`),
		regexp.MustCompile(`(?i)(?:window\.)?location(?:\.href)?\s*=\s*['"]([^'"]+)['"]`),
		regexp.MustCompile(`(?i)\.(load|get|post|put|patch|delete)\s*\(\s*['"]([^'"]+)['"]`),
		regexp.MustCompile(`(?i)(?:\.action\s*=|attr\s*\(\s*['"]action['"]\s*,)\s*['"]([^'"]+)['"]`),
	}
	for index, pattern := range patterns {
		matches := pattern.FindStringSubmatch(source)
		if len(matches) == 0 {
			continue
		}
		value := matches[len(matches)-1]
		if index == 2 && len(matches) > 2 {
			value = matches[2]
		}
		if target := resolvePageURL(baseURL, value); target != "" {
			return target
		}
	}
	return ""
}

func pageFunctionBody(document *pageNode, name string) string {
	if document == nil || name == "" {
		return ""
	}
	pattern := regexp.MustCompile(`(?is)\bfunction\s+` + regexp.QuoteMeta(name) + `\s*\([^)]*\)\s*\{(.*?)\}`)
	for _, script := range document.findAll("script") {
		if match := pattern.FindStringSubmatch(script.rawText()); len(match) > 1 {
			return match[1]
		}
	}
	return ""
}

func pageFunctionCalls(event string) []string {
	seen := map[string]bool{}
	pattern := regexp.MustCompile(`\b([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`)
	result := make([]string, 0)
	for _, match := range pattern.FindAllStringSubmatch(event, -1) {
		if len(match) < 2 || strings.EqualFold(match[1], "if") || strings.EqualFold(match[1], "alert") || strings.EqualFold(match[1], "confirm") {
			continue
		}
		if !seen[match[1]] {
			seen[match[1]] = true
			result = append(result, match[1])
		}
	}
	return result
}

func pageActionTarget(node, form *pageNode, document *pageNode, pageURL string) (method, target string) {
	method = "GET"
	if node == nil {
		return method, ""
	}
	if form != nil {
		method = strings.ToUpper(form.attr("method"))
		if method == "" {
			method = "GET"
		}
	}
	if formMethod := node.attr("formmethod"); formMethod != "" {
		method = strings.ToUpper(formMethod)
	}
	if formaction := node.attr("formaction"); formaction != "" {
		return method, resolvePageURL(pageURL, formaction)
	}
	if href := node.attr("href"); href != "" && !strings.HasPrefix(strings.ToLower(href), "javascript:") && href != "#" {
		return "GET", resolvePageURL(pageURL, href)
	}
	for _, event := range pageEventValues(node) {
		if target = pageQuotedTarget(event, pageURL); target != "" {
			lower := strings.ToLower(event)
			if strings.Contains(lower, ".post(") || strings.Contains(lower, "method:'post'") || strings.Contains(lower, "method:\"post\"") || strings.Contains(lower, ".submit(") {
				method = "POST"
			}
			return method, target
		}
		for _, function := range pageFunctionCalls(event) {
			body := pageFunctionBody(document, function)
			if body == "" {
				continue
			}
			if target = pageQuotedTarget(body, pageURL); target != "" {
				lower := strings.ToLower(body)
				if strings.Contains(lower, ".post(") || strings.Contains(lower, "method:'post'") || strings.Contains(lower, "method:\"post\"") || strings.Contains(lower, ".submit(") {
					method = "POST"
				}
				return method, target
			}
		}
	}
	if form != nil && pageActionSubmits(node) {
		return method, resolvePageURL(pageURL, form.attr("action"))
	}
	if node.tag == "form" {
		return method, resolvePageURL(pageURL, node.attr("action"))
	}
	_ = document
	return method, ""
}

func pageControl(node *pageNode, document *pageNode) map[string]any {
	sensitive := pageSensitiveControl(node)
	value := pageElementValue(node)
	if sensitive {
		value = "<redacted>"
	}
	result := map[string]any{
		"tag":         node.tag,
		"type":        firstNonEmpty(node.attr("type"), node.tag),
		"id":          node.attr("id"),
		"name":        node.attr("name"),
		"value":       pageEvent(value),
		"text":        "",
		"role":        node.attr("role"),
		"aria_label":  pageEvent(node.attr("aria-label")),
		"placeholder": pageEvent(node.attr("placeholder")),
		"href":        pageSafeValue(node.attr("href"), "https://www.csust.edu.cn/"),
		"onclick":     pageEvent(node.attr("onclick")),
		"disabled":    node.disabled(),
		"checked":     node.has("checked"),
	}
	if !sensitive {
		result["text"] = pageDisplayText(node)
	}
	if node.tag == "select" {
		result["multiple"] = node.has("multiple")
		options := make([]map[string]any, 0)
		for _, option := range node.findAll("option") {
			optionValue := pageOptionValue(option)
			if sensitive {
				optionValue = "<redacted>"
			}
			options = append(options, map[string]any{
				"text":     ifSensitiveText(sensitive, pageDisplayText(option)),
				"value":    pageEvent(optionValue),
				"selected": option.has("selected"),
				"disabled": option.disabled(),
			})
		}
		result["options"] = options
	}
	events := map[string]string{}
	for _, name := range pageEventAttrs {
		if value := node.attr(name); value != "" {
			events[name] = pageEvent(value)
		}
	}
	if len(events) > 0 {
		result["events"] = events
	}
	if label := pageControlLabel(document, node); label != "" {
		result["label"] = label
	}
	return result
}

func pageControlLabel(document, node *pageNode) string {
	if value := node.attr("aria-label"); value != "" {
		return pageEvent(value)
	}
	if labelled := strings.Fields(node.attr("aria-labelledby")); len(labelled) > 0 {
		parts := make([]string, 0, len(labelled))
		for _, id := range labelled {
			if label := document.first("", id); label != nil {
				parts = append(parts, pageDisplayText(label))
			}
		}
		if value := strings.TrimSpace(strings.Join(parts, " ")); value != "" {
			return value
		}
	}
	if id := node.attr("id"); id != "" {
		for _, label := range document.findAll("label") {
			if label.attr("for") == id && pageDisplayText(label) != "" {
				return pageDisplayText(label)
			}
		}
	}
	for current := node.parent; current != nil; current = current.parent {
		if current.tag == "label" && pageDisplayText(current) != "" {
			return pageDisplayText(current)
		}
	}
	return firstNonEmpty(pageEvent(node.attr("placeholder")), pageEvent(node.attr("title")))
}

func pageActionRef(node *pageNode, method, target string, form *pageNode, fields []string) string {
	formKey := strings.Join(fields, "|")
	if formKey == "" && form != nil {
		formKey = firstNonEmpty(form.attr("name"), form.attr("id"))
	}
	targetIdentity := ""
	if parsed, err := url.Parse(target); err == nil {
		targetIdentity = parsed.Path
		if parsed.RawQuery != "" {
			targetIdentity += "?" + parsed.RawQuery
		}
	}
	label := firstNonEmpty(pageDisplayText(node), node.attr("value"))
	call := ""
	for _, event := range pageEventValues(node) {
		if match := regexp.MustCompile(`(?i)([A-Za-z_$][\w$]*)\s*\(`).FindStringSubmatch(event); len(match) > 1 {
			call = match[1]
			break
		}
	}
	identity := strings.Join([]string{
		node.tag, strings.ToLower(node.attr("type")), strings.ToLower(node.attr("name")), strings.ToLower(node.attr("id")),
		strings.ToLower(node.attr("role")), strings.ToLower(node.attr("aria-label")), strings.ToLower(label), strings.ToLower(call),
		strings.ToUpper(method), strings.ToLower(formKey), targetIdentity,
	}, "\x1f")
	hash := sha256.Sum256([]byte(identity))
	return "action:" + hex.EncodeToString(hash[:])[:16]
}

func pageDescribeAction(node *pageNode, document *pageNode, pageURL string, index int) map[string]any {
	form := pageFormOwner(node, document)
	method, target := pageActionTarget(node, form, document, pageURL)
	fields := pageFormFields(form, node, document)
	fieldNames := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, field := range fields {
		if !seen[field.name] {
			seen[field.name] = true
			fieldNames = append(fieldNames, field.name)
		}
	}
	sort.Strings(fieldNames)
	return map[string]any{
		"index":        index,
		"ref":          pageActionRef(node, method, target, form, fieldNames),
		"text":         pageDisplayText(node),
		"tag":          node.tag,
		"method":       method,
		"target":       pageSafeValue(target, pageURL),
		"name":         node.attr("name"),
		"id":           node.attr("id"),
		"role":         node.attr("role"),
		"aria_label":   pageEvent(node.attr("aria-label")),
		"form":         firstNonEmpty(formAttr(form, "id"), ""),
		"form_index":   pageFormIndex(document, form),
		"fields":       fieldNames,
		"state_fields": []string{},
		"href":         pageSafeValue(node.attr("href"), pageURL),
		"onclick":      pageEvent(node.attr("onclick")),
		"events":       pageEvents(node),
		"arguments":    []string{},
	}
}

func pageEvents(node *pageNode) map[string]string {
	result := map[string]string{}
	for _, name := range pageEventAttrs {
		if value := node.attr(name); value != "" {
			result[name] = pageEvent(value)
		}
	}
	return result
}

func formAttr(form *pageNode, name string) string {
	if form == nil {
		return ""
	}
	return form.attr(name)
}

func pageFormIndex(document, form *pageNode) int {
	if form == nil {
		return 0
	}
	for index, item := range document.findAll("form") {
		if item == form {
			return index + 1
		}
	}
	return 0
}

func pageTable(node *pageNode) map[string]any {
	rows := make([][]string, 0)
	for _, row := range node.findAll("tr") {
		cells := make([]string, 0)
		for _, cell := range row.children {
			if cell.tag == "th" || cell.tag == "td" {
				cells = append(cells, pageDisplayText(cell))
			}
		}
		if len(cells) > 0 {
			rows = append(rows, cells)
		}
	}
	headers := []string{}
	dataRows := rows
	if len(rows) > 0 {
		first := node.findAll("th")
		if len(first) > 0 {
			headers = append(headers, rows[0]...)
			dataRows = rows[1:]
		}
	}
	return map[string]any{"headers": headers, "data_rows": dataRows, "rows": rows}
}

func pageShapeFingerprint(document *pageNode) string {
	var parts []string
	shapeAttrs := map[string]bool{"type": true, "name": true, "role": true, "aria-label": true, "aria-labelledby": true, "placeholder": true, "method": true, "enctype": true, "multiple": true}
	var visit func(*pageNode)
	visit = func(node *pageNode) {
		if node == nil || node.tag == "#text" || node.tag == "script" || node.tag == "style" {
			return
		}
		parts = append(parts, "<"+node.tag)
		keys := make([]string, 0)
		for key := range node.attrs {
			if shapeAttrs[key] {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		parts = append(parts, strings.Join(keys, ",")+">")
		for _, child := range node.children {
			visit(child)
		}
		parts = append(parts, "</"+node.tag+">")
	}
	visit(document)
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(hash[:])
}

func pageInspect(source, pageURL string) (map[string]any, *siteError) {
	document, err := parsePage(source)
	if err != nil {
		return nil, &siteError{Code: "parse_error", Message: err.Error()}
	}
	links := make([]map[string]any, 0)
	for _, node := range document.findAll("a") {
		if node.attr("href") != "" || node.attr("onclick") != "" {
			links = append(links, pageLink(node, pageURL))
		}
	}
	forms := make([]map[string]any, 0)
	for index, form := range document.findAll("form") {
		controls := make([]map[string]any, 0)
		for _, node := range pageControls(form, document) {
			controls = append(controls, pageControl(node, document))
		}
		method := strings.ToUpper(form.attr("method"))
		if method == "" {
			method = "GET"
		}
		forms = append(forms, map[string]any{
			"index":    index + 1,
			"action":   pageSafeValue(pageURLValue(pageURL, form.attr("action")), pageURL),
			"method":   method,
			"name":     form.attr("name"),
			"id":       form.attr("id"),
			"enctype":  form.attr("enctype"),
			"controls": controls,
		})
	}
	tables := make([]map[string]any, 0)
	for _, table := range document.findAll("table") {
		tables = append(tables, pageTable(table))
	}
	actions := make([]map[string]any, 0)
	for index, node := range pageActionNodes(document) {
		actions = append(actions, pageDescribeAction(node, document, pageURL, index+1))
	}
	scripts := document.findAll("script")
	scriptSources := make([]string, 0)
	functions := make([]string, 0)
	endpoints := map[string]bool{}
	messageSet := map[string]bool{}
	for _, script := range scripts {
		if src := script.attr("src"); src != "" {
			if target := resolvePageURL(pageURL, src); target != "" {
				scriptSources = append(scriptSources, safeSiteURL(mustParseURL(target)))
			}
		}
		for _, match := range pageFunctionName.FindAllStringSubmatch(script.rawText(), -1) {
			if len(match) > 1 {
				functions = append(functions, match[1])
			}
		}
		for _, match := range pageEndpointLiteral.FindAllStringSubmatch(script.rawText(), -1) {
			if len(match) > 1 {
				if target := resolvePageURL(pageURL, match[1]); target != "" {
					endpoints[safeSiteURL(mustParseURL(target))] = true
				}
			}
		}
		for _, match := range regexp.MustCompile(`(?i)(?:alert|showMsg)\s*\(\s*['"]([^'"]+)['"]`).FindAllStringSubmatch(script.rawText(), -1) {
			if len(match) > 1 && pageEvent(match[1]) != "" {
				messageSet[pageEvent(match[1])] = true
			}
		}
	}
	text := pageDisplayText(document)
	if len(text) > 12000 {
		text = text[:12000]
	}
	kind := "html"
	if len(scripts) > 0 && len(links) == 0 && len(forms) == 0 && len(tables) == 0 && len(actions) == 0 && text == "" {
		kind = "dynamic"
	}
	confidence := "high"
	if kind == "dynamic" {
		confidence = "low"
	}
	capabilities := make([]string, 0, 6)
	if len(links) > 0 {
		capabilities = append(capabilities, "links")
	}
	if len(forms) > 0 {
		capabilities = append(capabilities, "forms")
	}
	if len(tables) > 0 {
		capabilities = append(capabilities, "tables")
	}
	if len(actions) > 0 {
		capabilities = append(capabilities, "actions")
	}
	if len(endpoints) > 0 {
		capabilities = append(capabilities, "api_candidates")
	}
	if kind == "dynamic" {
		capabilities = append(capabilities, "dynamic")
	}
	endpointList := make([]string, 0, len(endpoints))
	for endpoint := range endpoints {
		endpointList = append(endpointList, endpoint)
	}
	sort.Strings(endpointList)
	sort.Strings(functions)
	sort.Strings(scriptSources)
	messages := make([]string, 0, len(messageSet))
	for message := range messageSet {
		messages = append(messages, message)
	}
	sort.Strings(messages)
	return map[string]any{
		"schema_version": pageSnapshotSchema,
		"adapter":        "html-contract",
		"contract":       "page-snapshot-v1",
		"kind":           kind,
		"confidence":     confidence,
		"confidence_evidence": map[string]any{
			"reason": func() string {
				if kind == "dynamic" {
					return "页面只有脚本且没有静态业务内容，无法从 HTML 确认功能"
				}
				return "页面包含可解析的静态页面内容或业务控件"
			}(),
			"script_count": len(scripts), "link_count": len(links), "form_count": len(forms), "table_count": len(tables), "action_count": len(actions), "visible_text_length": len(text),
		},
		"url":               pageSafeValue(pageURL, pageURL),
		"fingerprint":       sha256String(source),
		"shape_fingerprint": pageShapeFingerprint(document),
		"title": func() string {
			if title := document.first("title", ""); title != nil {
				return pageDisplayText(title)
			}
			return ""
		}(),
		"text": text, "capabilities": capabilities, "messages": messages, "links": links, "forms": forms, "tables": tables, "actions": actions,
		"scripts": map[string]any{"src": scriptSources, "functions": uniqueStrings(functions)}, "endpoints": endpointList,
	}, nil
}

func sha256String(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func ifSensitiveText(sensitive bool, value string) string {
	if sensitive {
		return ""
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func pageFeedback(source, contentType string) (bool, bool) {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return false, false
	}
	if strings.Contains(strings.ToLower(contentType), "json") || strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var value any
		if json.Unmarshal([]byte(trimmed), &value) == nil {
			return jsonBusinessState(value)
		}
	}
	if strings.Contains(strings.ToLower(contentType), "html") || strings.HasPrefix(trimmed, "<") {
		if document, err := parsePage(source); err == nil {
			for _, message := range document.findAll("script") {
				for _, match := range regexp.MustCompile(`(?i)(?:alert|showMsg)\s*\(\s*['"]([^'"]+)['"]`).FindAllStringSubmatch(message.rawText(), -1) {
					if len(match) > 1 {
						if pageSuccessMessage.MatchString(strings.TrimSpace(match[1])) {
							return true, true
						}
						if pageFailureMessage.MatchString(match[1]) {
							return false, true
						}
					}
				}
			}
		}
		return false, false
	}
	if pageFailureMessage.MatchString(trimmed) {
		return false, true
	}
	if pageSuccessMessage.MatchString(trimmed) {
		return true, true
	}
	return false, false
}
