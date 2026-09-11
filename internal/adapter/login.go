package adapter

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	authServerBase       = "https://authserver.csust.edu.cn"
	authLoginPath        = "/authserver/login"
	authCaptchaCheckPath = "/authserver/checkNeedCaptcha.htl"
	authCaptchaPath      = "/authserver/getCaptcha.htl"
	academicSSOPath      = "/sso.jsp"
	academicProbePath    = "/jsxsd/xskb/xskb_list.do"
	casAESChars          = "ABCDEFGHJKMNPQRSTWXYZabcdefhijkmnprstwxyz2345678"
)

type loginOptions struct {
	username      string
	password      string
	auth          string
	captcha       string
	captchaImage  string
	passwordStdin bool
	mobile        string
	dynamicCode   string
	qrImage       string
	sendCode      bool
	yes           bool
}

func (a NativeSite) runLoginCommand(ctx context.Context, args []string, jsonMode bool) (bool, []byte, []byte, int, error) {
	if len(args) == 0 || (args[0] != "login" && args[0] != "logout") || containsHelp(args[1:]) {
		return false, nil, nil, 0, nil
	}
	if args[0] == "logout" {
		result, logoutErr := a.logoutAcademic(ctx)
		return loginOutput(jsonMode, result, logoutErr)
	}
	options, parseErr := parseLoginOptions(args[1:])
	if parseErr != nil {
		return loginOutput(jsonMode, nil, parseErr)
	}
	result, err := a.loginAcademic(ctx, options)
	return loginOutput(jsonMode, result, err)
}

func (a NativeSite) logoutAcademic(ctx context.Context) (map[string]any, *siteError) {
	cookiePath := academicCookiePath()
	result, requestErr := a.execute(ctx, siteRequest{
		Service: "academic", Path: "/jsxsd/xk/LoginToXk?method=exit&tktime=" + strconv.FormatInt(time.Now().UnixMilli(), 10),
		Method: "GET", CookieFile: cookiePath, ReadOnly: true, Yes: true,
	})
	if requestErr != nil {
		_ = removeCookieFile(cookiePath)
		return nil, requestErr
	}
	response, _ := result["response"].(map[string]any)
	status, _ := response["status"].(int)
	body, _ := response["body_internal"].(string)
	if body == "" {
		body, _ = response["body"].(string)
	}
	confirmed, evidence := false, "unknown"
	switch {
	case status == http.StatusNoContent:
		confirmed, evidence = true, "no_content"
	case loginPageBody(body):
		confirmed, evidence = true, "login_page"
	case body != "":
		if state, known, _ := businessState([]byte(body), fmt.Sprint(response["content_type"])); known {
			confirmed, evidence = state, "business"
		} else if successMessage(body) {
			confirmed, evidence = true, "html_message"
		}
	case response["json"] != nil:
		if state, known := jsonBusinessState(response["json"]); known {
			confirmed, evidence = state, "business"
		}
	}
	if !confirmed {
		_ = removeCookieFile(cookiePath)
		return nil, &siteError{Code: "mutation_unverified", Message: "远端退出请求已发送但未取得成功证据", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": evidence, "response": response}}
	}
	if err := removeCookieFile(cookiePath); err != nil {
		return nil, &siteError{Code: "cookie_write_failed", Message: err.Error(), Details: map[string]any{"submitted": true, "confirmed": true, "evidence": evidence}}
	}
	result["ok"], result["submitted"], result["confirmed"] = true, true, true
	result["evidence"], result["logged_out"], result["cookie_file"] = evidence, true, cookiePath
	return result, nil
}

func loginOutput(jsonMode bool, result map[string]any, err *siteError) (bool, []byte, []byte, int, error) {
	if err != nil {
		if jsonMode {
			return true, errorJSON(err), nil, 2, nil
		}
		return true, nil, []byte("错误: " + err.Error() + "\n"), 2, nil
	}
	if jsonMode {
		return true, mustJSON(result), nil, 0, nil
	}
	if result["logged_out"] == true {
		return true, []byte("已退出登录\n"), nil, 0, nil
	}
	return true, []byte(fmt.Sprintf("登录成功：%v\n会话已保存：%v\n", result["username"], result["cookie_file"])), nil, 0, nil
}

func parseLoginOptions(args []string) (loginOptions, *siteError) {
	options := loginOptions{auth: "auto"}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		value := ""
		if strings.HasPrefix(arg, "--") {
			if name, inline, found := strings.Cut(arg, "="); found {
				arg, value = name, inline
			} else if index+1 < len(args) && !strings.HasPrefix(args[index+1], "--") {
				index++
				value = args[index]
			}
		}
		switch arg {
		case "--username":
			options.username = value
		case "--auth":
			options.auth = value
		case "--captcha":
			options.captcha = value
		case "--captcha-image":
			options.captchaImage = value
		case "--password-stdin":
			options.passwordStdin = true
		case "--json":
		default:
			return loginOptions{}, &siteError{Code: "invalid_argument", Message: "login 参数无效: " + arg}
		}
	}
	if options.auth != "auto" && options.auth != "sso" && options.auth != "local" {
		return loginOptions{}, &siteError{Code: "invalid_argument", Message: "--auth 必须是 auto、sso 或 local"}
	}
	if options.passwordStdin {
		password, err := readBoundedSiteInput(os.Stdin)
		if err != nil {
			if errors.Is(err, errSiteRequestTooLarge) {
				return loginOptions{}, siteRequestTooLarge("标准输入密码")
			}
			return loginOptions{}, &siteError{Code: "credentials_required", Message: "无法读取标准输入密码: " + err.Error()}
		}
		options.password = strings.TrimRight(string(password), "\r\n")
	}
	return options, nil
}

