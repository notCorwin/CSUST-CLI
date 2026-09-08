package adapter

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type vpnResource struct {
	name, webPath, nativePath, kind string
}

var vpnResources = []vpnResource{
	{"files", "/enclient/files/{path}", "/api/v1/files/{path}", "image/download"},
	{"pics", "/enclient/files/{path}", "/api/v1/pics/{path}", "image"},
	{"service-agreement", "/enclient/serviceAgreement.html", "", "login agreement"},
	{"user-terms", "/enclient/userTerms.html", "", "login terms"},
}

func (a NativeSite) runVPNCommand(ctx context.Context, args []string, jsonMode bool) (bool, []byte, []byte, int, error) {
	if len(args) == 0 || args[0] != "vpn" {
		return false, nil, nil, 0, nil
	}
	if len(args) < 2 || containsHelp(args[1:]) {
		return false, nil, nil, 0, nil
	}
	if args[1] == "login" {
		result, runErr := a.runVPNLogin(ctx, args[2:])
		if runErr != nil {
			if jsonMode {
				return true, errorJSON(runErr), nil, 2, nil
			}
			return true, nil, []byte("错误: " + runErr.Error() + "\n"), 2, nil
		}
		if jsonMode {
			encoded, err := json.Marshal(result)
			if err != nil {
				return true, nil, nil, 2, err
			}
			return true, encoded, nil, 0, nil
		}
		return true, []byte(fmt.Sprintf("VPN 登录成功：%v\n会话已保存：%v\n", result["username"], result["session_file"])), nil, 0, nil
	}
	result, runErr := a.executeVPNCommand(ctx, args[1:])
	if runErr != nil {
		if jsonMode {
			return true, errorJSON(runErr), nil, 2, nil
		}
		return true, nil, []byte("错误: " + runErr.Error() + "\n"), 2, nil
	}
	if jsonMode {
		encoded, err := json.Marshal(result)
		if err != nil {
			return true, nil, nil, 2, err
		}
		return true, encoded, nil, 0, nil
	}
	return true, []byte(renderVPNResult(result)), nil, 0, nil
}

func (a NativeSite) executeVPNCommand(ctx context.Context, args []string) (map[string]any, *siteError) {
	command := args[0]
	switch command {
	case "routes":
		if err := onlyJSONArgs(args[1:]); err != nil {
			return nil, err
		}
		return vpnRoutesResult(), nil
	case "controls":
		if err := onlyJSONArgs(args[1:]); err != nil {
			return nil, err
		}
		return vpnControlsResult(), nil
	case "catalog":
		return parseVPNCatalog(args[1:])
	case "page":
		return parseVPNPage(args[1:])
	case "api":
		return a.runVPNAPI(ctx, args[1:])
	case "resource":
		return a.runVPNResource(ctx, args[1:])
	case "sso-url":
		return a.runVPNSSOURL(ctx, args[1:])
	case "status":
		return a.runVPNStatus(ctx, args[1:])
	case "logout":
		return a.runVPNLogout(ctx, args[1:])
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "未知 vpn 子命令: " + command}
	}
}

type vpnLoginOptions struct {
	username, password, auth, captchaInfo string
	passwordStdin                         bool
}

func parseVPNLoginOptions(args []string) (vpnLoginOptions, *siteError) {
	options := vpnLoginOptions{auth: "auto"}
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		if arg == "--json" {
			continue
		}
		if arg == "--password-stdin" {
			if inline {
				return vpnLoginOptions{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			options.passwordStdin = true
			continue
		}
		if !inline {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return vpnLoginOptions{}, &siteError{Code: "invalid_argument", Message: arg + " 缺少参数值"}
			}
			index++
			value = args[index]
		}
		switch arg {
		case "--username":
			options.username = value
		case "--auth":
			options.auth = value
		case "--captcha-info":
			options.captchaInfo = value
		default:
			return vpnLoginOptions{}, &siteError{Code: "invalid_argument", Message: "vpn login 参数无效: " + arg}
		}
	}
	if options.auth != "auto" && options.auth != "cas" && options.auth != "local" {
		return vpnLoginOptions{}, &siteError{Code: "invalid_argument", Message: "--auth 必须是 auto、cas 或 local"}
	}
	if options.passwordStdin {
		password, err := io.ReadAll(os.Stdin)
		if err != nil {
			return vpnLoginOptions{}, &siteError{Code: "credentials_required", Message: "无法读取标准输入密码: " + err.Error()}
		}
		options.password = strings.TrimRight(string(password), "\r\n")
	}
	return options, nil
}

