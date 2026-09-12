package adapter

import (
	"context"
	"crypto/des"
	"encoding/hex"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type unionRole struct {
	name       string
	label      string
	loginType  string
	capability string
}

var unionRoles = []unionRole{
	{"representative", "代表", "0", "提出提案、提案附议"},
	{"department", "承办部门", "1", "承办提案、答复提案"},
	{"admin", "审核管理员", "2", "审核立案、管理提案"},
	{"leader", "领导/委员/部门负责人", "3", "审议提案"},
	{"delegation-leader", "代表团长", "4", "审议提案"},
}

var unionModules = []map[string]any{
	{"name": "proposal", "label": "电子提案系统", "path": "/front/news.do?dispatch=sysproposal&proposal_type=1", "capability": "提案提出、附议、承办、答复、审议和管理"},
	{"name": "branch-union", "label": "二级分工会系统", "path": "/front/news.do?dispatch=listByType_&ntype_id=0903", "capability": "二级分工会信息"},
	{"name": "membership", "label": "会员会籍系统", "path": "/center/center.do?dispatch=centerindex", "capability": "会员会籍"},
	{"name": "association", "label": "协会管理系统", "path": "/front/news.do?dispatch=listByType_&ntype_id=0901", "capability": "协会管理"},
	{"name": "activity-registration", "label": "活动报名系统", "path": "/center/bmreg.do?dispatch=tobmreglist", "capability": "工会活动报名"},
	{"name": "survey", "label": "在线调查系统", "path": "/front/news.do?dispatch=listByType_&ntype_id=1103&statistic_type=1", "capability": "在线调查"},
	{"name": "quiz", "label": "知识竞答系统", "path": "/front/news.do?dispatch=listByType_&ntype_id=1103&statistic_type=1&ispa=1", "capability": "知识竞答"},
	{"name": "benefits", "label": "工会福利平台", "path": "/center/welbut.do?dispatch=freeactlist", "capability": "工会福利"},
}

func (a NativeSite) executeUnion(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" || args[0] == "modules" || args[0] == "roles" {
		result := businessCatalogFilter("union")
		result["modules"] = unionModules
		result["roles"] = unionRoleCatalog()
		result["operations"] = []string{"catalog", "modules", "roles", "organizations", "branches", "associations", "organization", "login", "logout"}
		if len(args) > 0 && args[0] != "catalog" {
			result["operation"] = args[0]
		}
		return result, nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "organizations":
		return a.unionOrganizations(ctx, args[1:], cookie, "")
	case "branches":
		return a.unionOrganizations(ctx, args[1:], cookie, "branch")
	case "associations":
		return a.unionOrganizations(ctx, args[1:], cookie, "association")
	case "organization":
		return a.unionOrganization(ctx, args[1:], cookie)
	case "login":
		return a.unionLogin(ctx, args[1:], cookie)
	case "logout":
		return a.unionLogout(ctx, args[1:], cookie)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "union 只支持 catalog、modules、roles、organizations、branches、associations、organization、login、logout"}
	}
}

func unionDirectoryKind(value string) (string, *siteError) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "all", "全部":
		return "", nil
	case "branch", "branches", "branch-union", "分工会", "二级分工会":
		return "branch", nil
	case "association", "associations", "club", "协会", "社团":
		return "association", nil
	default:
		return "", &siteError{Code: "invalid_argument", Message: "--kind 只支持 branch、association 或 all"}
	}
}

func unionDirectoryPath(kind string) string {
	if kind == "association" {
		return "/front/news.do?dispatch=listByType_&ntype_id=0901"
	}
	return "/front/news.do?dispatch=listByType_&ntype_id=0903"
}