func (a NativeSite) loginAcademic(ctx context.Context, options loginOptions) (map[string]any, *siteError) {
	account, password, err := credentialsGo(options.username, options.password)
	if err != nil {
		return nil, err
	}
	if options.captcha == "" {
		options.captcha = envValueGo("CSUST_CAPTCHA")
	}
	baseValue := os.Getenv("CSUST_BASE_URL")
	if baseValue == "" {
		baseValue = "http://xk.csust.edu.cn/"
	}
	base, parseErr := url.Parse(baseValue)
	if parseErr != nil || base == nil || base.Host == "" || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Scheme != "http" && base.Scheme != "https") {
		return nil, &siteError{Code: "invalid_path", Message: "CSUST_BASE_URL 地址无效"}
	}
	base.Path, base.RawQuery, base.Fragment = "/", "", ""
	if options.auth == "sso" || options.auth == "auto" {
		result, loginErr := a.loginSSO(ctx, base, account, password, options)
		if loginErr == nil {
			return result, nil
		}
		if options.auth == "sso" || loginErr.Code != "network_error" {
			return nil, loginErr
		}
	}
	return a.loginLocal(ctx, base, account, password, options)
}

func (a NativeSite) loginSSO(ctx context.Context, serviceTarget *url.URL, account, password string, options loginOptions) (map[string]any, *siteError) {
	probe := *serviceTarget
	probe.Path, probe.RawQuery, probe.Fragment = academicProbePath, "", ""
	return a.loginSSOWith(ctx, serviceTarget, &probe, account, password, options, academicCookiePath(), true)
}

func (a NativeSite) loginSSOService(ctx context.Context, serviceTarget *url.URL, cookiePath string, options loginOptions) (map[string]any, *siteError) {
	if options.auth != "dynamic" && (options.mobile != "" || options.dynamicCode != "" || options.sendCode) {
		return nil, &siteError{Code: "invalid_argument", Message: "--mobile、--dynamic-code、--send-code 只适用于 --auth dynamic"}
	}
	if options.auth != "qr" && options.qrImage != "" {
		return nil, &siteError{Code: "invalid_argument", Message: "--qr-image 只适用于 --auth qr"}
	}
	if options.auth != "sso" && options.auth != "auto" && options.passwordStdin {
		return nil, &siteError{Code: "invalid_argument", Message: "--password-stdin 只适用于账号密码登录"}
	}
	callback := ssoServiceTarget(serviceTarget)
	if options.auth == "qr" {
		return a.loginSSOQR(ctx, callback, serviceTarget, cookiePath, options)
	}
	if options.auth == "dynamic" {
		return a.loginSSODynamic(ctx, callback, serviceTarget, cookiePath, options)
	}
	if options.passwordStdin {
		value, readErr := readBoundedSiteInput(os.Stdin)
		if readErr != nil {
			if errors.Is(readErr, errSiteRequestTooLarge) {
				return nil, siteRequestTooLarge("标准输入密码")
			}
			return nil, &siteError{Code: "credentials_required", Message: "无法读取标准输入密码: " + readErr.Error()}
		}
		options.password = strings.TrimRight(string(value), "\r\n")
	}
	account, password, err := credentialsGo(options.username, options.password)
	if err != nil {
		return nil, err
	}
	if options.captcha == "" {
		options.captcha = envValueGo("CSUST_CAPTCHA")
	}
	result, loginErr := a.loginSSOWith(ctx, callback, serviceTarget, account, password, options, cookiePath, false)
	if result != nil {
		result["service"] = safeSiteURL(serviceTarget)
	}
	return result, loginErr
}

func ssoServiceTarget(target *url.URL) *url.URL {
	if target == nil || !strings.EqualFold(target.Hostname(), "ehall.csust.edu.cn") {
		return target
	}
	portal := *target
	portal.Path, portal.RawQuery, portal.Fragment = "/index.html", "", "/"
	callback := *target
	callback.Path, callback.RawQuery, callback.Fragment = "/login", "", ""
	query := callback.Query()
	query.Set("portalService", portal.String())
	callback.RawQuery = query.Encode()
	return &callback
}