func (a NativeSite) runVPNLogin(ctx context.Context, args []string) (map[string]any, *siteError) {
	options, parseErr := parseVPNLoginOptions(args)
	if parseErr != nil {
		return nil, parseErr
	}
	account, password, credentialErr := credentialsGo(options.username, options.password)
	if credentialErr != nil {
		return nil, credentialErr
	}
	base, cookie, _, connErr := vpnConnection(false)
	if connErr != nil {
		return nil, connErr
	}
	session := map[string]any{}
	configResult, configErr := a.vpnJSONRequest(ctx, base, cookie, session, "/api/users/custom/page/login/cfg/select", "POST", map[string]any{}, true)
	if configErr != nil {
		return nil, configErr
	}
	config := resultJSON(configResult)
	if code := vpnResponseCode(configResult); code != "" && code != "200" {
		return nil, &siteError{Code: "authentication_failed", Message: "VPN 登录配置获取失败", Details: map[string]any{"code": code}}
	}
	if options.auth == "cas" || options.auth == "auto" && vpnConfigUsesCAS(config) {
		return a.vpnCASLogin(ctx, base, cookie, session, account, password, config, options.captchaInfo)
	}
	return a.vpnLocalLogin(ctx, base, cookie, session, account, password, options.captchaInfo)
}

func vpnConfigUsesCAS(config map[string]any) bool {
	data, _ := config["data"].(map[string]any)
	if strings.EqualFold(fmt.Sprint(data["defaultAuthType"]), "CAS") || strings.EqualFold(fmt.Sprint(data["defaultAuthType"]), "SSO_CAS") {
		return true
	}
	items, _ := data["ssoLoginTypes"].([]any)
	for _, item := range items {
		entry, _ := item.(map[string]any)
		if strings.EqualFold(fmt.Sprint(entry["loginType"]), "SSO_CAS") && fmt.Sprint(entry["hidden"]) != "true" {
			return true
		}
	}
	return false
}

func (a NativeSite) vpnLocalLogin(ctx context.Context, base *url.URL, cookie string, session map[string]any, account, password, captchaInfo string) (map[string]any, *siteError) {
	keyResult, keyErr := a.vpnJSONRequest(ctx, base, cookie, session, "/api/users/client/auth/generateKey", "GET", nil, true)
	if keyErr != nil {
		return nil, keyErr
	}
	keyPayload := resultJSON(keyResult)
	if vpnResponseCode(keyResult) != "200" {
		return nil, &siteError{Code: "authentication_failed", Message: "VPN 登录密钥获取失败"}
	}
	data, _ := keyPayload["data"].(map[string]any)
	seed, _ := data["enToken"].(string)
	parts := strings.Split(seed, "-")
	key := seed
	if len(parts) >= 3 {
		key = parts[2]
	} else if len(parts) > 0 {
		key = parts[len(parts)-1]
	}
	encrypted, encryptErr := encryptVPNPassword(password, key)
	if encryptErr != nil {
		return nil, encryptErr
	}
	payload := map[string]any{"account": account, "passwd": encrypted}
	if captchaInfo != "" {
		value, parseErr := readJSONArgument(captchaInfo)
		if parseErr != nil {
			return nil, parseErr
		}
		object, ok := value.(map[string]any)
		if !ok {
			return nil, &siteError{Code: "invalid_argument", Message: "--captcha-info 必须是 JSON 对象"}
		}
		payload["captchaInfo"] = object
	}
	loginResult, loginErr := a.vpnJSONRequest(ctx, base, cookie, session, "/api/users/auth/login", "POST", payload, true)
	if loginErr != nil {
		return nil, loginErr
	}
	if vpnResponseCode(loginResult) != "200" {
		pending := map[string]bool{"2050": true, "2051": true, "2060": true, "2080": true, "4010": true, "4020": true, "4030": true, "4040": true}[vpnResponseCode(loginResult)]
		if pending {
			session["pendingToken"] = seed
			session["account"], session["auth"] = account, "local"
			if saveErr := saveVPNSession(sessionPath(false), session); saveErr != nil {
				return nil, saveErr
			}
			return map[string]any{"ok": false, "submitted": true, "confirmed": false, "evidence": "pending", "username": account, "pending": true, "next": "second-auth", "code": vpnResponseCode(loginResult), "session_file": sessionPath(false)}, nil
		}
		return nil, &siteError{Code: "authentication_failed", Message: "VPN 登录失败", Details: map[string]any{"code": vpnResponseCode(loginResult)}}
	}
	loginData, _ := resultJSON(loginResult)["data"].(map[string]any)
	for _, name := range []string{"token", "refreshToken", "account", "username", "name", "userId", "id"} {
		if value, exists := loginData[name]; exists {
			session[name] = value
		}
	}
	session["account"], session["auth"] = account, "local"
	if saveErr := saveVPNSession(sessionPath(false), session); saveErr != nil {
		return nil, saveErr
	}
	infoResult, infoErr := a.vpnJSONRequest(ctx, base, cookie, session, "/api/users/info", "GET", nil, true)
	if infoErr != nil {
		return nil, &siteError{Code: "authentication_failed", Message: "VPN 登录后未建立有效会话", Details: map[string]any{"cause": infoErr.Code}}
	}
	if vpnResponseCode(infoResult) != "200" {
		return nil, &siteError{Code: "authentication_failed", Message: "VPN 登录后未建立有效会话"}
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed", "username": account, "auth": "local", "session_file": sessionPath(false), "user": resultJSON(infoResult)["data"]}, nil
}

func encryptVPNPassword(password, key string) (string, *siteError) {
	keyBytes := []byte(key)
	if len(keyBytes) != 16 {
		return "", &siteError{Code: "vpn_protocol_error", Message: "VPN 登录密钥不是 16 字节，无法复现门户 AES 登录协议"}
	}
	padding := aes.BlockSize - len([]byte(password))%aes.BlockSize
	padded := append([]byte(password), strings.Repeat(string(rune(padding)), padding)...)
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", &siteError{Code: "vpn_protocol_error", Message: "无法创建 VPN AES 密码加密器"}
	}
	output := make([]byte, len(padded))
	iv := append([]byte(nil), keyBytes...)
	for index := range iv {
		iv[index] = keyBytes[len(keyBytes)-index-1]
	}
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(output, padded)
	return base64.StdEncoding.EncodeToString(output), nil
}

