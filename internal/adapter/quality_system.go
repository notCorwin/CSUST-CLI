package adapter

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"
)

const qualitySystemService = "quality-system"

func (a NativeSite) executeQualitySystem(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return businessCatalogFilter(qualitySystemService), nil
	}
	cookie, _, valueErr := businessValue(args, "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "config":
		return a.qualitySystemConfig(ctx, cookie)
	case "login":
		return a.qualitySystemLogin(ctx, args[1:], cookie)
	case "logout":
		return a.qualitySystemLogout(ctx, args[1:], cookie)
	case "status":
		return a.qualitySystemStatus(ctx, args[1:], cookie)
	case "profile":
		return a.qualitySystemProfile(ctx, args[1:], cookie)
	case "home":
		return a.qualitySystemProtectedQuery(ctx, args[1:], cookie, "home", "/api/manage/homePage/selectByLoginName", nil, "当前用户首页接口返回 code=200")
	case "semesters":
		return a.qualitySystemProtectedQuery(ctx, args[1:], cookie, "semesters", "/api/manage/selectopt/semesters", nil, "学期字典接口返回 code=200")
	case "organizations", "orgs":
		return a.qualitySystemProtectedQuery(ctx, args[1:], cookie, "organizations", "/api/manage/selectopt/orgns", []qualitySystemParamSpec{{"--keyword", "search", false}}, "组织字典接口返回 code=200")
	case "courses":
		return a.qualitySystemProtectedQuery(ctx, args[1:], cookie, "courses", "/api/manage/selectopt/courses", []qualitySystemParamSpec{{"--organization", "orgCode", false}, {"--keyword", "search", false}}, "课程字典接口返回 code=200")
	case "teachers":
		return a.qualitySystemProtectedQuery(ctx, args[1:], cookie, "teachers", "/api/manage/selectopt/teachers", []qualitySystemParamSpec{{"--organization", "orgCode", false}, {"--keyword", "search", false}}, "教师字典接口返回 code=200")
	case "roles":
		return a.qualitySystemProtectedQuery(ctx, args[1:], cookie, "roles", "/api/manage/selectopt/roles", nil, "角色字典接口返回 code=200")
	case "tasks":
		return a.qualitySystemProtectedQuery(ctx, args[1:], cookie, "tasks", "/api/tpk/tpk/getMytpktask", qualitySystemListParams, "当前用户听评课任务接口返回 code=200")
	case "results":
		return a.qualitySystemProtectedQuery(ctx, args[1:], cookie, "results", "/api/tpk/tpk/getTtpkListenresultList", qualitySystemListParams, "听评课结果列表接口返回 code=200")
	case "result":
		if _, requiredErr := businessRequired(args[1:], "--id", "result 必须提供 --id"); requiredErr != nil {
			return nil, requiredErr
		}
		return a.qualitySystemProtectedQuery(ctx, args[1:], cookie, "result", "/api/tpk/tpk/getTtpkListenresultListxq", []qualitySystemParamSpec{{"--id", "resultid", false}}, "听评课结果详情接口返回 code=200")
	case "improvements":
		return a.qualitySystemProtectedQuery(ctx, args[1:], cookie, "improvements", "/api/tpk/tpk/getTtpkImprovementsList", qualitySystemListParams, "教学改进报告接口返回 code=200")
	case "waitlist":
		return a.qualitySystemProtectedQuery(ctx, args[1:], cookie, "waitlist", "/api/tpk/tpk/getTtpkWaitListencourseList", qualitySystemListParams, "待听评课列表接口返回 code=200")
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "quality-system 只支持 config、login、logout、status、profile、home、semesters、organizations、courses、teachers、roles、tasks、results、result、improvements、waitlist、catalog"}
	}
}

type qualitySystemParamSpec struct {
	flag     string
	remote   string
	positive bool
}

var qualitySystemListParams = []qualitySystemParamSpec{
	{"--semester", "yeartermcode", false},
	{"--organization", "orgcode", false},
	{"--keyword", "searchss", false},
	{"--page", "page", true},
	{"--page-size", "limit", true},
}