func (a NativeSite) loginSSOPage(ctx context.Context, callback, session *url.URL, cookiePath, formID, loginType string) (*pageNode, string, *siteError) {
	authURL, _ := url.Parse(authServerBase + authLoginPath)
	query := authURL.Query()
	query.Set("service", callback.String())
	if loginType != "" {
		query.Set("type", loginType)
	}
	authURL.RawQuery = query.Encode()
	result, requestErr := a.loginHTTP(ctx, siteRequest{Target: authURL, SessionTarget: session, CookieFile: cookiePath, Method: "GET", ReadOnly: true, AllowSSO: true, Yes: true})
	if requestErr != nil {
		return nil, "", requestErr
	}
	body, responseURL, bodyErr := loginBody(result)
	if bodyErr != nil {
		return nil, "", bodyErr
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, "", &siteError{Code: "authentication_failed", Message: "统一认证登录页解析失败: " + parseErr.Error()}
	}
	form := document.first("form", formID)
	if form == nil {
		return nil, "", &siteError{Code: "authentication_failed", Message: "统一认证登录页缺少" + formID + "表单"}
	}
	return form, responseURL, nil
}

func authServerTarget(session *url.URL, path string) *url.URL {
	target := *session
	target.Scheme, target.Host, target.Path, target.RawQuery, target.Fragment = "https", "authserver.csust.edu.cn", path, "", ""
	return &target
}

func (a NativeSite) requiredAuthCaptcha(ctx context.Context, session url.URL, cookiePath, output, message string) (string, *siteError) {
	if output == "" {
		output = filepath.Join(filepath.Dir(cookiePath), "authserver-captcha.png")
	}
	path, fetchErr := a.fetchLoginCaptcha(ctx, session, cookiePath, true, output)
	if fetchErr != nil {
		return "", fetchErr
	}
	return "", &siteError{Code: "captcha_required", Message: message, Details: map[string]any{"captcha_image": path}}
}

func ssoFormAction(form *pageNode, responseURL, serviceURL string) (*url.URL, *siteError) {
	actionValue := form.attr("action")
	if actionValue == "" {
		actionValue = responseURL
	}
	action := resolvePageURL(responseURL, actionValue)
	actionURL, actionErr := validatePageActionTarget(action, responseURL, false)
	if actionErr != nil {
		return nil, actionErr
	}
	if !strings.EqualFold(actionURL.Host, "authserver.csust.edu.cn") {
		return nil, &siteError{Code: "invalid_path", Message: "统一认证表单地址不是认证服务器"}
	}
	query := actionURL.Query()
	if query.Get("service") == "" {
		query.Set("service", serviceURL)
		actionURL.RawQuery = query.Encode()
	}
	return actionURL, nil
}

func (a NativeSite) loginSSODynamic(ctx context.Context, callback, serviceTarget *url.URL, cookiePath string, options loginOptions) (map[string]any, *siteError) {
	mobile := strings.TrimSpace(options.mobile)
	if mobile == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "动态码登录必须提供 --mobile"}
	}
	if options.sendCode && options.dynamicCode != "" {
		return nil, &siteError{Code: "invalid_argument", Message: "--send-code 不能与 --dynamic-code 同时使用"}
	}
	if options.sendCode && !options.yes {
		return nil, &siteError{Code: "confirmation_required", Message: "发送动态码会触发短信，请加 --yes"}
	}
	if !options.sendCode && strings.TrimSpace(options.dynamicCode) == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "动态码登录必须提供 --dynamic-code，或先使用 --send-code --yes"}
	}
	session := *serviceTarget
	session.Path, session.RawQuery, session.Fragment = "/", "", ""
	form, responseURL, pageErr := a.loginSSOPage(ctx, callback, &session, cookiePath, "phoneFromId", "dynamicLogin")
	if pageErr != nil {
		return nil, pageErr
	}
	captcha := options.captcha
	if captcha == "" {
		_, captchaErr := a.requiredAuthCaptcha(ctx, session, cookiePath, options.captchaImage, "动态码登录需要验证码，请提供 --captcha")
		return nil, captchaErr
	}
	if options.sendCode {
		result, requestErr := a.loginHTTP(ctx, siteRequest{
			Target: authServerTarget(&session, "/authserver/dynamicCode/getDynamicCode.htl"), SessionTarget: &session, CookieFile: cookiePath,
			Method: "POST", Data: []pair{{"mobile", mobile}, {"captcha", captcha}},
			Headers: []pair{{"Referer", responseURL}}, ReadOnly: true, AllowSSO: true, AllowBusinessFailure: true, Yes: true, RawJSON: true,
		})
		if requestErr != nil {
			return nil, requestErr
		}
		value, ok := businessData(result)
		payload, payloadOK := value.(map[string]any)
		if !ok || !payloadOK || fmt.Sprint(payload["code"]) != "success" {
			return nil, &siteError{Code: "authentication_failed", Message: "动态码发送失败", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "dynamic-code-response", "response": value}}
		}
		return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "dynamic-code-response-success", "service": safeSiteURL(serviceTarget), "operation": "send-code", "auth": "dynamic", "mobile": mobile}, nil
	}
	fields := casLoginFields(form)
	fields = setFormField(fields, "username", mobile)
	fields = setFormField(fields, "captcha", captcha)
	fields = setFormField(fields, "dynamicCode", strings.TrimSpace(options.dynamicCode))
	fields = setFormField(fields, "_eventId", "submit")
	fields = setFormField(fields, "cllt", "dynamicLogin")
	fields = setFormField(fields, "dllt", "generalLogin")
	actionURL, actionErr := ssoFormAction(form, responseURL, callback.String())
	if actionErr != nil {
		return nil, actionErr
	}
	loginResult, requestErr := a.loginHTTP(ctx, siteRequest{Target: actionURL, SessionTarget: &session, CookieFile: cookiePath, Method: "POST", Data: fields, Headers: []pair{{"Referer", responseURL}}, ReadOnly: true, AllowBusinessFailure: true, AllowSSO: true, Yes: true})
	if requestErr != nil {
		return nil, requestErr
	}
	return a.finishSSOLogin(ctx, serviceTarget, &session, cookiePath, loginResult, "dynamic", map[string]any{"mobile": mobile})
}

