package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
)

type siteCommand struct {
	name          string
	request       siteRequest
	allowExternal bool
	data          []pair
	files         []filePart
	form          int
	button        int
	index         int
	ref           string
	fingerprint   string
	maxScripts    int
	depth         int
	maxPages      int
	login         loginOptions
}

// runSiteCommand covers the protocol-independent page adapter.
func (a NativeSite) runSiteCommand(ctx context.Context, args []string, jsonMode bool) (bool, []byte, []byte, int, error) {
	if len(args) < 2 || (args[0] != "site" && args[0] != "domain" && args[0] != "portal") || args[1] == "request" {
		return false, nil, nil, 0, nil
	}
	if containsHelp(args[2:]) {
		return false, nil, nil, 0, nil
	}
	command, parseErr := parseSiteCommand(args[1:])
	if parseErr != nil {
		if jsonMode {
			return true, errorJSON(parseErr), nil, 2, nil
		}
		return true, nil, []byte("错误: " + parseErr.Error() + "\n"), 2, nil
	}
	result, runErr := a.executeSiteCommand(ctx, command)
	if runErr != nil {
		if jsonMode {
			return true, errorJSON(runErr), nil, 2, nil
		}
		return true, nil, []byte("错误: " + runErr.Error() + "\n"), 2, nil
	}
	if jsonMode {
		encoded, encodeErr := json.Marshal(stripSiteInternal(result))
		if encodeErr != nil {
			return true, nil, nil, 2, encodeErr
		}
		return true, encoded, nil, 0, nil
	}
	return true, []byte(renderSiteCommand(stripSiteInternal(result).(map[string]any))), nil, 0, nil
}