func saveVPNSession(path string, session map[string]any) *siteError {
	content, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return &siteError{Code: "session_write_failed", Message: "无法序列化 VPN 会话"}
	}
	if err := writeCookieFile(path, string(content)+"\n"); err != nil {
		return &siteError{Code: "session_write_failed", Message: "无法保存 VPN 会话: " + err.Error()}
	}
	return nil
}

func (a NativeSite) vpnJSONRequest(ctx context.Context, base *url.URL, cookie string, session map[string]any, path, method string, body any, readOnly bool) (map[string]any, *siteError) {
	target, pathErr := vpnAPIPath(base, path, false)
	if pathErr != nil {
		return nil, pathErr
	}
	request := siteRequest{Target: target, CookieFile: cookie, Method: method, Headers: vpnHeaders(base, target, session, false), RequireLogin: false, ReadOnly: readOnly, Yes: true, RawJSON: true}
	if method != "GET" && method != "HEAD" && method != "OPTIONS" {
		request.JSON, request.HasJSON = body, true
	}
	result, runErr := a.execute(ctx, request)
	if runErr != nil {
		return nil, runErr
	}
	return result, nil
}

func (a NativeSite) vpnCASLogin(ctx context.Context, base *url.URL, cookie string, session map[string]any, account, password string, config map[string]any, captchaInfo string) (map[string]any, *siteError) {
	casPath := "/enclient/api/users/admin/custom/page/login/sso/cas"
	if data, ok := config["data"].(map[string]any); ok {
		if configured, ok := data["casLoginUrl"].(string); ok && configured != "" {
			casPath = configured
		}
	}
	casTarget, pathErr := vpnAPIPath(base, casPath, false)
	if pathErr != nil {
		return nil, pathErr
	}
	sessionTarget := *base
	sessionTarget.Path = "/"
	first, firstErr := a.execute(ctx, siteRequest{Target: casTarget, SessionTarget: &sessionTarget, CookieFile: cookie, Method: "GET", Headers: vpnHeaders(base, casTarget, session, false), ReadOnly: true, AllowSSO: true, Yes: true})
	if firstErr != nil {
		return nil, firstErr
	}
	body, responseURL, bodyErr := loginBody(first)
	if bodyErr != nil {
		return nil, bodyErr
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "authentication_failed", Message: "VPN 统一认证登录页解析失败: " + parseErr.Error()}
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
		return nil, &siteError{Code: "authentication_failed", Message: "VPN 统一认证登录页缺少账号密码表单"}
	}
	fields := make([]pair, 0)
	reserved := map[string]bool{"username": true, "password": true, "passwordText": true, "pwdEncryptSalt": true, "captcha": true, "_eventId": true, "cllt": true, "dllt": true, "lt": true}
	for _, field := range form.findAll("input") {
		name := strings.TrimSpace(field.attr("name"))
		kind := strings.ToLower(firstNonEmpty(field.attr("type"), "text"))
		if name == "" || reserved[name] || field.disabled() || kind == "button" || kind == "file" || kind == "reset" || kind == "submit" || (kind == "checkbox" || kind == "radio") && !field.has("checked") {
			continue
		}
		fields = append(fields, pair{name, field.attr("value")})
	}
	salt := ""
	if field := form.first("input", "pwdEncryptSalt"); field != nil {
		salt = field.attr("value")
	}
	encrypted, encryptErr := encryptCASPassword(password, salt)
	if encryptErr != nil {
		return nil, encryptErr
	}
	fields = append(fields, pair{"username", account}, pair{"password", encrypted}, pair{"_eventId", "submit"}, pair{"cllt", "userNameLogin"}, pair{"dllt", "generalLogin"}, pair{"lt", ""})
	if captchaInfo != "" {
		value, jsonErr := readJSONArgument(captchaInfo)
		if jsonErr != nil {
			return nil, jsonErr
		}
		object, ok := value.(map[string]any)
		if !ok {
			return nil, &siteError{Code: "invalid_argument", Message: "--captcha-info 必须是 JSON 对象"}
		}
		captcha := fmt.Sprint(object["captcha"])
		if captcha == "<nil>" || captcha == "" {
			captcha = fmt.Sprint(object["code"])
		}
		fields = append(fields, pair{"captcha", captcha})
	} else {
		fields = append(fields, pair{"captcha", ""})
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
	if !strings.EqualFold(actionURL.Hostname(), "authserver.csust.edu.cn") {
		return nil, &siteError{Code: "invalid_path", Message: "VPN 统一认证表单地址不是认证服务器"}
	}
	loginResult, loginErr := a.execute(ctx, siteRequest{Target: actionURL, SessionTarget: &sessionTarget, CookieFile: cookie, Method: "POST", Data: fields, Headers: []pair{{"Referer", responseURL}}, ReadOnly: true, AllowSSO: true, Yes: true})
	if loginErr != nil {
		return nil, loginErr
	}
	body, _, bodyErr = loginBody(loginResult)
	if bodyErr != nil {
		return nil, bodyErr
	}
	if failure := loginFailure(body); failure != nil {
		return nil, failure
	}
	if loginPageBody(body) {
		return nil, &siteError{Code: "authentication_failed", Message: "VPN 统一认证登录失败，未建立有效会话"}
	}
	infoResult, infoErr := a.vpnJSONRequest(ctx, base, cookie, session, "/api/users/info", "GET", nil, true)
	if infoErr != nil || vpnResponseCode(infoResult) != "200" {
		cause := "authentication_failed"
		if infoErr != nil {
			cause = infoErr.Code
		}
		return nil, &siteError{Code: "authentication_failed", Message: "统一认证回跳成功，但 VPN 未建立有效会话", Details: map[string]any{"cause": cause}}
	}
	session["account"], session["auth"] = account, "cas"
	if saveErr := saveVPNSession(sessionPath(false), session); saveErr != nil {
		return nil, saveErr
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed", "username": account, "auth": "cas", "session_file": sessionPath(false), "user": resultJSON(infoResult)["data"]}, nil
}

func onlyJSONArgs(args []string) *siteError {
	for _, arg := range args {
		if arg != "--json" {
			return &siteError{Code: "invalid_argument", Message: "该 vpn 清单命令不接受参数: " + arg}
		}
	}
	return nil
}

func vpnRoutesResult() map[string]any {
	routes := make([]map[string]any, 0, len(vpnRoutes))
	for _, item := range vpnRoutes {
		aliases := []string{}
		for _, prefix := range []string{"/login/", "/home/"} {
			if strings.HasPrefix(item.path, prefix) {
				aliases = append(aliases, strings.TrimPrefix(item.path, prefix))
			}
		}
		routes = append(routes, map[string]any{"name": item.name, "section": item.section, "path": item.path, "aliases": aliases, "description": item.description, "apis": item.apis})
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed", "source": "https://vpn.csust.edu.cn/enclient/start.html", "routes": routes, "route_count": len(routes), "api_count": len(vpnAPIRows)}
}

func vpnControlsResult() map[string]any {
	controls := make([]map[string]any, 0, len(vpnControls))
	for _, item := range vpnControls {
		controls = append(controls, map[string]any{"name": item.name, "section": item.section, "label": item.label, "routes": item.routes, "apis": item.apis})
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed", "controls": controls, "control_count": len(controls), "api_count": len(vpnAPIRows)}
}

func parseVPNCatalog(args []string) (map[string]any, *siteError) {
	group := ""
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		if arg == "--json" {
			continue
		}
		if arg != "--group" {
			return nil, &siteError{Code: "invalid_argument", Message: "vpn catalog 参数无效: " + arg}
		}
		if !inline {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return nil, &siteError{Code: "invalid_argument", Message: "--group 缺少参数值"}
			}
			index++
			value = args[index]
		}
		group = value
	}
	items := vpnAPICatalog()
	if group != "" {
		filtered := items[:0]
		for _, item := range items {
			if item["group"] == group {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	resources := make([]map[string]any, 0, len(vpnResources))
	for _, item := range vpnResources {
		resources = append(resources, map[string]any{"name": item.name, "web_path": item.webPath, "native_path": item.nativePath, "kind": item.kind})
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed", "source": "https://vpn.csust.edu.cn/enclient/start.html", "catalog": items, "api_count": len(items), "resources": resources}, nil
}

func parseVPNPage(args []string) (map[string]any, *siteError) {
	routeName := ""
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		if arg == "--json" {
			continue
		}
		if arg != "--route" {
			return nil, &siteError{Code: "invalid_argument", Message: "vpn page 参数无效: " + arg}
		}
		if !inline {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return nil, &siteError{Code: "invalid_argument", Message: "--route 缺少参数值"}
			}
			index++
			value = args[index]
		}
		routeName = value
	}
	if routeName == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "vpn page 必须提供 --route"}
	}
	route, found, ambiguous := vpnFindRoute(routeName)
	if ambiguous {
		return nil, &siteError{Code: "ambiguous_route", Message: "VPN 路由别名有歧义，请使用完整路径或路由名"}
	}
	if !found {
		return nil, &siteError{Code: "unknown_route", Message: "未知 VPN 路由: " + routeName}
	}
	controls := make([]map[string]any, 0)
	for _, control := range vpnControls {
		for _, name := range control.routes {
			if name == route.name {
				controls = append(controls, map[string]any{"name": control.name, "section": control.section, "label": control.label, "routes": control.routes, "apis": control.apis})
				break
			}
		}
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed", "route": map[string]any{"name": route.name, "section": route.section, "path": route.path, "description": route.description, "apis": route.apis}, "controls": controls}, nil
}

func (a NativeSite) runVPNAPI(ctx context.Context, args []string) (map[string]any, *siteError) {
	name, path, method, native, yes, output, dataJSON := "", "", "GET", false, false, "", ""
	params, form, files, pathArgs := []pair{}, []pair{}, []filePart{}, []string{}
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		if arg == "--json" {
			continue
		}
		if arg == "--yes" || arg == "--native" {
			if inline {
				return nil, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			if arg == "--yes" {
				yes = true
			} else {
				native = true
			}
			continue
		}
		if !inline {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return nil, &siteError{Code: "invalid_argument", Message: arg + " 缺少参数值"}
			}
			index++
			value = args[index]
		}
		switch arg {
		case "--name":
			name = value
		case "--path":
			path = value
		case "--method":
			method = strings.ToUpper(value)
		case "--data-json", "--data":
			dataJSON = value
		case "--form":
			item, err := splitPair(value, "--form")
			if err != nil {
				return nil, err
			}
			form = append(form, item)
		case "--file":
			item, err := readFilePart(value)
			if err != nil {
				return nil, err
			}
			files = append(files, item)
		case "--param":
			item, err := splitPair(value, "--param")
			if err != nil {
				return nil, err
			}
			params = append(params, item)
		case "--path-arg":
			pathArgs = append(pathArgs, value)
		case "--output":
			output = expandUserPath(value)
		default:
			return nil, &siteError{Code: "invalid_argument", Message: "vpn api 参数无效: " + arg}
		}
	}
	if (name == "") == (path == "") {
		return nil, &siteError{Code: "invalid_argument", Message: "--name 与 --path 必须且只能指定一个"}
	}
	spec := vpnAPISpec{method: method, path: path}
	if name != "" {
		var found bool
		spec, found = vpnSpec(name)
		if !found {
			return nil, &siteError{Code: "unknown_api", Message: "未知 VPN API: " + name}
		}
		method = spec.method
	} else if method == "" {
		method = "GET"
	}
	if !supportedSiteMethod(method) {
		return nil, &siteError{Code: "invalid_argument", Message: "不支持的 HTTP 方法: " + method}
	}
	mutating := vpnAPIMutating(method, spec.path)
	if mutating && !yes {
		return nil, &siteError{Code: "confirmation_required", Message: "VPN API 可能改变远端状态，请加 --yes"}
	}
	if readOnlyMethod(method) && (dataJSON != "" || len(form) > 0 || len(files) > 0) {
		return nil, &siteError{Code: "invalid_argument", Message: "GET/HEAD/OPTIONS 只能使用 --param"}
	}
	if len(form) > 0 && dataJSON != "" {
		return nil, &siteError{Code: "invalid_argument", Message: "--data-json 不能与 --form/--file 同时使用"}
	}
	base, cookie, session, err := vpnConnection(native)
	if err != nil {
		return nil, err
	}
	requestPath := resolveVPNPath(spec.path, pathArgs, params)
	target, targetErr := vpnAPIPath(base, requestPath, native)
	if targetErr != nil {
		return nil, targetErr
	}
	request := siteRequest{Target: target, CookieFile: cookie, Method: method, Params: nil, Headers: vpnHeaders(base, target, session, native), Yes: yes, Output: output, RequireLogin: true, ReadOnly: !mutating}
	request.Params = nil
	for _, item := range params {
		query := target.Query()
		query.Add(item.name, item.value)
		target.RawQuery = query.Encode()
	}
	request.Target = target
	if len(form) > 0 {
		request.Data = form
		request.Files = files
	} else if len(files) > 0 {
		request.Files = files
	} else if dataJSON != "" {
		body, parseErr := readJSONArgument(dataJSON)
		if parseErr != nil {
			return nil, parseErr
		}
		request.JSON, request.HasJSON = body, true
	}
	result, runErr := a.execute(ctx, request)
	if runErr != nil {
		return nil, runErr
	}
	if vpnResponseCode(result) == "3010" {
		return nil, &siteError{Code: "login_required", Message: "VPN 会话已失效"}
	}
	result["vpn"] = true
	result["request"] = map[string]any{"name": func() string {
		if name != "" {
			return name
		}
		return vpnAPIName(spec.path)
	}(), "method": method, "path": spec.path, "native": native}
	return result, nil
}

func splitInline(value string) (string, string, bool) {
	if strings.HasPrefix(value, "--") {
		if name, item, found := strings.Cut(value, "="); found {
			return name, item, true
		}
	}
	return value, "", false
}

func resolveVPNPath(path string, pathArgs []string, params []pair) string {
	for _, value := range pathArgs {
		if index := strings.Index(path, "{"); index >= 0 {
			end := strings.Index(path[index:], "}")
			if end > 0 {
				path = path[:index] + url.PathEscape(value) + path[index+end+1:]
				continue
			}
		}
		path = strings.TrimRight(path, "/") + "/" + url.PathEscape(value)
	}
	return pathWithParams(path, params)
}

func pathWithParams(path string, params []pair) string {
	parsed, err := url.Parse(path)
	if err != nil {
		return path
	}
	query := parsed.Query()
	for _, item := range params {
		query.Add(item.name, item.value)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func vpnConnection(native bool) (*url.URL, string, map[string]any, *siteError) {
	defaultBase := "https://vpn.csust.edu.cn"
	baseEnv, cookieEnv, sessionEnv := "CSUST_VPN_BASE_URL", "CSUST_VPN_COOKIE_FILE", "CSUST_VPN_SESSION_FILE"
	if native {
		defaultBase = "http://127.0.0.1:30303"
		baseEnv, cookieEnv, sessionEnv = "CSUST_VPN_NATIVE_URL", "CSUST_VPN_NATIVE_COOKIE_FILE", "CSUST_VPN_NATIVE_SESSION_FILE"
	}
	baseValue := os.Getenv(baseEnv)
	if baseValue == "" {
		baseValue = defaultBase
	}
	base, err := url.Parse(baseValue)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") || base.User != nil {
		return nil, "", nil, &siteError{Code: "invalid_argument", Message: "VPN 基地址无效"}
	}
	cookie := os.Getenv(cookieEnv)
	if cookie == "" {
		home, _ := os.UserHomeDir()
		cookie = filepath.Join(home, ".config", "csust-cli", "vpn-cookies.txt")
		if native {
			cookie = filepath.Join(home, ".config", "csust-cli", "vpn-native-cookies.txt")
		}
	}
	sessionFile := os.Getenv(sessionEnv)
	if sessionFile == "" {
		home, _ := os.UserHomeDir()
		sessionFile = filepath.Join(home, ".config", "csust-cli", "vpn-session.json")
		if native {
			sessionFile = filepath.Join(home, ".config", "csust-cli", "vpn-native-session.json")
		}
	}
	session := map[string]any{}
	if info, statErr := os.Lstat(sessionFile); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, "", nil, &siteError{Code: "session_error", Message: "VPN 会话文件必须是普通文件且不能是符号链接"}
		}
		content, readErr := os.ReadFile(sessionFile)
		if readErr != nil {
			return nil, "", nil, &siteError{Code: "session_error", Message: "无法读取 VPN 会话文件: " + readErr.Error()}
		}
		if len(strings.TrimSpace(string(content))) > 0 {
			if jsonErr := json.Unmarshal(content, &session); jsonErr != nil {
				return nil, "", nil, &siteError{Code: "session_error", Message: "VPN 会话文件不是有效 JSON"}
			}
		}
	}
	return base, expandUserPath(cookie), session, nil
}