func (a NativeSite) loginSSOQR(ctx context.Context, callback, serviceTarget *url.URL, cookiePath string, options loginOptions) (map[string]any, *siteError) {
	session := *serviceTarget
	session.Path, session.RawQuery, session.Fragment = "/", "", ""
	form, responseURL, pageErr := a.loginSSOPage(ctx, callback, &session, cookiePath, "qrLoginForm", "qrcode")
	if pageErr != nil {
		return nil, pageErr
	}
	result, requestErr := a.loginHTTP(ctx, siteRequest{Target: authServerTarget(&session, "/authserver/qrCode/getToken"), SessionTarget: &session, CookieFile: cookiePath, Method: "GET", Params: []pair{{"ts", strconv.FormatInt(time.Now().UnixMilli(), 10)}}, ReadOnly: true, AllowSSO: true, Yes: true})
	if requestErr != nil {
		return nil, requestErr
	}
	body, _, bodyErr := loginBody(result)
	if bodyErr != nil {
		return nil, bodyErr
	}
	uuid := strings.TrimSpace(body)
	if uuid == "" {
		return nil, &siteError{Code: "authentication_failed", Message: "统一认证未返回扫码令牌"}
	}
	qrPath := options.qrImage
	if qrPath == "" {
		qrPath = filepath.Join(filepath.Dir(cookiePath), cookieHost(serviceTarget)+"-qr.png")
	}
	qrPath = expandUserPath(qrPath)
	qrURL := authServerTarget(&session, "/authserver/qrCode/getCode")
	qrResult, qrErr := a.loginHTTP(ctx, siteRequest{Target: qrURL, SessionTarget: &session, CookieFile: cookiePath, Method: "GET", Params: []pair{{"uuid", uuid}}, Output: qrPath, ReadOnly: true, AllowSSO: true, Yes: true})
	if qrErr != nil {
		return nil, qrErr
	}
	if qrResult == nil {
		return nil, &siteError{Code: "authentication_failed", Message: "二维码保存失败"}
	}
	fmt.Fprintf(os.Stderr, "二维码已保存：%s，请扫码确认\n", qrPath)
	deadline := time.NewTimer(120 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		statusResult, statusErr := a.loginHTTP(ctx, siteRequest{Target: authServerTarget(&session, "/authserver/qrCode/getStatus.htl"), SessionTarget: &session, CookieFile: cookiePath, Method: "GET", Params: []pair{{"ts", strconv.FormatInt(time.Now().UnixMilli(), 10)}, {"uuid", uuid}}, ReadOnly: true, AllowSSO: true, Yes: true})
		if statusErr != nil {
			return nil, statusErr
		}
		statusBody, _, statusBodyErr := loginBody(statusResult)
		if statusBodyErr != nil {
			return nil, statusBodyErr
		}
		switch strings.TrimSpace(statusBody) {
		case "1":
			fields := casLoginFields(form)
			fields = setFormField(fields, "uuid", uuid)
			actionURL, actionErr := ssoFormAction(form, responseURL, callback.String())
			if actionErr != nil {
				return nil, actionErr
			}
			loginResult, loginErr := a.loginHTTP(ctx, siteRequest{Target: actionURL, SessionTarget: &session, CookieFile: cookiePath, Method: "POST", Data: fields, Headers: []pair{{"Referer", responseURL}}, ReadOnly: true, AllowBusinessFailure: true, AllowSSO: true, Yes: true})
			if loginErr != nil {
				return nil, loginErr
			}
			return a.finishSSOLogin(ctx, serviceTarget, &session, cookiePath, loginResult, "qr", map[string]any{"qr_image": qrPath})
		case "3":
			return nil, &siteError{Code: "authentication_failed", Message: "二维码已失效", Details: map[string]any{"qr_image": qrPath, "evidence": "qr-status-3"}}
		}
		select {
		case <-ctx.Done():
			return nil, &siteError{Code: "authentication_timeout", Message: "扫码登录已取消", Details: map[string]any{"qr_image": qrPath}}
		case <-deadline.C:
			return nil, &siteError{Code: "authentication_timeout", Message: "扫码登录超时", Details: map[string]any{"qr_image": qrPath}}
		case <-ticker.C:
		}
	}
}