func qualitySystemParams(args []string, specs []qualitySystemParamSpec) ([]pair, map[string]any, *siteError) {
	params := make([]pair, 0, len(specs))
	filters := map[string]any{}
	for _, spec := range specs {
		value, found, valueErr := businessValue(args, spec.flag)
		if valueErr != nil {
			return nil, nil, valueErr
		}
		if !found {
			continue
		}
		filterName := strings.ReplaceAll(strings.TrimPrefix(spec.flag, "--"), "-", "_")
		if spec.positive {
			number, numberErr := strconv.Atoi(value)
			if numberErr != nil || number < 1 {
				return nil, nil, &siteError{Code: "invalid_argument", Message: spec.flag + " 必须是正整数"}
			}
			value = strconv.Itoa(number)
			filters[filterName] = number
		} else {
			value = strings.TrimSpace(value)
			if value == "" {
				return nil, nil, &siteError{Code: "invalid_argument", Message: spec.flag + " 不能为空"}
			}
			filters[filterName] = value
		}
		params = append(params, pair{spec.remote, value})
	}
	return params, filters, nil
}

func (a NativeSite) qualitySystemProtectedQuery(ctx context.Context, args []string, cookie, operation, path string, specs []qualitySystemParamSpec, evidence string) (map[string]any, *siteError) {
	token, tokenPath, sessionErr := qualitySystemSession(args, cookie)
	if sessionErr != nil {
		return nil, sessionErr
	}
	if token == "" {
		return nil, &siteError{Code: "login_required", Message: "请先运行 quality-system login 或提供 --access-token"}
	}
	var params []pair
	filters := map[string]any{}
	if specs != nil {
		var queryErr *siteError
		params, filters, queryErr = qualitySystemParams(args, specs)
		if queryErr != nil {
			return nil, queryErr
		}
	}
	payload, requestErr := a.qualitySystemRequest(ctx, "POST", path, params, token, cookie)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := qualitySystemPayload(payload)
	if parseErr != nil {
		return nil, parseErr
	}
	if !qualitySystemSuccess(payload) {
		return nil, qualitySystemRejected(payload)
	}
	result := qualitySystemResult(operation, evidence)
	result["token_file"], result["filters"], result["api_code"] = tokenPath, filters, payload["code"]
	result["data"], result["raw"] = redactSiteJSON(payload["data"]), redactSiteJSON(payload)
	return result, nil
}

func (a NativeSite) qualitySystemRequest(ctx context.Context, method, path string, params []pair, token, cookie string) (map[string]any, *siteError) {
	headers := []pair{}
	if strings.TrimSpace(token) != "" {
		headers = append(headers, pair{"Authorization", "Bearer" + strings.TrimSpace(token)})
	}
	return businessRequest(ctx, qualitySystemService, method, path, params, nil, headers, businessRequestOptions{
		cookieFile: cookie, allowBusinessFailure: true,
	}, true, true)
}

func qualitySystemPayload(result map[string]any) (map[string]any, *siteError) {
	value, ok := businessData(result)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "教学质量保障系统响应不是 JSON 对象"}
	}
	payload, ok := value.(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "教学质量保障系统响应 JSON 结构无效"}
	}
	return payload, nil
}

func qualitySystemSuccess(payload map[string]any) bool {
	if success, ok := payload["success"].(bool); ok {
		return success
	}
	code := strings.TrimSpace(fmt.Sprint(payload["code"]))
	return code == "0" || code == "200"
}

func qualitySystemMessage(payload map[string]any) string {
	for _, key := range []string{"message", "msg", "error"} {
		if value, ok := payload[key]; ok && value != nil {
			if message := strings.TrimSpace(fmt.Sprint(value)); message != "" {
				return message
			}
		}
	}
	return ""
}