func (a NativeSite) unionOrganizations(ctx context.Context, args []string, cookie, forcedKind string) (map[string]any, *siteError) {
	kindValue, _, valueErr := businessValue(args, "--kind")
	if valueErr != nil {
		return nil, valueErr
	}
	if forcedKind != "" && kindValue != "" {
		parsedKind, kindErr := unionDirectoryKind(kindValue)
		if kindErr != nil {
			return nil, kindErr
		}
		if parsedKind != forcedKind {
			return nil, &siteError{Code: "invalid_argument", Message: "该命令的 --kind 与命令名冲突"}
		}
	}
	kind := forcedKind
	if kind == "" {
		kind, valueErr = unionDirectoryKind(kindValue)
		if valueErr != nil {
			return nil, valueErr
		}
	}
	keyword, _, valueErr := businessValue(args, "--keyword")
	if valueErr != nil {
		return nil, valueErr
	}

	kinds := []string{kind}
	if kind == "" {
		kinds = []string{"branch", "association"}
	}
	items := make([]map[string]any, 0)
	for _, currentKind := range kinds {
		result, requestErr := a.businessGet(ctx, "union", unionDirectoryPath(currentKind), nil, businessRequestOptions{cookieFile: cookie})
		if requestErr != nil {
			return nil, requestErr
		}
		document, parseErr := parsePage(businessBody(result))
		if parseErr != nil {
			return nil, &siteError{Code: "parse_error", Message: "工会组织目录解析失败: " + parseErr.Error()}
		}
		items = append(items, unionDirectoryItems(document, safeResponseURL(result), currentKind)...)
	}
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		filtered := items[:0]
		for _, item := range items {
			if strings.Contains(strings.ToLower(item["name"].(string)), strings.ToLower(keyword)) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "工会公开分工会/协会目录页面返回组织卡片",
		"service":  "union", "operation": "organizations", "kind": firstNonEmpty(kind, "all"), "keyword": keyword,
		"data": items, "total": len(items),
	}, nil
}

func unionDirectoryItems(document *pageNode, pageURL, kind string) []map[string]any {
	items := make([]map[string]any, 0)
	seen := map[string]bool{}
	for _, node := range document.findAll("a") {
		if !strings.Contains(" "+node.attr("class")+" ", " GH-mian-card1 ") {
			continue
		}
		target := resolvePageURL(pageURL, node.attr("href"))
		parsed, err := url.Parse(target)
		if err != nil || parsed.Query().Get("dispatch") != "shetuanMain" {
			continue
		}
		id := strings.TrimSpace(parsed.Query().Get("ntype_id"))
		if id == "" || seen[id] {
			continue
		}
		name := strings.TrimSpace(pageDisplayText(libraryRemoteNodeWithClass(node, "text1")))
		if name == "" {
			name = strings.TrimSpace(node.attr("title"))
		}
		if name == "" {
			continue
		}
		items = append(items, map[string]any{"id": id, "name": name, "kind": kind, "url": pagePath(pageURL, target)})
		seen[id] = true
	}
	sort.Slice(items, func(i, j int) bool { return items[i]["name"].(string) < items[j]["name"].(string) })
	return items
}

func (a NativeSite) unionOrganization(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	id, requiredErr := businessRequired(args, "--id", "organization 必须提供 --id")
	if requiredErr != nil {
		return nil, requiredErr
	}
	if _, err := strconv.Atoi(strings.TrimSpace(id)); err != nil {
		return nil, &siteError{Code: "invalid_argument", Message: "organization --id 必须是数字"}
	}
	result, requestErr := a.businessGet(ctx, "union", "/front/news.do", []pair{{"dispatch", "shetuanMain"}, {"ntype_id", strings.TrimSpace(id)}}, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	document, parseErr := parsePage(businessBody(result))
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "工会组织详情解析失败: " + parseErr.Error()}
	}
	data := map[string]any{"id": strings.TrimSpace(id), "url": pagePath(safeResponseURL(result), safeResponseURL(result))}
	for _, box := range document.findAll("") {
		if !strings.Contains(" "+box.attr("class")+" ", " pc-user-box2 ") {
			continue
		}
		label := strings.TrimRight(strings.TrimSpace(pageDisplayText(libraryRemoteNodeWithClass(box, "info-text1"))), " ：:")
		value := strings.TrimSpace(pageDisplayText(libraryRemoteNodeWithClass(box, "info-text2")))
		if label == "" || value == "" {
			continue
		}
		switch label {
		case "分工会简介", "协会简介", "简介":
			data["description"] = value
		case "所辖部门", "活动地点", "地点":
			data["department"] = value
		case "分工会主席", "协会负责人", "负责人":
			data["leader"] = value
		default:
			if fields, ok := data["fields"].(map[string]any); ok {
				fields[label] = value
			} else {
				data["fields"] = map[string]any{label: value}
			}
		}
	}
	if name := strings.TrimSpace(pageDisplayText(libraryRemoteNodeWithClass(document, "userbox-text1"))); name != "" {
		data["name"] = name
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "工会公开组织详情页面返回组织资料",
		"service":  "union", "operation": "organization", "data": data,
	}, nil
}