func parseSiteCommand(args []string) (siteCommand, *siteError) {
	if len(args) == 0 {
		return siteCommand{}, &siteError{Code: "invalid_argument", Message: "缺少 site 子命令"}
	}
	command := siteCommand{name: args[0], request: siteRequest{Method: "GET"}, maxScripts: 30, depth: 1, maxPages: 30, login: loginOptions{auth: "auto"}}
	if command.name == "catalog" {
		for _, arg := range args[1:] {
			if arg != "--json" {
				return siteCommand{}, &siteError{Code: "invalid_argument", Message: "site catalog 不接受参数"}
			}
		}
		return command, nil
	}
	for index := 1; index < len(args); index++ {
		arg := args[index]
		inlineValue, inline := "", false
		if strings.HasPrefix(arg, "--") {
			if name, value, found := strings.Cut(arg, "="); found {
				arg, inlineValue, inline = name, value, true
			}
		}
		if arg == "--json" {
			continue
		}
		if arg == "--yes" {
			if inline {
				return siteCommand{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			command.request.Yes = true
			continue
		}
		value, next, hasValue := inlineValue, index, inline
		if !inline {
			value, next, hasValue = nextValue(args, index)
		}
		if hasValue && !inline {
			index = next
		}
		switch arg {
		case "--service":
			command.request.Service = value
		case "--path":
			command.request.Path = value
		case "--scheme":
			command.request.Scheme = value
		case "--cookie-file":
			command.request.CookieFile = expandUserPath(value)
		case "--output":
			command.request.Output = expandUserPath(value)
		case "--require-login":
			command.request.RequireLogin = true
		case "--insecure":
			if inline {
				return siteCommand{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			command.request.InsecureTLS = true
		case "--allow-external":
			command.allowExternal = true
		case "--username":
			command.login.username = value
		case "--auth":
			command.login.auth = value
		case "--captcha":
			command.login.captcha = value
		case "--captcha-image":
			command.login.captchaImage = expandUserPath(value)
		case "--password-stdin":
			if inline || hasValue {
				return siteCommand{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			command.login.passwordStdin = true
		case "--mobile":
			command.login.mobile = value
		case "--dynamic-code":
			command.login.dynamicCode = value
		case "--send-code":
			if inline || hasValue {
				return siteCommand{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			command.login.sendCode = true
		case "--qr-image":
			command.login.qrImage = expandUserPath(value)
		case "--param":
			item, err := splitPair(value, "--param")
			if err != nil {
				return siteCommand{}, err
			}
			command.request.Params = append(command.request.Params, item)
		case "--data":
			item, err := splitPair(value, "--data")
			if err != nil {
				return siteCommand{}, err
			}
			command.data = append(command.data, item)
		case "--file":
			item, err := readFilePart(value)
			if err != nil {
				return siteCommand{}, err
			}
			command.files = append(command.files, item)
		case "--header":
			item, err := splitPair(value, "--header")
			if err != nil {
				return siteCommand{}, err
			}
			command.request.Headers = append(command.request.Headers, item)
		case "--form":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return siteCommand{}, &siteError{Code: "invalid_argument", Message: "--form 必须是正整数"}
			}
			command.form = parsed
		case "--button":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return siteCommand{}, &siteError{Code: "invalid_argument", Message: "--button 必须是正整数"}
			}
			command.button = parsed
		case "--index":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return siteCommand{}, &siteError{Code: "invalid_argument", Message: "--index 必须是正整数"}
			}
			command.index = parsed
		case "--ref":
			command.ref = value
		case "--fingerprint":
			command.fingerprint = value
		case "--max-scripts":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return siteCommand{}, &siteError{Code: "invalid_argument", Message: "--max-scripts 必须是整数"}
			}
			command.maxScripts = parsed
		case "--depth":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return siteCommand{}, &siteError{Code: "invalid_argument", Message: "--depth 必须是整数"}
			}
			command.depth = parsed
		case "--max-pages":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return siteCommand{}, &siteError{Code: "invalid_argument", Message: "--max-pages 必须是整数"}
			}
			command.maxPages = parsed
		default:
			if strings.HasPrefix(arg, "-") {
				return siteCommand{}, &siteError{Code: "invalid_argument", Message: "site 参数无效: " + arg}
			}
			return siteCommand{}, &siteError{Code: "invalid_argument", Message: "site 子命令不接受位置参数"}
		}
		if !hasValue && arg != "--require-login" && arg != "--allow-external" && arg != "--insecure" && arg != "--yes" && arg != "--json" && arg != "--password-stdin" && arg != "--send-code" {
			return siteCommand{}, &siteError{Code: "invalid_argument", Message: arg + " 缺少参数值"}
		}
	}
	if command.name == "scripts" && os.Getenv("CSUST_EXPLORATION") != "1" {
		return siteCommand{}, &siteError{Code: "exploration_required", Message: "site scripts 仅限开发阶段，请设置 CSUST_EXPLORATION=1"}
	}
	if command.name == "discover" && os.Getenv("CSUST_EXPLORATION") != "1" {
		return siteCommand{}, &siteError{Code: "exploration_required", Message: "site discover 仅限开发阶段，请设置 CSUST_EXPLORATION=1"}
	}
	if command.request.Service == "" {
		return siteCommand{}, &siteError{Code: "invalid_argument", Message: "必须提供 --service"}
	}
	if command.name == "login" && command.login.auth != "auto" && command.login.auth != "sso" && command.login.auth != "dynamic" && command.login.auth != "qr" {
		return siteCommand{}, &siteError{Code: "invalid_argument", Message: "site login 只支持 auto、sso、dynamic 或 qr"}
	}
	if command.form < 0 || command.button < 0 || command.index < 0 {
		return siteCommand{}, &siteError{Code: "invalid_argument", Message: "表单、按钮和动作序号必须为正整数"}
	}
	if command.name == "form" && command.form == 0 {
		return siteCommand{}, &siteError{Code: "invalid_argument", Message: "site form 必须提供 --form"}
	}
	if command.name == "action" && command.ref == "" && command.index == 0 {
		return siteCommand{}, &siteError{Code: "invalid_argument", Message: "site action 必须提供 --ref 或 --index"}
	}
	if command.name == "scripts" && (command.maxScripts < 1 || command.maxScripts > 100) {
		return siteCommand{}, &siteError{Code: "invalid_argument", Message: "--max-scripts 必须在 1 到 100 之间"}
	}
	if command.name == "discover" && (command.depth < 0 || command.depth > 3 || command.maxPages < 1 || command.maxPages > 2000) {
		return siteCommand{}, &siteError{Code: "invalid_argument", Message: "--depth 必须在 0 到 3 之间，--max-pages 必须在 1 到 2000 之间"}
	}
	return command, nil
}

func (a NativeSite) executeSiteCommand(ctx context.Context, command siteCommand) (map[string]any, *siteError) {
	switch command.name {
	case "login":
		target, cookiePath, resolveErr := resolveSite(command.request)
		if resolveErr != nil {
			return nil, resolveErr
		}
		command.login.yes = command.request.Yes
		return a.loginSSOService(ctx, target, cookiePath, command.login)
	case "catalog":
		return siteCatalogResult(), nil
	case "get":
		result, err := a.execute(ctx, command.request)
		if err != nil {
			return nil, err
		}
		return sitePageResult(result), nil
	case "form":
		return a.submitSiteForm(ctx, command, false)
	case "action", "run":
		return a.submitSiteForm(ctx, command, true)
	case "scripts":
		return a.scanSiteScripts(ctx, command)
	case "discover":
		return a.discoverSite(ctx, command)
	case "logout":
		_, path, err := resolveSite(command.request)
		if err != nil {
			return nil, err
		}
		if removeErr := removeCookieFile(path); removeErr != nil {
			return nil, &siteError{Code: "cookie_write_failed", Message: removeErr.Error()}
		}
		return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed", "site": siteOriginForRequest(command.request), "cookie_file": path, "logged_out": true}, nil
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "未知 site 子命令: " + command.name}
	}
}

func siteCatalogResult() map[string]any {
	type entry struct{ service, host, name, url string }
	entries := make([]entry, 0, len(knownSites))
	for service, info := range knownSites {
		entries = append(entries, entry{service, info.host, service, info.scheme + "://" + info.host + info.path})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].service < entries[j].service })
	catalog := make([]map[string]any, 0, len(entries))
	for _, item := range entries {
		catalog = append(catalog, map[string]any{"service": item.service, "host": item.host, "name": item.name, "url": item.url})
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed", "source": "https://www.csust.edu.cn/", "catalog": catalog, "domain": siteDomain, "note": "清单是已观察到的入口；开发阶段可用 CSUST_EXPLORATION=1 实时发现，site 命令接受新子域名。"}
}