func qualitySystemRejected(payload map[string]any) *siteError {
	code := strings.TrimSpace(fmt.Sprint(payload["code"]))
	message := qualitySystemMessage(payload)
	if code == "21327" || code == "21325" || code == "401" {
		if message == "" {
			message = "教学质量保障系统会话已失效"
		}
		return &siteError{Code: "login_required", Message: message, Details: map[string]any{"remote_code": payload["code"]}}
	}
	if message == "" {
		message = "教学质量保障系统接口拒绝请求"
	}
	return &siteError{Code: "business_rejected", Message: message, Details: map[string]any{"remote_code": payload["code"]}}
}

func (a NativeSite) qualitySystemConfig(ctx context.Context, cookie string) (map[string]any, *siteError) {
	result, requestErr := a.qualitySystemRequest(ctx, "GET", "/api/manage/config/selectOne", nil, "", cookie)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := qualitySystemPayload(result)
	if parseErr != nil {
		return nil, parseErr
	}
	if !qualitySystemSuccess(payload) {
		return nil, qualitySystemRejected(payload)
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "教学质量保障系统配置缺少 data 对象"}
	}
	config, ok := data["config"].(map[string]any)
	if !ok {
		config = data
	}
	normalized := map[string]any{
		"title":                   config["title"],
		"logo":                    config["logo"],
		"captcha_enabled":         config["isOpenLoginVali"] == "是",
		"login_error_limit":       config["loginErrorNum"],
		"login_lock_minutes":      config["loginLockedTime"],
		"multiple_sessions":       config["userLogins"],
		"password_length":         config["pwdLength"],
		"password_rule":           config["pwdRule"],
		"force_password_change":   config["pwd_rule_check"],
		"source_fields_preserved": true,
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "公开配置接口返回系统标题、登录策略和密码规则",
		"service":  qualitySystemService, "operation": "config", "data": normalized,
		"raw": redactSiteJSON(config),
	}, nil
}

func qualitySystemSalt() (string, *siteError) {
	value, err := cryptorand.Int(cryptorand.Reader, big.NewInt(1000))
	if err != nil {
		return "", &siteError{Code: "authentication_failed", Message: "无法生成登录随机盐: " + err.Error()}
	}
	return strconv.FormatInt(value.Int64(), 10), nil
}

func (a NativeSite) qualitySystemCaptcha(ctx context.Context, cookie, filename string) (string, *siteError) {
	result, requestErr := a.execute(ctx, siteRequest{
		Service: qualitySystemService, Method: "GET", Path: "/api/manage/common/makeVeriCode",
		Params: []pair{{"t", time.Now().Format(time.RFC3339Nano)}}, CookieFile: cookie,
		Output: expandUserPath(filename), ReadOnly: true, Yes: true,
	})
	if requestErr != nil {
		return "", requestErr
	}
	serialNo, _ := result["serialno_internal"].(string)
	return strings.TrimSpace(serialNo), nil
}