func (a NativeSite) finishSSOLogin(ctx context.Context, serviceTarget, session *url.URL, cookiePath string, loginResult map[string]any, auth string, extra map[string]any) (map[string]any, *siteError) {
	body, _, bodyErr := loginBody(loginResult)
	if bodyErr != nil {
		return nil, bodyErr
	}
	if failure := loginFailure(body); failure != nil {
		return nil, failure
	}
	if loginPageBody(body) {
		return nil, &siteError{Code: "authentication_failed", Message: "统一认证登录失败，未建立有效会话"}
	}
	if _, probeErr := a.loginHTTP(ctx, siteRequest{Target: serviceTarget, SessionTarget: session, CookieFile: cookiePath, Method: "GET", RequireLogin: true, ReadOnly: true, AllowSSO: true, Yes: true}); probeErr != nil {
		details := loginResponseDetails(loginResult)
		details["cause"] = probeErr.Code
		return nil, &siteError{Code: "authentication_failed", Message: "统一认证回跳成功，但服务未建立有效会话", Details: details}
	}
	result := map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed", "auth": auth, "cookie_file": cookiePath, "service": safeSiteURL(serviceTarget), "attempts": 1}
	for key, value := range extra {
		result[key] = value
	}
	return result, nil
}

func (a NativeSite) loginSSOWith(ctx context.Context, serviceTarget, probeTarget *url.URL, account, password string, options loginOptions, cookiePath string, useHandoff bool) (map[string]any, *siteError) {
	session := *serviceTarget
	session.Path, session.RawQuery, session.Fragment = "/", "", ""
	serviceURL := serviceTarget.String()
	if useHandoff {
		handoff := session
		handoff.Path = academicSSOPath
		if _, err := a.loginHTTP(ctx, siteRequest{Target: &handoff, SessionTarget: &session, CookieFile: cookiePath, Method: "GET", ReadOnly: true, AllowSSO: true, Yes: true}); err != nil && err.Code != "network_error" {
			return nil, err
		}
		service := session
		service.Path = academicSSOPath
		serviceURL = service.String()
	}
	authURL, _ := url.Parse(authServerBase + authLoginPath)
	query := authURL.Query()
	query.Set("service", serviceURL)
	authURL.RawQuery = query.Encode()
	loginResult, err := a.loginHTTP(ctx, siteRequest{Target: authURL, SessionTarget: &session, CookieFile: cookiePath, Method: "GET", ReadOnly: true, AllowSSO: true, Yes: true})
	if err != nil {
		return nil, err
	}
	body, responseURL, err := loginBody(loginResult)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "authentication_failed", Message: "统一认证登录页解析失败: " + parseErr.Error()}
	}
	form := document.first("form", "pwdFromId")
	if form == nil {
		for _, candidate := range document.findAll("form") {
			if candidate.first("input", "execution") != nil {
				form = candidate
				break
			}
		}
	}
	if form == nil {
		return nil, &siteError{Code: "authentication_failed", Message: "统一认证登录页缺少账号密码表单"}
	}
	execution := form.first("input", "execution")
	if execution == nil || execution.attr("value") == "" {
		return nil, &siteError{Code: "authentication_failed", Message: "统一认证登录页缺少 execution 参数"}
	}
	salt := ""
	if field := form.first("input", "pwdEncryptSalt"); field != nil {
		salt = field.attr("value")
	}
	fields := casLoginFields(form)
	encrypted, encryptErr := encryptCASPassword(password, salt)
	if encryptErr != nil {
		return nil, encryptErr
	}
	fields = append(fields, pair{"username", account}, pair{"password", encrypted}, pair{"_eventId", "submit"}, pair{"cllt", "userNameLogin"}, pair{"dllt", "generalLogin"})
	needCaptcha, captchaErr := a.authNeedsCaptcha(ctx, &session, account, cookiePath)
	if captchaErr != nil {
		return nil, captchaErr
	}
	if needCaptcha {
		if options.captcha == "" {
			path, fetchErr := a.fetchLoginCaptcha(ctx, session, cookiePath, true, options.captchaImage)
			details := map[string]any{}
			if fetchErr == nil {
				details["captcha_image"] = path
			}
			return nil, &siteError{Code: "captcha_required", Message: "统一认证需要验证码，请提供 --captcha", Details: details}
		}
		fields = append(fields, pair{"captcha", options.captcha})
	} else {
		fields = append(fields, pair{"captcha", options.captcha})
	}
	actionValue := form.attr("action")
	if actionValue == "" {
		actionValue = responseURL
	}
	action := resolvePageURL(responseURL, actionValue)
	actionURL, actionErr := validatePageActionTarget(action, responseURL, false)
	if actionErr != nil {
		return nil, actionErr
	}
	if !strings.EqualFold(actionURL.Host, "authserver.csust.edu.cn") {
		return nil, &siteError{Code: "invalid_path", Message: "统一认证表单地址不是认证服务器"}
	}
	actionQuery := actionURL.Query()
	if actionQuery.Get("service") == "" {
		actionQuery.Set("service", serviceURL)
		actionURL.RawQuery = actionQuery.Encode()
	}
	loginResult, err = a.loginHTTP(ctx, siteRequest{Target: actionURL, SessionTarget: &session, CookieFile: cookiePath, Method: "POST", Data: fields, Headers: []pair{{"Referer", responseURL}}, ReadOnly: true, AllowBusinessFailure: true, AllowSSO: true, Yes: true})
	if err != nil {
		return nil, err
	}
	body, _, bodyErr := loginBody(loginResult)
	if bodyErr != nil {
		return nil, bodyErr
	}
	if loginFailure(body) != nil {
		return nil, loginFailure(body)
	}
	if loginPageBody(body) {
		return nil, &siteError{Code: "authentication_failed", Message: "统一认证登录失败，未建立有效会话"}
	}
	probeRequest := siteRequest{Target: probeTarget, SessionTarget: &session, CookieFile: cookiePath, Method: "GET", RequireLogin: true, ReadOnly: true, AllowSSO: true, Yes: true}
	if _, probeErr := a.loginHTTP(ctx, probeRequest); probeErr != nil {
		details := loginResponseDetails(loginResult)
		details["cause"] = probeErr.Code
		return nil, &siteError{Code: "authentication_failed", Message: "统一认证回跳成功，但服务未建立有效会话", Details: details}
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed", "username": account, "auth": "sso", "cookie_file": cookiePath, "service": safeSiteURL(serviceTarget), "attempts": 1}, nil
}