func unionRoleCatalog() []map[string]any {
	result := make([]map[string]any, 0, len(unionRoles))
	for _, role := range unionRoles {
		result = append(result, map[string]any{"role": role.name, "label": role.label, "login_type": role.loginType, "capability": role.capability})
	}
	return result
}

func findUnionRole(value string) (unionRole, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	aliases := map[string]string{
		"代表": "representative", "我 是 代 表": "representative", "representatives": "representative",
		"承办部门": "department", "部门": "department", "审核": "admin", "管理员": "admin", "审核管理员": "admin",
		"领导": "leader", "委员": "leader", "代表团长": "delegation-leader", "代表团": "delegation-leader",
	}
	if alias, ok := aliases[normalized]; ok {
		normalized = alias
	}
	for _, role := range unionRoles {
		if role.name == normalized {
			return role, true
		}
	}
	return unionRole{}, false
}

func unionEncryptedPassword(password string) (string, *siteError) {
	block, err := des.NewCipher([]byte("gjrsuppo"))
	if err != nil {
		return "", &siteError{Code: "protocol_error", Message: "工会登录密码加密失败"}
	}
	padding := des.BlockSize - len([]byte(password))%des.BlockSize
	plain := append([]byte(password), make([]byte, padding)...)
	for index := len(plain) - padding; index < len(plain); index++ {
		plain[index] = byte(padding)
	}
	ciphertext := make([]byte, len(plain))
	for offset := 0; offset < len(plain); offset += des.BlockSize {
		block.Encrypt(ciphertext[offset:offset+des.BlockSize], plain[offset:offset+des.BlockSize])
	}
	return hex.EncodeToString(ciphertext), nil
}

func (a NativeSite) unionLogin(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	role, ok := findUnionRole(flagValue(args, "--role"))
	if !ok {
		return nil, &siteError{Code: "invalid_argument", Message: "login 必须提供 --role representative、department、admin、leader 或 delegation-leader"}
	}
	username, password, credentialErr := businessCredentials(args, "CSUST_UNION_PASSWORD")
	if credentialErr != nil {
		return nil, credentialErr
	}
	referer := "https://gonghui.csust.edu.cn/front/proposal.do?dispatch=toProposalLogin&proposal_type=1&login_type=" + role.loginType
	page, requestErr := a.businessGet(ctx, "union", "/front/user.do", []pair{{"dispatch", "toajaxlogin"}, {"login_type", role.loginType}, {"proposal_type", "1"}, {"_", strconv.FormatInt(time.Now().UnixMilli(), 10)}}, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	document, parseErr := parsePage(businessBody(page))
	if parseErr != nil || len(document.findAll("form")) == 0 {
		return nil, &siteError{Code: "parse_error", Message: "工会登录接口没有返回登录表单"}
	}
	captcha := flagValue(args, "--captcha")
	if captcha == "" {
		return nil, a.unionCaptcha(ctx, args, cookie)
	}
	encrypted, encryptErr := unionEncryptedPassword(password)
	if encryptErr != nil {
		return nil, encryptErr
	}
	result, requestErr := businessRequest(ctx, "union", "POST", "/front/user.do", []pair{{"dispatch", "ajaxlogin"}}, []pair{
		{"admin_id", username}, {"admin_pwd", encrypted}, {"identifying_code", captcha},
		{"login_type", role.loginType}, {"proposal_type", "1"}, {"time", time.Now().Format(time.RFC1123)},
	}, []pair{{"Referer", referer}}, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	if failure := unionLoginFailure(strings.TrimSpace(businessBody(result))); failure != nil {
		failure.Details = map[string]any{"submitted": true, "confirmed": false, "evidence": "ajaxlogin-response"}
		return nil, failure
	}
	session, requestErr := a.businessGet(ctx, "union", "/center/center.do", []pair{{"dispatch", "getCenterSessionName"}, {"time", strconv.FormatInt(time.Now().UnixMilli(), 10)}}, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, &siteError{Code: "authentication_failed", Message: "工会登录成功响应已返回，但会话探针失败", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "session-probe-error"}}
	}
	sessionName := strings.TrimSpace(businessBody(session))
	if sessionName == "" {
		return nil, &siteError{Code: "authentication_failed", Message: "工会登录响应成功，但未确认业务会话", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "empty-session-probe"}}
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "ajaxlogin=0-and-center-session", "service": "union", "operation": "login", "role": role.name, "username": username, "session": sessionName}, nil
}