func vpnHeaders(base, target *url.URL, session map[string]any, native bool) []pair {
	originURL := (&url.URL{Scheme: base.Scheme, Host: base.Host}).String()
	startPath := "/enclient/start.html"
	if native {
		startPath = "/api/v1/local/info"
	}
	headers := []pair{{"Accept", "application/json,text/plain,*/*"}, {"Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8"}, {"ajax-Flow", "ajaxFlow"}, {"UseMode", "0"}, {"userProtocolState", "1"}, {"Referer", originURL + startPath}, {"Origin", originURL}}
	if !native {
		headers = append(headers, pair{"Cookie", "BROWSER_LOGIN=1; UseMode=0"})
	}
	if token, ok := session["token"].(string); ok && token != "" && !native {
		headers = append(headers, pair{"Authorization", "Bearer " + token})
	}
	_ = target
	return headers
}

func (a NativeSite) runVPNResource(ctx context.Context, args []string) (map[string]any, *siteError) {
	name, resourcePath, output := "", "", ""
	native := false
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		if arg == "--json" {
			continue
		}
		if arg == "--native" {
			if inline {
				return nil, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			native = true
			continue
		}
		if !inline {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return nil, &siteError{Code: "invalid_argument", Message: arg + " 缺少参数值"}
			}
			index++
			value = args[index]
		}
		switch arg {
		case "--name":
			name = value
		case "--path":
			resourcePath = value
		case "--output":
			output = expandUserPath(value)
		default:
			return nil, &siteError{Code: "invalid_argument", Message: "vpn resource 参数无效: " + arg}
		}
	}
	var resource vpnResource
	found := false
	for _, item := range vpnResources {
		if item.name == name {
			resource, found = item, true
			break
		}
	}
	if !found {
		return nil, &siteError{Code: "unknown_resource", Message: "未知 VPN 资源: " + name}
	}
	if resourcePath == "" {
		return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed", "resource": map[string]any{"name": resource.name, "web_path": resource.webPath, "native_path": resource.nativePath, "kind": resource.kind}}, nil
	}
	if output == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "下载 VPN 资源时必须提供 --output"}
	}
	decoded, err := url.PathUnescape(resourcePath)
	if err != nil || strings.Contains(decoded, "\\") || strings.Contains(decoded, "..") {
		return nil, &siteError{Code: "invalid_path", Message: "VPN 资源路径不能包含目录跳转"}
	}
	template := resource.webPath
	if native {
		template = resource.nativePath
	}
	if template == "" {
		return nil, &siteError{Code: "unsupported", Message: "该 VPN 资源不支持当前请求模式"}
	}
	base, cookie, session, connErr := vpnConnection(native)
	if connErr != nil {
		return nil, connErr
	}
	requestPath := strings.Replace(template, "{path}", url.PathEscape(strings.TrimLeft(resourcePath, "/")), 1)
	target, pathErr := vpnAPIPath(base, requestPath, native)
	if pathErr != nil {
		return nil, pathErr
	}
	result, runErr := a.execute(ctx, siteRequest{Target: target, CookieFile: cookie, Method: "GET", Headers: vpnHeaders(base, target, session, native), Output: output, RequireLogin: true, ReadOnly: true, Yes: true})
	if runErr != nil {
		return nil, runErr
	}
	result["resource"] = resource.name
	return result, nil
}