func (a NativeSite) loginLocal(ctx context.Context, base *url.URL, account, password string, options loginOptions) (map[string]any, *siteError) {
	cookiePath := academicCookiePath()
	root := *base
	root.Path = "/"
	root.RawQuery = ""
	root.Fragment = ""
	if _, err := a.loginHTTP(ctx, siteRequest{Target: &root, SessionTarget: &root, CookieFile: cookiePath, Method: "GET", ReadOnly: true, Yes: true}); err != nil {
		return nil, err
	}
	seedTarget := root
	seedTarget.Path = "/Logon.do"
	seedQuery := seedTarget.Query()
	seedQuery.Set("method", "logon")
	seedQuery.Set("flag", "sess")
	seedTarget.RawQuery = seedQuery.Encode()
	seed, seedErr := a.loginHTTP(ctx, siteRequest{Target: &seedTarget, SessionTarget: &root, CookieFile: cookiePath, Method: "POST", Data: []pair{}, ReadOnly: true, Yes: true})
	if seedErr != nil {
		return nil, seedErr
	}
	seedBody, _, seedBodyErr := loginBody(seed)
	if seedBodyErr != nil {
		return nil, seedBodyErr
	}
	encoded, encodedErr := generateEncodedGo(account, password, strings.TrimSpace(seedBody))
	if encodedErr != nil {
		return nil, encodedErr
	}
	if options.captcha == "" {
		path, fetchErr := a.fetchLoginCaptcha(ctx, root, cookiePath, false, options.captchaImage)
		if fetchErr != nil {
			return nil, fetchErr
		}
		return nil, &siteError{Code: "captcha_required", Message: "教务登录需要验证码，请提供 --captcha", Details: map[string]any{"captcha_image": path}}
	}
	loginTarget := root
	loginTarget.Path = "/Logon.do"
	loginTarget.RawQuery = "method=logon"
	response, requestErr := a.loginHTTP(ctx, siteRequest{Target: &loginTarget, SessionTarget: &root, CookieFile: cookiePath, Method: "POST", Data: []pair{{"userAccount", ""}, {"userPassword", ""}, {"RANDOMCODE", options.captcha}, {"encoded", encoded}}, Headers: []pair{{"Referer", root.String()}}, ReadOnly: true, AllowBusinessFailure: true, Yes: true})
	if requestErr != nil {
		if requestErr.Code == "business_rejected" {
			if failure := loginFailure(requestErr.Message); failure != nil {
				return nil, failure
			}
		}
		return nil, requestErr
	}
	body, _, bodyErr := loginBody(response)
	if bodyErr != nil {
		return nil, bodyErr
	}
	if failure := loginFailure(body); failure != nil {
		return nil, failure
	}
	if loginPageBody(body) {
		return nil, &siteError{Code: "authentication_failed", Message: "教务登录失败，未建立有效会话"}
	}
	probe := root
	probe.Path = academicProbePath
	if _, probeErr := a.loginHTTP(ctx, siteRequest{Target: &probe, SessionTarget: &root, CookieFile: cookiePath, Method: "GET", RequireLogin: true, ReadOnly: true, Yes: true}); probeErr != nil {
		details := loginResponseDetails(response)
		details["cause"] = probeErr.Code
		return nil, &siteError{Code: "authentication_failed", Message: "登录失败，教务系统未建立有效会话", Details: details}
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed", "username": account, "auth": "local", "cookie_file": cookiePath, "attempts": 1}, nil
}

func casLoginFields(form *pageNode) []pair {
	fields := make([]pair, 0)
	for _, field := range form.findAll("input") {
		name := strings.TrimSpace(field.attr("name"))
		kind := strings.ToLower(firstNonEmpty(field.attr("type"), "text"))
		if name == "" || casLoginFieldIgnored(name) || field.disabled() || kind == "button" || kind == "file" || kind == "reset" || kind == "submit" || (kind == "checkbox" || kind == "radio") && !field.has("checked") {
			continue
		}
		fields = append(fields, pair{name, field.attr("value")})
	}
	return fields
}

func casLoginFieldIgnored(name string) bool {
	switch name {
	case "username", "password", "passwordText", "pwdEncryptSalt", "captcha", "_eventId", "cllt", "dllt":
		return true
	default:
		return false
	}
}

func (a NativeSite) loginHTTP(ctx context.Context, request siteRequest) (map[string]any, *siteError) {
	return a.execute(ctx, request)
}

func (a NativeSite) executeAcademicRequestWithRecovery(ctx context.Context, request siteRequest) (map[string]any, *siteError) {
	result, err := a.execute(ctx, request)
	if err == nil || err.Code != "login_required" {
		return result, err
	}
	if _, loginErr := a.loginAcademic(ctx, loginOptions{auth: "auto"}); loginErr != nil {
		return nil, loginErr
	}
	return a.execute(ctx, request)
}

func (a NativeSite) executeAcademicRunWithRecovery(ctx context.Context, run func() (map[string]any, *siteError)) (map[string]any, *siteError) {
	result, err := run()
	if err == nil || err.Code != "login_required" {
		return result, err
	}
	if _, loginErr := a.loginAcademic(ctx, loginOptions{auth: "auto"}); loginErr != nil {
		return nil, loginErr
	}
	return run()
}

func loginBody(result map[string]any) (string, string, *siteError) {
	response, ok := result["response"].(map[string]any)
	if !ok {
		return "", "", &siteError{Code: "parse_error", Message: "认证响应不是文本页面"}
	}
	body, _ := response["body_internal"].(string)
	if body == "" {
		body, _ = response["body"].(string)
	}
	pageURL, _ := response["raw_url"].(string)
	if pageURL == "" {
		pageURL, _ = response["url"].(string)
	}
	return body, pageURL, nil
}

func loginResponseDetails(result map[string]any) map[string]any {
	details := map[string]any{}
	response, _ := result["response"].(map[string]any)
	for _, key := range []string{"status", "url", "content_type", "format"} {
		if value, ok := response[key]; ok {
			details[key] = value
		}
	}
	return details
}

func (a NativeSite) authNeedsCaptcha(ctx context.Context, session *url.URL, account, cookiePath string) (bool, *siteError) {
	target := *session
	target.Scheme, target.Host, target.Path, target.RawQuery = "https", "authserver.csust.edu.cn", authCaptchaCheckPath, ""
	query := target.Query()
	query.Set("username", account)
	query.Set("_", strconv.FormatInt(time.Now().UnixMilli(), 10))
	target.RawQuery = query.Encode()
	result, err := a.loginHTTP(ctx, siteRequest{Target: &target, SessionTarget: session, CookieFile: cookiePath, Method: "GET", ReadOnly: true, AllowSSO: true, Yes: true})
	if err != nil {
		return false, err
	}
	body, _, bodyErr := loginBody(result)
	if bodyErr != nil {
		return false, nil
	}
	var value any
	if json.Unmarshal([]byte(body), &value) != nil {
		return false, nil
	}
	return jsonBoolField(value, "isNeed"), nil
}

func (a NativeSite) fetchLoginCaptcha(ctx context.Context, session url.URL, cookiePath string, auth bool, output string) (string, *siteError) {
	target := session
	if auth {
		target.Scheme, target.Host = "https", "authserver.csust.edu.cn"
		target.Path = authCaptchaPath
		target.RawQuery = ""
	} else {
		target.Path = "/verifycode.servlet"
		if strings.HasPrefix(session.Path, "/jsxsd/") {
			target.Path = "/jsxsd/verifycode.servlet"
		}
		target.RawQuery = ""
	}
	if output == "" {
		output = filepath.Join(filepath.Dir(cookiePath), map[bool]string{true: "authserver-captcha.png", false: "captcha.png"}[auth])
	}
	request := siteRequest{Target: &target, SessionTarget: &session, CookieFile: cookiePath, Method: "GET", Output: expandUserPath(output), ReadOnly: true, AllowSSO: auth, Yes: true}
	if _, err := a.loginHTTP(ctx, request); err != nil {
		return "", err
	}
	return request.Output, nil
}

func loginPageBody(source string) bool {
	document, err := parsePage(source)
	if err != nil {
		return false
	}
	if document.first("form", "loginForm") != nil || document.first("form", "pwdFromId") != nil || document.first("form", "Form1") != nil {
		return true
	}
	text := pageDisplayText(document)
	return strings.Contains(text, "请输入账号") && strings.Contains(text, "用户登录")
}
func loginFailure(source string) *siteError {
	lower := strings.ToLower(source)
	if strings.Contains(source, "验证码错误") || strings.Contains(source, "验证码不正确") || strings.Contains(source, "验证码无效") || strings.Contains(source, "随机码错误") {
		return &siteError{Code: "captcha_failed", Message: "验证码错误"}
	}
	if strings.Contains(source, "密码错误") || strings.Contains(source, "账号不存在") || strings.Contains(source, "用户不存在") || strings.Contains(lower, "用户名或密码") {
		return &siteError{Code: "authentication_failed", Message: "账号或密码错误"}
	}
	return nil
}

func credentialsGo(username, password string) (string, string, *siteError) {
	values := dotenvGo()
	account := strings.TrimSpace(username)
	if account == "" {
		account = strings.TrimSpace(firstNonEmpty(os.Getenv("CSUST_USERNAME"), os.Getenv("username"), values["CSUST_USERNAME"], values["username"]))
	}
	if password == "" {
		password = firstNonEmpty(os.Getenv("CSUST_PASSWORD"), os.Getenv("password"), values["CSUST_PASSWORD"], values["password"])
	}
	missing := []string{}
	if account == "" {
		missing = append(missing, "CSUST_USERNAME")
	}
	if password == "" {
		missing = append(missing, "CSUST_PASSWORD")
	}
	if len(missing) > 0 {
		return "", "", &siteError{Code: "credentials_required", Message: "缺少登录凭据：" + strings.Join(missing, ", ") + "（也可在 .env 中设置 username/password）"}
	}
	return account, password, nil
}
func envValueGo(name string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}
	return dotenvGo()[name]
}
func dotenvGo() map[string]string {
	result := map[string]string{}
	path := os.Getenv("CSUST_ENV_FILE")
	if path == "" {
		path = ".env"
	}
	path = expandUserPath(path)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return result
	}
	if info.Mode().Perm()&0o077 != 0 && os.Getenv("CSUST_ALLOW_INSECURE_ENV") != "1" {
		return result
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		name, value, found := strings.Cut(line, "=")
		if !found || !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(strings.TrimSpace(name)) {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		result[strings.TrimSpace(name)] = value
	}
	return result
}
func removeCookieIfPresent(path string) error {
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
}