func unionLoginFailure(body string) *siteError {
	switch {
	case body == "0":
		return nil
	case body == "-1":
		return &siteError{Code: "authentication_failed", Message: "工会登录失败，单位信息不存在"}
	case body == "-4":
		return &siteError{Code: "captcha_invalid", Message: "工会登录验证码错误"}
	case body == "-5":
		return &siteError{Code: "authentication_failed", Message: "工会用户名或密码错误"}
	case body == "-6":
		return &siteError{Code: "permission_denied", Message: "当前账号不是该角色，不能登录电子提案系统"}
	case strings.HasPrefix(body, "-3#"):
		return &siteError{Code: "account_locked", Message: "工会账号连续登录失败，已被锁定"}
	case body == "":
		return &siteError{Code: "authentication_failed", Message: "工会登录响应为空"}
	default:
		return &siteError{Code: "authentication_failed", Message: "工会登录响应未确认成功"}
	}
}

func (a NativeSite) unionCaptcha(ctx context.Context, args []string, cookie string) *siteError {
	_, cookiePath, resolveErr := resolveSite(siteRequest{Service: "union", CookieFile: cookie})
	if resolveErr != nil {
		return resolveErr
	}
	imagePath := flagValue(args, "--captcha-image")
	if imagePath == "" {
		imagePath = filepath.Join(filepath.Dir(cookiePath), "union-captcha.png")
	}
	imagePath = expandUserPath(imagePath)
	if _, imageErr := (NativeSite{}).execute(ctx, siteRequest{Service: "union", Method: "GET", Path: "/center/center.do", Params: []pair{{"dispatch", "generateImage"}, {"admin_id", strconv.FormatInt(time.Now().UnixNano()%100000, 10)}}, CookieFile: cookie, Output: imagePath, ReadOnly: true, Yes: true}); imageErr != nil {
		return imageErr
	}
	return &siteError{Code: "captcha_required", Message: "工会登录需要验证码，请查看图片后提供 --captcha", Details: map[string]any{"captcha_image": imagePath, "submitted": false, "confirmed": false, "evidence": "generateImage"}}
}

func (a NativeSite) unionLogout(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "工会退出会话需要 --yes"}
	}
	if _, requestErr := businessRequest(ctx, "union", "GET", "/logout.jsp", nil, nil, nil, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true); requestErr != nil {
		return nil, requestErr
	}
	session, requestErr := a.businessGet(ctx, "union", "/center/center.do", []pair{{"dispatch", "getCenterSessionName"}, {"time", strconv.FormatInt(time.Now().UnixMilli(), 10)}}, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	if strings.TrimSpace(businessBody(session)) != "" {
		return nil, &siteError{Code: "logout_unconfirmed", Message: "工会退出请求已发送，但会话仍存在", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "session-probe-not-empty"}}
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "logout-and-empty-session", "service": "union", "operation": "logout", "logged_out": true}, nil
}
