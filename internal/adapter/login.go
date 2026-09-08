package adapter

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
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
}

func (a NativeSite) runLoginCommand(ctx context.Context, args []string, jsonMode bool) (bool, []byte, []byte, int, error) {
	if len(args) == 0 || (args[0] != "login" && args[0] != "logout") || containsHelp(args[1:]) {
		return false, nil, nil, 0, nil
	}
	if args[0] == "logout" {
		if err := removeCookieFile(academicCookiePath()); err != nil {
			return loginOutput(jsonMode, nil, &siteError{Code: "cookie_write_failed", Message: err.Error()})
		}
		return loginOutput(jsonMode, map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed", "logged_out": true, "cookie_file": academicCookiePath()}, nil)
	}
	options, parseErr := parseLoginOptions(args[1:])
	if parseErr != nil {
		return loginOutput(jsonMode, nil, parseErr)
	}
	result, err := a.loginAcademic(ctx, options)
	return loginOutput(jsonMode, result, err)
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
		password, err := io.ReadAll(os.Stdin)
		if err != nil {
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
	if parseErr != nil || base == nil || base.Host == "" {
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
	account, password, err := credentialsGo(options.username, options.password)
	if err != nil {
		return nil, err
	}
	if options.captcha == "" {
		options.captcha = envValueGo("CSUST_CAPTCHA")
	}
	return a.loginSSOWith(ctx, serviceTarget, serviceTarget, account, password, options, cookiePath, false)
}

func (a NativeSite) loginSSOWith(ctx context.Context, serviceTarget, probeTarget *url.URL, account, password string, options loginOptions, cookiePath string, useHandoff bool) (map[string]any, *siteError) {
	if err := removeCookieIfPresent(cookiePath); err != nil {
		return nil, &siteError{Code: "cookie_write_failed", Message: err.Error()}
	}
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
	encrypted, encryptErr := encryptCASPassword(password, salt)
	if encryptErr != nil {
		return nil, encryptErr
	}
	fields = append(fields, pair{"username", account}, pair{"password", encrypted}, pair{"_eventId", "submit"}, pair{"cllt", "userNameLogin"}, pair{"dllt", "generalLogin"}, pair{"lt", ""})
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
	loginResult, err = a.loginHTTP(ctx, siteRequest{Target: actionURL, SessionTarget: &session, CookieFile: cookiePath, Method: "POST", Data: fields, Headers: []pair{{"Referer", responseURL}}, ReadOnly: true, AllowSSO: true, Yes: true})
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
		return nil, &siteError{Code: "authentication_failed", Message: "统一认证回跳成功，但服务未建立有效会话", Details: map[string]any{"cause": probeErr.Code}}
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed", "username": account, "auth": "sso", "cookie_file": cookiePath, "service": safeSiteURL(serviceTarget), "attempts": 1}, nil
}

func (a NativeSite) loginLocal(ctx context.Context, base *url.URL, account, password string, options loginOptions) (map[string]any, *siteError) {
	cookiePath := academicCookiePath()
	if err := removeCookieIfPresent(cookiePath); err != nil {
		return nil, &siteError{Code: "cookie_write_failed", Message: err.Error()}
	}
	root := *base
	if _, err := a.loginHTTP(ctx, siteRequest{Target: &root, SessionTarget: &root, CookieFile: cookiePath, Method: "GET", ReadOnly: true, Yes: true}); err != nil {
		return nil, err
	}
	path, fetchErr := a.fetchLoginCaptcha(ctx, root, cookiePath, false, options.captchaImage)
	if fetchErr != nil {
		return nil, fetchErr
	}
	if options.captcha == "" {
		return nil, &siteError{Code: "captcha_required", Message: "旧教务登录需要验证码，请提供 --captcha", Details: map[string]any{"captcha_image": path}}
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
	loginTarget := root
	loginTarget.Path = "/Logon.do"
	loginTarget.RawQuery = "method=logon"
	response, requestErr := a.loginHTTP(ctx, siteRequest{Target: &loginTarget, SessionTarget: &root, CookieFile: cookiePath, Method: "POST", Data: []pair{{"userAccount", ""}, {"userPassword", ""}, {"RANDOMCODE", options.captcha}, {"encoded", encoded}}, Headers: []pair{{"Referer", root.String()}}, ReadOnly: true, Yes: true})
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
	probe := root
	probe.Path = academicProbePath
	if _, probeErr := a.loginHTTP(ctx, siteRequest{Target: &probe, SessionTarget: &root, CookieFile: cookiePath, Method: "GET", RequireLogin: true, ReadOnly: true, Yes: true}); probeErr != nil {
		return nil, &siteError{Code: "authentication_failed", Message: "登录失败，教务系统未建立有效会话", Details: map[string]any{"cause": probeErr.Code}}
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed", "username": account, "auth": "local", "cookie_file": cookiePath, "attempts": 1}, nil
}

func (a NativeSite) loginHTTP(ctx context.Context, request siteRequest) (map[string]any, *siteError) {
	return a.execute(ctx, request)
}

func loginBody(result map[string]any) (string, string, *siteError) {
	response, ok := result["response"].(map[string]any)
	if !ok {
		return "", "", &siteError{Code: "parse_error", Message: "认证响应不是文本页面"}
	}
	body, _ := response["body"].(string)
	pageURL, _ := response["url"].(string)
	return body, pageURL, nil
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
	if document.first("form", "loginForm") != nil || document.first("form", "pwdFromId") != nil {
		return true
	}
	text := pageDisplayText(document)
	return strings.Contains(text, "请输入账号") && strings.Contains(text, "用户登录")
}
func loginFailure(source string) *siteError {
	lower := strings.ToLower(source)
	if strings.Contains(source, "验证码错误") || strings.Contains(source, "验证码不正确") || strings.Contains(source, "随机码错误") {
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