func (a NativeSite) qualitySystemLogin(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	account, password, credentialErr := businessCredentials(args, "CSUST_QUALITY_SYSTEM_PASSWORD")
	if credentialErr != nil {
		return nil, credentialErr
	}
	config, configErr := a.qualitySystemConfig(ctx, cookie)
	if configErr != nil {
		return nil, configErr
	}
	configData, _ := config["data"].(map[string]any)
	captchaEnabled, _ := configData["captcha_enabled"].(bool)
	captcha, _, valueErr := businessValue(args, "--captcha")
	if valueErr != nil {
		return nil, valueErr
	}
	serialNo := ""
	if captchaEnabled {
		captchaImage, imageFound, imageErr := businessValue(args, "--captcha-image")
		if imageErr != nil {
			return nil, imageErr
		}
		if !imageFound || strings.TrimSpace(captchaImage) == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "登录验证码已开启，请提供 --captcha-image 保存验证码图片"}
		}
		if strings.TrimSpace(captcha) == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "登录验证码已开启，请提供 --captcha"}
		}
		var captchaErr *siteError
		serialNo, captchaErr = a.qualitySystemCaptcha(ctx, cookie, captchaImage)
		if captchaErr != nil {
			return nil, captchaErr
		}
	}
	salt, saltErr := qualitySystemSalt()
	if saltErr != nil {
		return nil, saltErr
	}
	result, requestErr := a.qualitySystemRequest(ctx, "POST", "/api/manage/doLogin", []pair{
		{"loginname", strings.TrimSpace(account)}, {"pwd", md5Hex(password + salt)},
		{"code", strings.TrimSpace(captcha)}, {"salt", salt}, {"serialNo", serialNo},
	}, "", cookie)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := qualitySystemPayload(result)
	if parseErr != nil {
		return nil, parseErr
	}
	if !qualitySystemSuccess(payload) {
		message := qualitySystemMessage(payload)
		if message == "" {
			message = "教学质量保障系统登录失败"
		}
		return nil, &siteError{Code: "authentication_failed", Message: message, Details: map[string]any{"remote_code": payload["code"]}}
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "authentication_failed", Message: "登录成功响应缺少用户会话数据"}
	}
	token, _ := data["accessToken"].(string)
	if strings.TrimSpace(token) == "" {
		token, _ = data["token"].(string)
	}
	if strings.TrimSpace(token) == "" {
		return nil, &siteError{Code: "authentication_failed", Message: "登录成功响应缺少访问令牌"}
	}
	_, tokenPath, sessionErr := qualitySystemSession(nil, cookie)
	if sessionErr != nil {
		return nil, sessionErr
	}
	if writeErr := atomicWrite(tokenPath, []byte(strings.TrimSpace(token)+"\n")); writeErr != nil {
		return nil, &siteError{Code: "session_error", Message: "登录成功但令牌保存失败: " + writeErr.Error(), Details: map[string]any{"token_file": tokenPath}}
	}
	profile, profileErr := a.qualitySystemProfilePayload(ctx, token, cookie)
	if profileErr != nil {
		return nil, &siteError{Code: "authentication_unverified", Message: "登录已返回令牌，但用户信息验证失败: " + profileErr.Error(), Details: map[string]any{"token_file": tokenPath}}
	}
	resultValue := qualitySystemResult("login", "登录接口返回访问令牌并通过用户信息回读验证")
	resultValue["username"], resultValue["token_file"] = strings.TrimSpace(account), tokenPath
	resultValue["user"] = redactSiteJSON(profile)
	return resultValue, nil
}

func qualitySystemTokenPath(cookie string) string {
	if strings.HasSuffix(cookie, ".cookies.txt") {
		return strings.TrimSuffix(cookie, ".cookies.txt") + ".token"
	}
	return cookie + ".token"
}

func qualitySystemSession(args []string, cookie string) (string, string, *siteError) {
	token, found, valueErr := businessValue(args, "--access-token")
	if valueErr != nil {
		return "", "", valueErr
	}
	if !found || strings.TrimSpace(token) == "" {
		token = os.Getenv("CSUST_QUALITY_SYSTEM_TOKEN")
	}
	_, cookiePath, resolveErr := resolveSite(siteRequest{Service: qualitySystemService, CookieFile: cookie})
	if resolveErr != nil {
		return "", "", resolveErr
	}
	tokenPath := qualitySystemTokenPath(cookiePath)
	if strings.TrimSpace(token) == "" {
		info, statErr := os.Lstat(tokenPath)
		if statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return "", tokenPath, &siteError{Code: "session_error", Message: "教学质量保障系统令牌文件必须是普通文件且不能是符号链接"}
			}
			content, readErr := os.ReadFile(tokenPath)
			if readErr != nil {
				return "", tokenPath, &siteError{Code: "session_error", Message: "无法读取教学质量保障系统令牌: " + readErr.Error()}
			}
			token = string(content)
		} else if !os.IsNotExist(statErr) {
			return "", tokenPath, &siteError{Code: "session_error", Message: "无法读取教学质量保障系统令牌: " + statErr.Error()}
		}
	}
	return strings.TrimSpace(token), tokenPath, nil
}