func generateEncodedGo(account, password, data string) (string, *siteError) {
	parts := strings.SplitN(data, "#", 2)
	if len(parts) != 2 {
		return "", &siteError{Code: "login_protocol_error", Message: "教务系统返回的登录参数格式异常"}
	}
	code := []rune(account + "%%%" + password)
	result := strings.Builder{}
	cursor := 0
	for index, character := range code {
		if index >= 20 {
			result.WriteString(string(code[index:]))
			break
		}
		if index >= len(parts[1]) {
			return "", &siteError{Code: "login_protocol_error", Message: "教务系统返回的登录参数长度异常"}
		}
		count := int(parts[1][index] - '0')
		if count < 0 || count > len(parts[0])-cursor {
			return "", &siteError{Code: "login_protocol_error", Message: "教务系统返回的登录参数校验失败"}
		}
		result.WriteRune(character)
		result.WriteString(parts[0][cursor : cursor+count])
		cursor += count
	}
	return result.String(), nil
}

func encryptCASPassword(password, salt string) (string, *siteError) {
	salt = strings.TrimSpace(salt)
	if salt == "" {
		return password, nil
	}
	key := []byte(salt)
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return "", &siteError{Code: "login_protocol_error", Message: "统一认证登录参数中的加密盐长度无效"}
	}
	prefix, err := randomCASString(64)
	if err != nil {
		return "", &siteError{Code: "login_protocol_error", Message: "无法生成统一认证加密随机数: " + err.Error()}
	}
	iv, err := randomCASString(16)
	if err != nil {
		return "", &siteError{Code: "login_protocol_error", Message: "无法生成统一认证加密随机数: " + err.Error()}
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", &siteError{Code: "login_protocol_error", Message: "统一认证加密参数无效: " + err.Error()}
	}
	plaintext := []byte(prefix + password)
	padding := aes.BlockSize - len(plaintext)%aes.BlockSize
	plaintext = append(plaintext, strings.Repeat(string(rune(padding)), padding)...)
	ciphertext := make([]byte, len(plaintext))
	cipher.NewCBCEncrypter(block, []byte(iv)).CryptBlocks(ciphertext, plaintext)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}
func randomCASString(length int) (string, error) {
	result := make([]byte, length)
	for index := range result {
		number, err := cryptorand.Int(cryptorand.Reader, bigInt(len(casAESChars)))
		if err != nil {
			return "", err
		}
		result[index] = casAESChars[number.Int64()]
	}
	return string(result), nil
}

func jsonBoolField(value any, key string) bool {
	if object, ok := value.(map[string]any); ok {
		if direct, exists := object[key]; exists {
			return boolValue(direct)
		}
		if nested, exists := object["data"]; exists {
			return jsonBoolField(nested, key)
		}
	}
	return false
}
func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func bigInt(value int) *big.Int { return big.NewInt(int64(value)) }