func (a NativeSite) runVPNSSOURL(ctx context.Context, args []string) (map[string]any, *siteError) {
	if err := onlyJSONArgs(args); err != nil {
		return nil, err
	}
	base, cookie, session, connErr := vpnConnection(false)
	if connErr != nil {
		return nil, connErr
	}
	target, pathErr := vpnAPIPath(base, "/api/users/custom/page/login/cfg/select", false)
	if pathErr != nil {
		return nil, pathErr
	}
	result, runErr := a.execute(ctx, siteRequest{Target: target, CookieFile: cookie, Method: "POST", Headers: vpnHeaders(base, target, session, false), JSON: map[string]any{}, HasJSON: true, ReadOnly: true, Yes: true})
	if runErr != nil {
		return nil, runErr
	}
	configured := ""
	if response, ok := result["response"].(map[string]any); ok {
		if payload, ok := response["json"].(map[string]any); ok {
			if data, ok := payload["data"].(map[string]any); ok {
				configured, _ = data["casLoginUrl"].(string)
			}
		}
	}
	if configured == "" {
		configured = "/enclient/api/users/admin/custom/page/login/sso/cas"
	}
	urlValue, urlErr := vpnAPIPath(base, configured, false)
	if urlErr != nil {
		return nil, urlErr
	}
	result["url"] = safeSiteURL(urlValue)
	return result, nil
}

