package adapter

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (a NativeSite) executeResearch(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "status" {
		statusArgs := args
		if len(statusArgs) > 0 {
			statusArgs = statusArgs[1:]
		}
		cookie, _, valueErr := businessValue(statusArgs, "--cookie-file")
		if valueErr != nil {
			return nil, valueErr
		}
		return a.executeServiceStatusWithOptions(ctx, "research", "status", "科研管理系统", businessRequestOptions{cookieFile: cookie})
	}
	if args[0] == "catalog" {
		result := businessCatalogFilter("research")
		result["roles"] = []map[string]any{{"role": "researcher", "label": "科研人员", "value": "01"}, {"role": "management", "label": "管理人员", "value": "02"}}
		return result, nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "login":
		return a.researchLogin(ctx, args[1:], cookie)
	case "logout":
		return a.researchLogout(ctx, args[1:], cookie)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "research 只支持 status、catalog、login、logout"}
	}
}

func researchRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "researcher", "科研人员", "01":
		return "01"
	case "management", "manager", "管理员", "管理人员", "02":
		return "02"
	case "03":
		return "03"
	default:
		return ""
	}
}

func researchLoginPage(body string) bool {
	return strings.Contains(body, "txtLoginName") && strings.Contains(body, "txtPassWord")
}

func researchFormFields(body string) ([]pair, *siteError) {
	document, err := parsePage(body)
	if err != nil {
		return nil, &siteError{Code: "parse_error", Message: "科研登录页解析失败: " + err.Error()}
	}
	form := document.first("form", "")
	if form == nil {
		return nil, &siteError{Code: "parse_error", Message: "科研登录页缺少表单"}
	}
	fields := make([]pair, 0)
	for _, input := range form.findAll("input") {
		name := input.attr("name")
		if name == "" {
			continue
		}
		typeName := strings.ToLower(firstNonEmpty(input.attr("type"), "text"))
		if typeName == "hidden" || name == "loginbutton" {
			fields = append(fields, pair{name, input.attr("value")})
		}
	}
	return fields, nil
}

func md5Hex(value string) string {
	digest := md5.Sum([]byte(value))
	return hex.EncodeToString(digest[:])
}

func (a NativeSite) researchLogin(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	role := researchRole(flagValue(args, "--role"))
	if role == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "login 必须提供 --role researcher 或 management"}
	}
	account, password, credentialErr := businessCredentials(args, "CSUST_RESEARCH_PASSWORD")
	if credentialErr != nil {
		return nil, credentialErr
	}
	page, requestErr := a.businessGet(ctx, "research", "/", nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	fields, fieldsErr := researchFormFields(businessBody(page))
	if fieldsErr != nil {
		return nil, fieldsErr
	}
	captcha := flagValue(args, "--captcha")
	if captcha == "" {
		return nil, businessCaptcha(ctx, "research", "/ValidateCode.aspx", "research-captcha.png", args, cookie)
	}
	encrypt, requestErr := a.businessGet(ctx, "research", "/AjaxAshx/EncryptString.ashx", []pair{{"strInput", account + "|" + captcha}, {"_", strconv.FormatInt(time.Now().UnixMilli(), 10)}}, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	parts := strings.Split(strings.TrimSpace(businessBody(encrypt)), "|")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, &siteError{Code: "parse_error", Message: "科研登录加密接口响应无效"}
	}
	fields = setFormField(fields, "h1", parts[0])
	fields = setFormField(fields, "h2", md5Hex(password))
	fields = setFormField(fields, "h3", parts[1])
	fields = setFormField(fields, "hRoleType", role)
	fields = setFormField(fields, "loginbutton", "登录")
	result, requestErr := businessRequest(ctx, "research", "POST", "/Login.aspx", nil, fields, []pair{{"Referer", safeResponseURL(page)}}, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	if strings.Contains(businessBody(result), "登录信息错误") || researchLoginPage(businessBody(result)) {
		return nil, &siteError{Code: "authentication_failed", Message: "科研管理系统登录失败", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "login-response"}}
	}
	evidence, probeErr := a.confirmBusinessLogin(ctx, "research", "/", cookie, researchLoginPage)
	if probeErr != nil {
		return nil, probeErr
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "Login.aspx-and-" + evidence, "service": "research", "operation": "login", "role": role, "username": account}, nil
}