func (a NativeSite) qualitySystemProfilePayload(ctx context.Context, token, cookie string) (map[string]any, *siteError) {
	result, requestErr := a.qualitySystemRequest(ctx, "GET", "/api/manage/common/getCurrenUser", nil, token, cookie)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := qualitySystemPayload(result)
	if parseErr != nil {
		return nil, parseErr
	}
	if !qualitySystemSuccess(payload) {
		return nil, qualitySystemRejected(payload)
	}
	data, _ := payload["data"].(map[string]any)
	if nested, ok := data["data"].(map[string]any); ok {
		data = nested
	}
	if data == nil {
		return nil, &siteError{Code: "parse_error", Message: "用户信息响应缺少 data 对象"}
	}
	return data, nil
}

func qualitySystemResult(operation, evidence string) map[string]any {
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": evidence, "service": qualitySystemService, "operation": operation}
}

func (a NativeSite) qualitySystemProfile(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	token, tokenPath, sessionErr := qualitySystemSession(args, cookie)
	if sessionErr != nil {
		return nil, sessionErr
	}
	if token == "" {
		return nil, &siteError{Code: "login_required", Message: "请先运行 quality-system login 或提供 --access-token"}
	}
	profile, profileErr := a.qualitySystemProfilePayload(ctx, token, cookie)
	if profileErr != nil {
		return nil, profileErr
	}
	result := qualitySystemResult("profile", "当前用户接口返回 code=200")
	result["token_file"] = tokenPath
	result["data"] = redactSiteJSON(profile)
	return result, nil
}

func (a NativeSite) qualitySystemStatus(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	token, tokenPath, sessionErr := qualitySystemSession(args, cookie)
	if sessionErr != nil {
		return nil, sessionErr
	}
	if token == "" {
		result := qualitySystemResult("status", "本地没有教学质量保障系统令牌")
		result["logged_in"] = false
		return result, nil
	}
	profile, profileErr := a.qualitySystemProfilePayload(ctx, token, cookie)
	if profileErr != nil {
		if profileErr.Code == "login_required" {
			result := qualitySystemResult("status", "用户信息接口报告会话失效")
			result["logged_in"] = false
			result["token_file"] = tokenPath
			return result, nil
		}
		return nil, profileErr
	}
	result := qualitySystemResult("status", "用户信息接口返回 code=200")
	result["logged_in"], result["token_file"] = true, tokenPath
	result["user"] = redactSiteJSON(profile)
	return result, nil
}

func (a NativeSite) qualitySystemLogout(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	token, tokenPath, sessionErr := qualitySystemSession(args, cookie)
	if sessionErr != nil {
		return nil, sessionErr
	}
	var remote map[string]any
	if token != "" {
		payload, requestErr := a.qualitySystemRequest(ctx, "GET", "/api/manage/doLogout", nil, token, cookie)
		if requestErr != nil {
			return nil, requestErr
		}
		remote, requestErr = qualitySystemPayload(payload)
		if requestErr != nil {
			return nil, requestErr
		}
		if !qualitySystemSuccess(remote) && qualitySystemRejected(remote).Code != "login_required" {
			return nil, qualitySystemRejected(remote)
		}
	}
	if removeErr := removeCookieFile(tokenPath); removeErr != nil {
		return nil, &siteError{Code: "session_error", Message: "无法删除教学质量保障系统令牌: " + removeErr.Error()}
	}
	if _, cookiePath, resolveErr := resolveSite(siteRequest{Service: qualitySystemService, CookieFile: cookie}); resolveErr == nil {
		if removeErr := removeCookieFile(cookiePath); removeErr != nil {
			return nil, &siteError{Code: "session_error", Message: "无法删除教学质量保障系统 Cookie: " + removeErr.Error()}
		}
	}
	result := qualitySystemResult("logout", "远端退出（如有会话）并删除本地令牌/Cookie")
	result["logged_out"], result["token_file"] = true, tokenPath
	if remote != nil {
		result["remote_code"] = remote["code"]
	}
	return result, nil
}