func (a NativeSite) runVPNStatus(ctx context.Context, args []string) (map[string]any, *siteError) {
	native := false
	for _, arg := range args {
		switch arg {
		case "--json":
		case "--native":
			native = true
		default:
			return nil, &siteError{Code: "invalid_argument", Message: "vpn status 参数无效: " + arg}
		}
	}
	base, cookie, session, connErr := vpnConnection(native)
	if connErr != nil {
		return nil, connErr
	}
	if token, ok := session["token"].(string); !ok || token == "" {
		if _, statErr := os.Stat(cookie); os.IsNotExist(statErr) {
			return map[string]any{"ok": true, "logged_in": false, "native": native, "session_file": sessionPath(native)}, nil
		}
	}
	target, pathErr := vpnAPIPath(base, "/api/users/info", native)
	if pathErr != nil {
		return nil, pathErr
	}
	result, runErr := a.execute(ctx, siteRequest{Target: target, CookieFile: cookie, Method: "GET", Headers: vpnHeaders(base, target, session, native), RequireLogin: true, ReadOnly: true})
	if runErr != nil {
		if runErr.Code == "login_required" || runErr.Code == "business_rejected" {
			return map[string]any{"ok": true, "logged_in": false, "native": native, "session_file": sessionPath(native)}, nil
		}
		return nil, runErr
	}
	if code := vpnResponseCode(result); code != "" && code != "200" {
		return map[string]any{"ok": true, "logged_in": false, "native": native, "session_file": sessionPath(native)}, nil
	}
	return map[string]any{"ok": true, "logged_in": true, "native": native, "session_file": sessionPath(native), "user": result["response"]}, nil
}