func (a NativeSite) researchLogout(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "科研系统退出会话需要 --yes"}
	}
	if _, requestErr := businessRequest(ctx, "research", "POST", "/AjaxAshx/LoginOut.ashx", []pair{{"t", strconv.FormatInt(time.Now().UnixMilli(), 10)}}, nil, nil, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true); requestErr != nil {
		return nil, requestErr
	}
	page, requestErr := a.businessGet(ctx, "research", "/", nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	if !researchLoginPage(businessBody(page)) {
		return nil, &siteError{Code: "logout_unconfirmed", Message: "科研系统退出请求已发送，但登录页未重新出现", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "login-page-probe"}}
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "LoginOut-and-login-page", "service": "research", "operation": "logout", "logged_out": true}, nil
}

func (a NativeSite) executeTransportInfo(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "status" {
		statusArgs := args
		if len(statusArgs) > 0 {
			statusArgs = statusArgs[1:]
		}
		cookie, _, valueErr := businessValue(statusArgs, "--cookie-file")
		if valueErr != nil {
			return nil, valueErr
		}
		return a.executeServiceStatusWithOptions(ctx, "transport-info", "status", "交通学院综合信息服务", businessRequestOptions{cookieFile: cookie})
	}
	if args[0] == "catalog" {
		return businessCatalogFilter("transport-info"), nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "login":
		return a.transportInfoLogin(ctx, args[1:], cookie)
	case "logout":
		return a.transportInfoLogout(ctx, args[1:], cookie)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "transport-info 只支持 status、catalog、login、logout"}
	}
}

func transportInfoLoginPage(body string) bool {
	return strings.Contains(body, "系统登录") || strings.Contains(body, "UserName") && strings.Contains(body, "PassWord")
}

func (a NativeSite) transportInfoLogin(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	account, password, credentialErr := businessCredentials(args, "CSUST_TRANSPORT_INFO_PASSWORD")
	if credentialErr != nil {
		return nil, credentialErr
	}
	page, requestErr := a.businessGet(ctx, "transport-info", "/Login/Index", nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	captcha := flagValue(args, "--captcha")
	if captcha == "" {
		return nil, businessCaptcha(ctx, "transport-info", "/Login/GetVerifyCode", "transport-info-captcha.png", args, cookie)
	}
	result, requestErr := businessRequest(ctx, "transport-info", "POST", "/Login/CheckLogin", nil, []pair{{"username", account}, {"password", password}, {"code", captcha}}, []pair{{"X-Requested-With", "XMLHttpRequest"}, {"Accept", "application/json, text/javascript, */*; q=0.01"}, {"Referer", safeResponseURL(page)}}, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return nil, parseErr
	}
	if strings.ToLower(fmt.Sprint(payload["state"])) != "success" {
		return nil, &siteError{Code: "authentication_failed", Message: firstNonEmpty(fmt.Sprint(payload["message"]), "交通学院综合信息登录失败"), Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "CheckLogin-response", "reason": payload["data"]}}
	}
	evidence, probeErr := a.confirmBusinessLogin(ctx, "transport-info", "/Home/Index", cookie, transportInfoLoginPage)
	if probeErr != nil {
		return nil, probeErr
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "CheckLogin-and-" + evidence, "service": "transport-info", "operation": "login", "username": account}, nil
}

func (a NativeSite) transportInfoLogout(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "交通学院综合信息退出会话需要 --yes"}
	}
	_, cookiePath, resolveErr := resolveSite(siteRequest{Service: "transport-info", CookieFile: cookie})
	if resolveErr != nil {
		return nil, resolveErr
	}
	if removeErr := removeCookieFile(cookiePath); removeErr != nil {
		return nil, &siteError{Code: "cookie_write_failed", Message: removeErr.Error()}
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "local-cookie-removed", "service": "transport-info", "operation": "logout", "logged_out": true, "cookie_file": cookiePath}, nil
}

func businessCaptcha(ctx context.Context, service, path, filename string, args []string, cookie string) *siteError {
	_, cookiePath, resolveErr := resolveSite(siteRequest{Service: service, CookieFile: cookie})
	if resolveErr != nil {
		return resolveErr
	}
	imagePath := flagValue(args, "--captcha-image")
	if imagePath == "" {
		imagePath = filepath.Join(filepath.Dir(cookiePath), filename)
	}
	imagePath = expandUserPath(imagePath)
	if _, imageErr := (NativeSite{}).execute(ctx, siteRequest{Service: service, Method: "GET", Path: path, Params: []pair{{"time", strconv.FormatInt(time.Now().UnixNano(), 10)}}, CookieFile: cookie, Output: imagePath, ReadOnly: true, Yes: true}); imageErr != nil {
		return imageErr
	}
	return &siteError{Code: "captcha_required", Message: "登录需要验证码，请查看图片后提供 --captcha", Details: map[string]any{"captcha_image": imagePath, "submitted": false, "confirmed": false, "evidence": "captcha-image"}}
}