func siteOriginForRequest(request siteRequest) string {
	target, _, err := resolveSite(request)
	if err != nil {
		return ""
	}
	return origin(target)
}

func sitePageResult(result map[string]any) map[string]any {
	response, ok := result["response"].(map[string]any)
	if !ok {
		return result
	}
	if format, _ := response["format"].(string); format == "json" {
		if value, exists := response["json"]; exists {
			result["response"] = value
		}
		return result
	}
	body, _ := response["body_internal"].(string)
	if body == "" {
		body, _ = response["body"].(string)
	}
	pageURL, _ := response["raw_url"].(string)
	if pageURL == "" {
		pageURL, _ = response["url"].(string)
	}
	if body == "" {
		return result
	}
	page, err := pageInspect(body, pageURL)
	if err != nil {
		result["response"] = map[string]any{"kind": "text", "body": body, "confidence": "low", "confidence_evidence": map[string]any{"reason": "响应不是可解析的 HTML 页面"}}
		return result
	}
	result["response"] = page
	return result
}

func (a NativeSite) loadSitePage(ctx context.Context, command siteCommand) (*pageNode, string, string, string, *siteError) {
	request := command.request
	request.Method = "GET"
	request.Data = nil
	request.Files = nil
	request.HasJSON = false
	request.JSON = nil
	request.Output = ""
	result, err := a.execute(ctx, request)
	if err != nil {
		return nil, "", "", "", err
	}
	response, ok := result["response"].(map[string]any)
	if !ok {
		return nil, "", "", "", &siteError{Code: "parse_error", Message: "页面响应不是可操作的 HTML 页面"}
	}
	source, _ := response["body_internal"].(string)
	if source == "" {
		source, _ = response["body"].(string)
	}
	var target *url.URL
	var cookiePath string
	if request.Target != nil {
		target = request.Target
		cookiePath = request.CookieFile
		if cookiePath == "" {
			cookiePath = cookieFile("", target)
		}
	} else {
		var resolveErr *siteError
		target, cookiePath, resolveErr = resolveSite(request)
		if resolveErr != nil {
			return nil, "", "", "", resolveErr
		}
	}
	pageURL := target.String()
	if rawURL, _ := response["raw_url"].(string); rawURL != "" {
		if parsed, parseErr := url.Parse(rawURL); parseErr == nil && parsed.Host != "" {
			pageURL = parsed.String()
		}
	}
	if source == "" {
		return nil, "", "", "", &siteError{Code: "parse_error", Message: "页面响应为空"}
	}
	document, parseErr := parsePage(source)
	if parseErr != nil {
		return nil, "", "", "", &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	return document, source, pageURL, cookiePath, nil
}

func (a NativeSite) submitSiteForm(ctx context.Context, command siteCommand, actionMode bool) (map[string]any, *siteError) {
	document, source, pageURL, cookiePath, err := a.loadSitePage(ctx, command)
	if err != nil {
		return nil, err
	}
	if command.fingerprint != "" && command.fingerprint != sha256String(source) {
		return nil, &siteError{Code: "stale_page", Message: "页面指纹不匹配，拒绝提交"}
	}
	var node, form *pageNode
	if actionMode {
		actions := pageActionNodes(document)
		for index, candidate := range actions {
			description := pageDescribeAction(candidate, document, pageURL, index+1)
			if command.ref != "" && description["ref"] == command.ref {
				node = candidate
				break
			}
			if command.ref == "" && command.index == index+1 {
				node = candidate
				break
			}
		}
		if node == nil {
			return nil, &siteError{Code: "invalid_argument", Message: "找不到指定页面动作"}
		}
		form = pageFormOwner(node, document)
	} else {
		forms := document.findAll("form")
		if command.form < 1 || command.form > len(forms) {
			return nil, &siteError{Code: "invalid_argument", Message: "表单序号超出页面范围"}
		}
		form = forms[command.form-1]
		node = form
		if command.button > 0 {
			buttons := make([]*pageNode, 0)
			for _, candidate := range pageActionNodes(document) {
				if pageFormOwner(candidate, document) == form && pageActionSubmits(candidate) {
					buttons = append(buttons, candidate)
				}
			}
			if command.button > len(buttons) {
				return nil, &siteError{Code: "invalid_argument", Message: "按钮序号超出表单范围"}
			}
			node = buttons[command.button-1]
		}
	}
	method, targetValue := pageActionTarget(node, form, document, pageURL)
	if targetValue == "" {
		return nil, &siteError{Code: "parse_error", Message: "无法从页面动作解析请求地址；请使用 site request 显式调用"}
	}
	target, targetErr := validatePageActionTarget(targetValue, pageURL, command.allowExternal)
	if targetErr != nil {
		return nil, targetErr
	}
	fields := pageFormFields(form, node, document)
	fields = overridePairs(fields, command.data)
	method = strings.ToUpper(method)
	if method != "GET" && method != "POST" && method != "PUT" && method != "PATCH" && method != "DELETE" {
		return nil, &siteError{Code: "parse_error", Message: "页面动作使用了不支持的 HTTP 方法: " + method}
	}
	if command.request.Yes == false && (method != "GET" || sideEffectSiteURL(target)) {
		return nil, &siteError{Code: "confirmation_required", Message: "site 页面动作可能修改远端数据，请加 --yes"}
	}
	submit := command.request
	submit.Service = ""
	submit.Path = ""
	submit.Target = target
	submit.CookieFile = cookiePath
	submit.Method = method
	submit.Data = nil
	submit.Params = nil
	submit.Files = command.files
	if method == "GET" {
		submit.Params = fields
	} else {
		submit.Data = fields
	}
	result, requestErr := a.execute(ctx, submit)
	if requestErr != nil {
		return nil, requestErr
	}
	return sitePageResult(result), nil
}

func overridePairs(base, overrides []pair) []pair {
	if len(overrides) == 0 {
		return base
	}
	byName := make(map[string][]pair, len(overrides))
	for _, override := range overrides {
		byName[override.name] = append(byName[override.name], override)
	}
	result := make([]pair, 0, len(base)+len(overrides))
	replaced := make(map[string]bool, len(byName))
	for _, item := range base {
		replacement, found := byName[item.name]
		if !found {
			result = append(result, item)
			continue
		}
		if !replaced[item.name] {
			result = append(result, replacement...)
			replaced[item.name] = true
		}
	}
	for _, override := range overrides {
		if !replaced[override.name] {
			result = append(result, override)
			replaced[override.name] = true
		}
	}
	return result
}

func validatePageActionTarget(value, base string, allowExternal bool) (*url.URL, *siteError) {
	targetValue := resolvePageURL(base, value)
	if targetValue == "" {
		return nil, &siteError{Code: "invalid_path", Message: "页面动作目标不是 HTTP(S) 地址"}
	}
	target, err := url.Parse(targetValue)
	if err != nil || target.User != nil || target.Hostname() == "" {
		return nil, &siteError{Code: "invalid_path", Message: "页面动作目标格式无效"}
	}
	baseURL, baseErr := url.Parse(base)
	if baseErr != nil || baseURL.Host == "" {
		return nil, &siteError{Code: "invalid_path", Message: "页面基地址无效"}
	}
	if !allowExternal && !strings.EqualFold(target.Host, baseURL.Host) {
		return nil, &siteError{Code: "invalid_path", Message: "页面动作必须保持当前子域名；如页面明确指向外部服务请加 --allow-external"}
	}
	if strings.EqualFold(baseURL.Scheme, "https") && strings.EqualFold(target.Scheme, "http") && strings.EqualFold(baseURL.Host, target.Host) {
		return nil, &siteError{Code: "invalid_path", Message: "不允许 HTTPS 页面降级到 HTTP 动作"}
	}
	return target, nil
}

func (a NativeSite) scanSiteScripts(ctx context.Context, command siteCommand) (map[string]any, *siteError) {
	document, _, pageURL, cookiePath, err := a.loadSitePage(ctx, command)
	if err != nil {
		return nil, err
	}
	scripts := make([]map[string]any, 0)
	endpoints := map[string]bool{}
	for _, node := range document.findAll("script") {
		source := node.attr("src")
		if source == "" {
			continue
		}
		target := resolvePageURL(pageURL, source)
		parsed, parseErr := url.Parse(target)
		if parseErr != nil || parsed.Host != mustParseURL(pageURL).Host {
			scripts = append(scripts, map[string]any{"url": pageSafeValue(target, pageURL), "skipped": true, "reason": "external"})
			continue
		}
		request := command.request
		request.Service, request.Path, request.Target = "", "", parsed
		request.CookieFile, request.Method, request.Params, request.Data, request.Files, request.Output = cookiePath, "GET", nil, nil, nil, ""
		result, requestErr := a.execute(ctx, request)
		if requestErr != nil {
			scripts = append(scripts, map[string]any{"url": safeSiteURL(parsed), "error": requestErr.Message, "code": requestErr.Code})
			continue
		}
		response, _ := result["response"].(map[string]any)
		body, _ := response["body_internal"].(string)
		if body == "" {
			body, _ = response["body"].(string)
		}
		found := scriptEndpoints(body, target)
		for _, endpoint := range found {
			endpoints[endpoint] = true
		}
		scripts = append(scripts, map[string]any{"url": safeSiteURL(parsed), "bytes": len([]byte(body)), "endpoints": found})
		if len(scripts) >= command.maxScripts {
			break
		}
	}
	endpointList := make([]string, 0, len(endpoints))
	for endpoint := range endpoints {
		endpointList = append(endpointList, endpoint)
	}
	sort.Strings(endpointList)
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed", "page": map[string]any{"url": safeSiteURL(mustParseURL(pageURL)), "title": pageTitle(document), "kind": pageKind(document)}, "scripts": scripts, "script_count": len(scripts), "endpoints": endpointList}, nil
}

func scriptEndpoints(source, pageURL string) []string {
	seen := map[string]bool{}
	for _, match := range pageEndpointLiteral.FindAllStringSubmatch(source, -1) {
		if len(match) > 1 {
			if target := resolvePageURL(pageURL, match[1]); target != "" {
				seen[safeSiteURL(mustParseURL(target))] = true
			}
		}
	}
	result := make([]string, 0, len(seen))
	for endpoint := range seen {
		result = append(result, endpoint)
	}
	sort.Strings(result)
	return result
}

func pageTitle(document *pageNode) string {
	if title := document.first("title", ""); title != nil {
		return pageDisplayText(title)
	}
	return ""
}
func pageKind(document *pageNode) string {
	if len(document.findAll("script")) > 0 && len(pageActionNodes(document)) == 0 && len(document.findAll("form")) == 0 && pageDisplayText(document) == "" {
		return "dynamic"
	}
	return "html"
}

func (a NativeSite) discoverSite(ctx context.Context, command siteCommand) (map[string]any, *siteError) {
	target, cookiePath, resolveErr := resolveSite(command.request)
	if resolveErr != nil {
		return nil, resolveErr
	}
	queue := []struct {
		target string
		depth  int
	}{{target.String(), 0}}
	queued := map[string]bool{target.String(): true}
	pages := make([]map[string]any, 0)
	errors := make([]map[string]any, 0)
	hosts := map[string]bool{strings.ToLower(target.Hostname()): true}
	for len(queue) > 0 && len(pages) < command.maxPages {
		item := queue[0]
		queue = queue[1:]
		parsed, _ := url.Parse(item.target)
		request := command.request
		request.Service, request.Path, request.Target = "", "", parsed
		request.CookieFile, request.Method, request.Params, request.Data, request.Files, request.Output = cookiePath, "GET", nil, nil, nil, ""
		result, requestErr := a.execute(ctx, request)
		if requestErr != nil {
			errors = append(errors, map[string]any{"url": safeSiteURL(parsed), "code": requestErr.Code, "error": requestErr.Message})
			continue
		}
		response, _ := result["response"].(map[string]any)
		source, _ := response["body_internal"].(string)
		if source == "" {
			source, _ = response["body"].(string)
		}
		page, pageErr := pageInspect(source, item.target)
		if pageErr != nil {
			errors = append(errors, map[string]any{"url": safeSiteURL(parsed), "code": pageErr.Code, "error": pageErr.Message})
			continue
		}
		pages = append(pages, map[string]any{"url": page["url"], "title": page["title"], "kind": page["kind"], "links": page["links"], "forms": page["forms"], "actions": page["actions"], "endpoints": page["endpoints"]})
		for _, link := range page["links"].([]map[string]any) {
			value, _ := link["raw_path"].(string)
			if value == "" {
				value, _ = link["path"].(string)
			}
			child := resolvePageURL(item.target, value)
			childURL, childErr := url.Parse(child)
			if childErr != nil || childURL.Hostname() == "" || !strings.HasSuffix(strings.ToLower(childURL.Hostname()), "."+siteDomain) && !strings.EqualFold(childURL.Hostname(), siteDomain) {
				continue
			}
			hosts[strings.ToLower(childURL.Hostname())] = true
			if item.depth >= command.depth || sideEffectSiteURL(childURL) || !crawlableSitePath(childURL.Path) || queued[childURL.String()] {
				continue
			}
			queued[childURL.String()] = true
			queue = append(queue, struct {
				target string
				depth  int
			}{childURL.String(), item.depth + 1})
		}
	}
	hostList := make([]string, 0, len(hosts))
	for host := range hosts {
		hostList = append(hostList, host)
	}
	sort.Strings(hostList)
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed", "root": safeSiteURL(target), "depth": command.depth, "max_pages": command.maxPages, "pages": pages, "page_count": len(pages), "hosts": hostList, "errors": errors}, nil
}

func crawlableSitePath(path string) bool {
	path = strings.ToLower(path)
	for _, suffix := range []string{".css", ".js", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".pdf", ".zip", ".woff", ".woff2"} {
		if strings.HasSuffix(path, suffix) {
			return false
		}
	}
	return true
}

func removeCookieFile(path string) error {
	return withSiteFileLock(path, func() error {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("会话文件必须是普通文件且不能是符号链接")
		}
		return os.Remove(path)
	})
}

func renderSiteCommand(result map[string]any) string {
	if result["downloaded"] == true {
		return fmt.Sprintf("已保存：%v（%v bytes）\n", result["output"], result["bytes"])
	}
	if catalog, ok := result["catalog"].([]map[string]any); ok {
		var builder strings.Builder
		for _, item := range catalog {
			builder.WriteString(fmt.Sprintf("%v\t%v\t%v\t%v\n", item["service"], item["host"], item["name"], item["url"]))
		}
		return builder.String()
	}
	if hosts, ok := result["hosts"].([]string); ok {
		var builder strings.Builder
		builder.WriteString(fmt.Sprintf("发现 %v 个页面；官方子域名 %v 个\n", result["page_count"], len(hosts)))
		for _, host := range hosts {
			builder.WriteString(host + "\n")
		}
		return builder.String()
	}
	if response, ok := result["response"].(map[string]any); ok {
		return fmt.Sprintf("%v\t%v\n", response["title"], response["url"])
	}
	encoded, _ := json.MarshalIndent(result, "", "  ")
	return string(encoded) + "\n"
}