func vpnResponseCode(result map[string]any) string {
	response, ok := result["response"].(map[string]any)
	if !ok {
		return ""
	}
	payload, ok := response["json_internal"].(map[string]any)
	if !ok {
		payload, ok = response["json"].(map[string]any)
	}
	if !ok {
		return ""
	}
	return fmt.Sprint(payload["code"])
}

func (a NativeSite) runVPNLogout(ctx context.Context, args []string) (map[string]any, *siteError) {
	native := false
	for _, arg := range args {
		switch arg {
		case "--json":
		case "--native":
			native = true
		case "--yes":
		default:
			return nil, &siteError{Code: "invalid_argument", Message: "vpn logout 参数无效: " + arg}
		}
	}
	base, cookie, session, connErr := vpnConnection(native)
	if connErr != nil {
		return nil, connErr
	}
	remote := map[string]any{"skipped": true}
	if _, statErr := os.Stat(cookie); statErr == nil || session["token"] != nil {
		target, pathErr := vpnAPIPath(base, "/api/users/auth/logout", native)
		if pathErr != nil {
			return nil, pathErr
		}
		value, runErr := a.execute(ctx, siteRequest{Target: target, CookieFile: cookie, Method: "POST", Headers: vpnHeaders(base, target, session, native), JSON: map[string]any{}, HasJSON: true, Yes: true})
		if runErr != nil {
			return nil, runErr
		}
		remote = value
	}
	if removeErr := removeCookieFile(cookie); removeErr != nil {
		return nil, &siteError{Code: "cookie_write_failed", Message: removeErr.Error()}
	}
	if removeErr := removeCookieFile(sessionPath(native)); removeErr != nil {
		return nil, &siteError{Code: "session_write_failed", Message: removeErr.Error()}
	}
	return map[string]any{"ok": true, "submitted": remote["submitted"], "confirmed": true, "evidence": "confirmed", "logged_out": true, "native": native, "remote": remote}, nil
}

func sessionPath(native bool) string {
	name := os.Getenv("CSUST_VPN_SESSION_FILE")
	if native {
		name = os.Getenv("CSUST_VPN_NATIVE_SESSION_FILE")
	}
	if name != "" {
		return expandUserPath(name)
	}
	home, _ := os.UserHomeDir()
	if native {
		return filepath.Join(home, ".config", "csust-cli", "vpn-native-session.json")
	}
	return filepath.Join(home, ".config", "csust-cli", "vpn-session.json")
}

func renderVPNResult(result map[string]any) string {
	if result["downloaded"] == true {
		return fmt.Sprintf("已保存：%v（%v bytes）\n", result["output"], result["bytes"])
	}
	if result["logged_in"] == false {
		return "VPN 未登录\n"
	}
	if result["logged_out"] == true {
		return "VPN 已退出登录\n"
	}
	encoded, _ := json.MarshalIndent(result, "", "  ")
	return string(encoded) + "\n"
}
