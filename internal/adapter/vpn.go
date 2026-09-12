package adapter

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func (a NativeSite) runVPNCommand(ctx context.Context, args []string, jsonMode bool) (bool, []byte, []byte, int, error) {
	if len(args) == 0 || args[0] != "vpn" {
		return false, nil, nil, 0, nil
	}
	if len(args) < 2 || containsHelp(args[1:]) {
		return false, nil, nil, 0, nil
	}
	if args[1] == "login" {
		var result map[string]any
		var runErr *siteError
		if len(args) > 2 {
			switch args[2] {
			case "second-auth", "complete":
				result, runErr = a.runVPNSecondAuth(ctx, args[3:])
			case "reset-password", "forgot-password":
				result, runErr = a.runVPNResetPassword(ctx, args[3:])
			default:
				result, runErr = a.runVPNLogin(ctx, args[2:])
			}
		} else {
			result, runErr = a.runVPNLogin(ctx, args[2:])
		}
		if runErr != nil {
			if jsonMode {
				return true, errorJSON(runErr), nil, 2, nil
			}
			return true, nil, []byte("错误: " + runErr.Error() + "\n"), 2, nil
		}
		if jsonMode {
			encoded, err := json.Marshal(stripSiteInternal(result))
			if err != nil {
				return true, nil, nil, 2, err
			}
			return true, encoded, nil, 0, nil
		}
		if result["pending"] == true {
			if result["operation"] == "login-reset-password-send-code" {
				return true, []byte("VPN 找回密码验证码已发送，请收到验证码后再次执行 reset-password。\n"), nil, 0, nil
			}
			return true, []byte(fmt.Sprintf("VPN 登录待完成二次认证：%v（代码 %v）\n", result["username"], result["code"])), nil, 0, nil
		}
		if result["operation"] == "login-reset-password" {
			return true, []byte("VPN 密码重置成功。\n"), nil, 0, nil
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
		encoded, err := json.Marshal(stripSiteInternal(result))
		if err != nil {
			return true, nil, nil, 2, err
		}
		return true, encoded, nil, 0, nil
	}
	return true, []byte(renderVPNResult(stripSiteInternal(result).(map[string]any))), nil, 0, nil
}

func (a NativeSite) executeVPNCommand(ctx context.Context, args []string) (map[string]any, *siteError) {
	command := args[0]
	switch command {
	case "status":
		return a.runVPNStatus(ctx, args[1:])
	case "logout":
		return a.runVPNLogout(ctx, args[1:])
	case "apps":
		return a.runVPNApps(ctx, args[1:])
	case "groups", "app-groups":
		return a.runVPNGroups(ctx, args[1:])
	case "messages":
		return a.runVPNMessages(ctx, args[1:])
	case "message":
		return a.runVPNMessage(ctx, args[1:])
	case "approvals":
		return a.runVPNApprovals(ctx, args[1:])
	case "approval":
		return a.runVPNApproval(ctx, args[1:])
	case "devices":
		return a.runVPNDevices(ctx, args[1:])
	case "device":
		return a.runVPNDevice(ctx, args[1:])
	case "apply":
		return a.runVPNApply(ctx, args[1:])
	case "shares":
		return a.runVPNShares(ctx, args[1:])
	case "links":
		return a.runVPNLinks(ctx, args[1:])
	case "profile":
		return a.runVPNProfile(ctx, args[1:])
	case "otp":
		return a.runVPNOTP(ctx, args[1:])
	case "safe-space", "space":
		return a.runVPNSafeSpace(ctx, args[1:])
	case "usb":
		return a.runVPNUSB(ctx, args[1:])
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
		password, err := readBoundedSiteInput(os.Stdin)
		if err != nil {
			if errors.Is(err, errSiteRequestTooLarge) {
				return vpnLoginOptions{}, siteRequestTooLarge("标准输入密码")
			}
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

type vpnResetPasswordOptions struct {
	account, method, loginNumber, code, newPassword  string
	sendCode, yes, codeProvided, newPasswordProvided bool
}

func parseVPNResetPasswordOptions(args []string) (vpnResetPasswordOptions, *siteError) {
	options := vpnResetPasswordOptions{}
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		switch arg {
		case "--json":
		case "--account", "--username", "--method", "--type", "--login-number", "--code", "--new-password":
			var err *siteError
			value, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnResetPasswordOptions{}, err
			}
			switch arg {
			case "--account", "--username":
				options.account = strings.TrimSpace(value)
			case "--method", "--type":
				switch strings.ToLower(strings.TrimSpace(value)) {
				case "phone", "mobile":
					options.method = "phone"
				case "email", "mail":
					options.method = "email"
				default:
					return vpnResetPasswordOptions{}, &siteError{Code: "invalid_argument", Message: "--method 必须是 phone 或 email"}
				}
			case "--login-number":
				options.loginNumber = strings.TrimSpace(value)
			case "--code":
				options.code = strings.TrimSpace(value)
				options.codeProvided = true
			case "--new-password":
				options.newPassword = value
				options.newPasswordProvided = true
			}
		case "--new-password-stdin":
			if inline {
				return vpnResetPasswordOptions{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			options.newPasswordProvided = true
		case "--send-code":
			if inline {
				return vpnResetPasswordOptions{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			options.sendCode = true
		case "--yes":
			if inline {
				return vpnResetPasswordOptions{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			options.yes = true
		default:
			return vpnResetPasswordOptions{}, &siteError{Code: "invalid_argument", Message: "vpn login reset-password 参数无效: " + arg}
		}
	}
	if !options.yes {
		return vpnResetPasswordOptions{}, &siteError{Code: "confirmation_required", Message: "找回 VPN 密码会改变远端认证状态，请加 --yes"}
	}
	if options.sendCode {
		if options.account == "" || options.method == "" || options.loginNumber == "" {
			return vpnResetPasswordOptions{}, &siteError{Code: "invalid_argument", Message: "发送找回密码验证码必须提供 --account、--method 和 --login-number"}
		}
		if options.codeProvided || options.newPasswordProvided {
			return vpnResetPasswordOptions{}, &siteError{Code: "invalid_argument", Message: "--send-code 不能同时提供验证码或新密码，请收到验证码后再次执行"}
		}
	} else if options.code == "" {
		return vpnResetPasswordOptions{}, &siteError{Code: "invalid_argument", Message: "完成找回密码必须提供 --code"}
	}
	if strings.ContainsAny(options.account, "\r\n") || len([]rune(options.account)) > 256 {
		return vpnResetPasswordOptions{}, &siteError{Code: "invalid_argument", Message: "--account 不能超过 256 个字符或包含换行"}
	}
	if strings.ContainsAny(options.loginNumber, "\r\n") || len([]rune(options.loginNumber)) > 128 {
		return vpnResetPasswordOptions{}, &siteError{Code: "invalid_argument", Message: "--login-number 不能超过 128 个字符或包含换行"}
	}
	if strings.ContainsAny(options.code, "\r\n") || len([]rune(options.code)) > 64 {
		return vpnResetPasswordOptions{}, &siteError{Code: "invalid_argument", Message: "--code 不能超过 64 个字符或包含换行"}
	}
	return options, nil
}

type vpnResetPasswordState struct {
	account, method, loginNumber, reToken string
}

func vpnResetPasswordStateFromSession(session map[string]any) (vpnResetPasswordState, *siteError) {
	value, ok := session["resetPassword"].(map[string]any)
	if !ok {
		return vpnResetPasswordState{}, &siteError{Code: "password_reset_pending", Message: "没有待完成的 VPN 找回密码流程，请先使用 --send-code"}
	}
	state := vpnResetPasswordState{}
	state.account, _ = value["account"].(string)
	state.method, _ = value["method"].(string)
	state.loginNumber, _ = value["loginNumber"].(string)
	state.reToken, _ = value["reToken"].(string)
	if state.account == "" || state.method == "" || state.loginNumber == "" || state.reToken == "" {
		return vpnResetPasswordState{}, &siteError{Code: "password_reset_pending", Message: "VPN 找回密码流程状态不完整，请重新使用 --send-code"}
	}
	return state, nil
}

func vpnResetPasswordDynamicForm(payload map[string]any) (map[string]any, *siteError) {
	forms, err := vpnDynamicFlowForms(payload)
	if err != nil {
		return nil, err
	}
	form := map[string]any{"password": "", "newPassword": "", "userType": "1"}
	for _, stepForm := range forms {
		for key, value := range stepForm {
			form[key] = value
		}
	}
	for _, key := range []string{"account", "loginNum", "code", "password"} {
		if _, ok := form[key]; !ok {
			return nil, &siteError{Code: "response_error", Message: "VPN 找回密码动态表单缺少字段: " + key}
		}
	}
	return form, nil
}

func (a NativeSite) runVPNResetPassword(ctx context.Context, args []string) (map[string]any, *siteError) {
	options, parseErr := parseVPNResetPasswordOptions(args)
	if parseErr != nil {
		return nil, parseErr
	}
	if !options.sendCode {
		var secretErr *siteError
		options.newPassword, secretErr = businessSecret(args, "--new-password", "CSUST_VPN_NEW_PASSWORD")
		if secretErr != nil {
			return nil, secretErr
		}
		if options.newPassword == "" {
			return nil, &siteError{Code: "credentials_required", Message: "请使用 --new-password-stdin 或环境变量 CSUST_VPN_NEW_PASSWORD 提供新密码"}
		}
		if strings.ContainsAny(options.newPassword, "\r\n") || len([]rune(options.newPassword)) > 30 {
			return nil, &siteError{Code: "invalid_argument", Message: "新密码不能超过 30 个字符或包含换行"}
		}
	}

	base, cookie, session, connErr := vpnConnection(false)
	if connErr != nil {
		return nil, connErr
	}
	requestSession := map[string]any{}
	if options.sendCode {
		flowResult, flowErr := a.vpnJSONRequest(ctx, base, cookie, requestSession, "/api/users/flow/path", "POST", map[string]any{"scenes": "forgetPassword", "type": "forgetPassword"}, true)
		if flowErr != nil {
			return nil, flowErr
		}
		if vpnResponseCode(flowResult) != "200" {
			return nil, &siteError{Code: "business_rejected", Message: "VPN 找回密码流程获取失败", Details: map[string]any{"remote_code": vpnResponseCode(flowResult)}}
		}
		form, formErr := vpnResetPasswordDynamicForm(resultJSON(flowResult))
		if formErr != nil {
			return nil, formErr
		}
		form["account"] = options.account
		findPayload, findErr := a.vpnResetPasswordAPI(ctx, base, cookie, requestSession, "/api/users/reset/password/find/verify/type", form, true)
		if findErr != nil {
			return nil, findErr
		}
		findData, ok := findPayload["data"].(map[string]any)
		if !ok {
			return nil, &siteError{Code: "response_error", Message: "VPN 找回密码账号校验响应缺少 reToken"}
		}
		reToken, _ := findData["reToken"].(string)
		if reToken == "" {
			return nil, &siteError{Code: "response_error", Message: "VPN 找回密码账号校验响应缺少 reToken"}
		}
		form["type"], form["loginNum"], form["reToken"] = options.method, options.loginNumber, reToken
		if _, sendErr := a.vpnResetPasswordAPI(ctx, base, cookie, requestSession, "/api/users/reset/password/send/code", form, false); sendErr != nil {
			return nil, sendErr
		}
		if saveErr := saveVPNSession(sessionPath(false), map[string]any{"resetPassword": map[string]any{"account": options.account, "method": options.method, "loginNumber": options.loginNumber, "reToken": reToken}}); saveErr != nil {
			return nil, saveErr
		}
		return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "VPN reset/password/send/code code 200", "vpn": true, "operation": "login-reset-password-send-code", "api": "/api/users/reset/password/send/code", "method": options.method, "pending": true, "next": "reset-password"}, nil
	}

	state, stateErr := vpnResetPasswordStateFromSession(session)
	if stateErr != nil {
		return nil, stateErr
	}
	for name, values := range map[string][2]string{
		"account":      {options.account, state.account},
		"method":       {options.method, state.method},
		"login-number": {options.loginNumber, state.loginNumber},
	} {
		if values[0] != "" && values[0] != values[1] {
			return nil, &siteError{Code: "invalid_argument", Message: "本次参数的 " + name + " 与待完成的找回密码流程不一致"}
		}
	}
	flowResult, flowErr := a.vpnJSONRequest(ctx, base, cookie, requestSession, "/api/users/flow/path", "POST", map[string]any{"scenes": "forgetPassword", "type": "forgetPassword"}, true)
	if flowErr != nil {
		return nil, flowErr
	}
	if vpnResponseCode(flowResult) != "200" {
		return nil, &siteError{Code: "business_rejected", Message: "VPN 找回密码流程获取失败", Details: map[string]any{"remote_code": vpnResponseCode(flowResult)}}
	}
	form, formErr := vpnResetPasswordDynamicForm(resultJSON(flowResult))
	if formErr != nil {
		return nil, formErr
	}
	form["account"], form["type"], form["loginNum"], form["code"], form["reToken"] = state.account, state.method, state.loginNumber, options.code, state.reToken
	if _, verifyErr := a.vpnResetPasswordAPI(ctx, base, cookie, requestSession, "/api/users/reset/password/terminal/verify/code", form, false); verifyErr != nil {
		return nil, verifyErr
	}
	parts := strings.Split(state.reToken, "-")
	if len(parts) < 3 || len([]byte(parts[2])) != aes.BlockSize {
		return nil, &siteError{Code: "vpn_protocol_error", Message: "VPN 找回密码 reToken 缺少可用于加密密码的 16 字节密钥"}
	}
	encrypted, encryptErr := encryptVPNPassword(options.newPassword, parts[2])
	if encryptErr != nil {
		return nil, encryptErr
	}
	form["password"], form["newPassword"] = encrypted, options.newPassword
	if _, verifyErr := a.vpnResetPasswordAPI(ctx, base, cookie, requestSession, "/api/users/reset/password/verify/code", form, false); verifyErr != nil {
		return nil, verifyErr
	}
	if saveErr := saveVPNSession(sessionPath(false), map[string]any{"resetPassword": nil}); saveErr != nil {
		return nil, saveErr
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "VPN reset/password/verify/code code 200", "vpn": true, "operation": "login-reset-password", "api": "/api/users/reset/password/verify/code", "session_file": sessionPath(false)}, nil
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
			pendingToken := seed
			if loginData, ok := resultJSON(loginResult)["data"].(map[string]any); ok {
				if value, valueOK := loginData["entoken"].(string); valueOK && value != "" {
					pendingToken = value
				}
			}
			session["pendingToken"] = pendingToken
			session["token"] = pendingToken
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

type vpnSecondAuthOptions struct {
	method, loginNumber, code string
	sendCode                  bool
	yes                       bool
}

func parseVPNSecondAuthOptions(args []string) (vpnSecondAuthOptions, *siteError) {
	options := vpnSecondAuthOptions{}
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		switch arg {
		case "--json":
		case "--method", "--type":
			var err *siteError
			value, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnSecondAuthOptions{}, err
			}
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "phone", "mobile":
				options.method = "phone"
			case "email", "mail":
				options.method = "email"
			case "radius", "radiustop", "radius-top":
				options.method = "radiusTop"
			default:
				return vpnSecondAuthOptions{}, &siteError{Code: "invalid_argument", Message: "--method 必须是 phone、email 或 radius"}
			}
		case "--login-number":
			var err *siteError
			options.loginNumber, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnSecondAuthOptions{}, err
			}
		case "--code":
			var err *siteError
			options.code, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnSecondAuthOptions{}, err
			}
		case "--send-code":
			if inline {
				return vpnSecondAuthOptions{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			options.sendCode = true
		case "--yes":
			if inline {
				return vpnSecondAuthOptions{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			options.yes = true
		default:
			return vpnSecondAuthOptions{}, &siteError{Code: "invalid_argument", Message: "vpn login second-auth 参数无效: " + arg}
		}
	}
	if options.method == "" {
		return vpnSecondAuthOptions{}, &siteError{Code: "invalid_argument", Message: "必须提供 --method phone、email 或 radius"}
	}
	if options.sendCode {
		if !options.yes {
			return vpnSecondAuthOptions{}, &siteError{Code: "confirmation_required", Message: "发送 VPN 二次认证验证码会触发短信或邮件，请加 --yes"}
		}
		if options.method == "radiusTop" {
			return vpnSecondAuthOptions{}, &siteError{Code: "invalid_argument", Message: "radius 认证不发送验证码"}
		}
		if strings.TrimSpace(options.loginNumber) == "" {
			return vpnSecondAuthOptions{}, &siteError{Code: "invalid_argument", Message: "发送验证码必须提供 --login-number"}
		}
		if options.code != "" {
			return vpnSecondAuthOptions{}, &siteError{Code: "invalid_argument", Message: "--send-code 不能同时提供 --code，请收到验证码后再次执行"}
		}
	} else if strings.TrimSpace(options.code) == "" {
		return vpnSecondAuthOptions{}, &siteError{Code: "invalid_argument", Message: "完成二次认证必须提供 --code"}
	}
	if strings.ContainsAny(options.loginNumber, "\r\n") || len([]rune(options.loginNumber)) > 128 {
		return vpnSecondAuthOptions{}, &siteError{Code: "invalid_argument", Message: "--login-number 不能超过 128 个字符或包含换行"}
	}
	if strings.ContainsAny(options.code, "\r\n") || len([]rune(options.code)) > 64 {
		return vpnSecondAuthOptions{}, &siteError{Code: "invalid_argument", Message: "--code 不能超过 64 个字符或包含换行"}
	}
	return options, nil
}

func (a NativeSite) runVPNSecondAuth(ctx context.Context, args []string) (map[string]any, *siteError) {
	options, parseErr := parseVPNSecondAuthOptions(args)
	if parseErr != nil {
		return nil, parseErr
	}
	base, cookie, session, connErr := vpnConnection(false)
	if connErr != nil {
		return nil, connErr
	}
	pendingToken, _ := session["pendingToken"].(string)
	if pendingToken == "" {
		return nil, &siteError{Code: "authentication_pending", Message: "没有待完成的 VPN 二次认证，请先执行 vpn login"}
	}
	session["token"] = pendingToken
	if saveErr := saveVPNSession(sessionPath(false), session); saveErr != nil {
		return nil, saveErr
	}

	flowPath := "/api/users/flow/path"
	flowResult, flowErr := a.vpnJSONRequest(ctx, base, cookie, session, flowPath, "POST", map[string]any{"scenes": "secondAuth", "type": options.method}, true)
	if flowErr != nil {
		if flowErr.Code != "http_error" {
			return nil, flowErr
		}
		return a.finishVPNSecondAuth(ctx, options, nil, true)
	}
	if vpnResponseCode(flowResult) == "200" {
		form, formErr := vpnSecondAuthDynamicForm(resultJSON(flowResult), options)
		if formErr != nil {
			return nil, formErr
		}
		return a.finishVPNSecondAuth(ctx, options, form, false)
	}
	return a.finishVPNSecondAuth(ctx, options, nil, true)
}

func vpnDynamicFlowForms(payload map[string]any) ([]map[string]any, *siteError) {
	data, ok := payload["data"].([]any)
	if !ok || len(data) == 0 {
		return nil, &siteError{Code: "response_error", Message: "VPN 动态流程缺少表单数据"}
	}
	first, ok := data[0].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "response_error", Message: "VPN 动态流程格式无效"}
	}
	steps, ok := first["steps"].([]any)
	if !ok || len(steps) == 0 {
		return nil, &siteError{Code: "response_error", Message: "VPN 动态流程缺少步骤"}
	}
	forms := make([]map[string]any, 0, len(steps))
	for _, stepItem := range steps {
		step, ok := stepItem.(map[string]any)
		if !ok {
			continue
		}
		formConfig, ok := step["formConfig"].(map[string]any)
		if !ok {
			continue
		}
		formPage, ok := formConfig["formPage"].([]any)
		if !ok {
			continue
		}
		form := map[string]any{}
		for _, pageItem := range formPage {
			page, ok := pageItem.(map[string]any)
			if !ok {
				continue
			}
			components, _ := page["components"].([]any)
			for _, componentItem := range components {
				component, ok := componentItem.(map[string]any)
				if !ok {
					continue
				}
				refKey, _ := component["refKey"].(string)
				if refKey == "" {
					continue
				}
				value := any("")
				if hidden, _ := component["hidden"].(bool); hidden {
					if attributes, attributesOK := component["attributes"].(map[string]any); attributesOK {
						if placeholder, exists := attributes["placeholder"]; exists {
							value = placeholder
						}
					}
				}
				form[refKey] = value
			}
		}
		if len(form) > 0 {
			forms = append(forms, form)
		}
	}
	if len(forms) == 0 {
		return nil, &siteError{Code: "response_error", Message: "VPN 动态流程没有可提交字段"}
	}
	return forms, nil
}

func vpnSecondAuthDynamicForm(payload map[string]any, options vpnSecondAuthOptions) (map[string]any, *siteError) {
	forms, err := vpnDynamicFlowForms(payload)
	if err != nil {
		return nil, err
	}
	form := forms[0]
	if options.method != "radiusTop" {
		if _, exists := form["loginNum"]; !exists {
			return nil, &siteError{Code: "response_error", Message: "VPN 二次认证表单缺少登录号码字段"}
		}
		if strings.TrimSpace(options.loginNumber) == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "完成该二次认证必须提供 --login-number"}
		}
		form["loginNum"] = options.loginNumber
	}
	if options.sendCode {
		return form, nil
	}
	if _, exists := form["code"]; exists {
		form["code"] = options.code
	} else if options.method == "radiusTop" {
		if _, exists := form["radius"]; !exists {
			return nil, &siteError{Code: "response_error", Message: "VPN RADIUS 二次认证表单缺少令牌字段"}
		}
		form["radius"] = options.code
	} else {
		return nil, &siteError{Code: "response_error", Message: "VPN 二次认证表单缺少验证码字段"}
	}
	return form, nil
}

func (a NativeSite) finishVPNSecondAuth(ctx context.Context, options vpnSecondAuthOptions, form map[string]any, legacy bool) (map[string]any, *siteError) {
	if options.sendCode {
		path := "/api/users/second/send/code"
		body := form
		if legacy {
			body = map[string]any{"loginNum": options.loginNumber, "type": options.method}
		}
		payload, runErr := a.vpnBusinessJSON(ctx, path, "POST", body, false)
		if runErr != nil {
			return nil, runErr
		}
		return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "VPN second/send/code code 200", "vpn": true, "operation": "login-second-auth-send-code", "api": path, "method": vpnSecondAuthMethodName(options.method), "pending": true, "next": "second-auth", "data": payload["data"]}, nil
	}

	path := "/api/users/auth/secondAuth"
	if legacy {
		form = map[string]any{"code": options.code}
		if options.method == "radiusTop" {
			form["secondMethod"] = "radiusTop"
		}
	}
	payload, runErr := a.vpnBusinessJSON(ctx, path, "POST", form, false)
	if runErr != nil {
		return nil, runErr
	}
	_, _, session, connErr := vpnConnection(false)
	if connErr != nil {
		return nil, connErr
	}
	loginData, ok := payload["data"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "authentication_failed", Message: "VPN 二次认证成功响应缺少会话数据"}
	}
	token, _ := loginData["token"].(string)
	if token == "" {
		token, _ = loginData["entoken"].(string)
	}
	if token == "" {
		return nil, &siteError{Code: "authentication_failed", Message: "VPN 二次认证成功响应缺少会话令牌"}
	}
	for _, name := range []string{"token", "refreshToken", "account", "username", "name", "userId", "id"} {
		if value, exists := loginData[name]; exists {
			session[name] = value
		}
	}
	session["token"] = token
	session["pendingToken"] = nil
	session["auth"] = "local"
	if saveErr := saveVPNSession(sessionPath(false), session); saveErr != nil {
		return nil, saveErr
	}
	infoPayload, infoErr := a.vpnBusinessJSON(ctx, "/api/users/info", "GET", nil, true)
	if infoErr != nil {
		return nil, &siteError{Code: "authentication_failed", Message: "VPN 二次认证后未建立有效会话", Details: map[string]any{"cause": infoErr.Code}}
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed", "username": session["account"], "auth": "local", "session_file": sessionPath(false), "user": redactSiteJSON(infoPayload["data"])}, nil
}

func vpnSecondAuthMethodName(method string) string {
	if method == "radiusTop" {
		return "radius"
	}
	return method
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
	if err := validateSessionFile(path); err != nil {
		return &siteError{Code: "session_write_failed", Message: "无法保存 VPN 会话: " + err.Error()}
	}
	err := withSiteFileLock(path, func() error {
		merged := map[string]any{}
		if content, readErr := os.ReadFile(path); readErr == nil && strings.TrimSpace(string(content)) != "" {
			if jsonErr := json.Unmarshal(content, &merged); jsonErr != nil {
				return fmt.Errorf("VPN 会话文件不是有效 JSON: %w", jsonErr)
			}
		} else if readErr != nil && !os.IsNotExist(readErr) {
			return readErr
		}
		for key, value := range session {
			if value == nil {
				delete(merged, key)
				continue
			}
			merged[key] = value
		}
		content, marshalErr := json.MarshalIndent(merged, "", "  ")
		if marshalErr != nil {
			return fmt.Errorf("无法序列化 VPN 会话: %w", marshalErr)
		}
		return writeCookieFileUnlocked(path, string(content)+"\n")
	})
	if err != nil {
		return &siteError{Code: "session_write_failed", Message: "无法保存 VPN 会话: " + err.Error()}
	}
	return nil
}

func (a NativeSite) vpnJSONRequest(ctx context.Context, base *url.URL, cookie string, session map[string]any, path, method string, body any, readOnly bool) (map[string]any, *siteError) {
	target, pathErr := vpnAPIPath(base, path, false)
	if pathErr != nil {
		return nil, pathErr
	}
	request := siteRequest{Target: target, CookieFile: cookie, Method: method, Headers: vpnHeaders(base, target, session, false), RequireLogin: false, AllowBusinessFailure: true, ReadOnly: readOnly, Yes: true, RawJSON: true}
	if method != "GET" && method != "HEAD" && method != "OPTIONS" {
		request.JSON, request.HasJSON = body, true
	}
	result, runErr := a.execute(ctx, request)
	if runErr != nil {
		return nil, runErr
	}
	return result, nil
}

func (a NativeSite) vpnResetPasswordAPI(ctx context.Context, base *url.URL, cookie string, session map[string]any, path string, body any, readOnly bool) (map[string]any, *siteError) {
	result, runErr := a.vpnJSONRequest(ctx, base, cookie, session, path, "POST", body, readOnly)
	if runErr != nil {
		return nil, runErr
	}
	payload := resultJSON(result)
	code := vpnResponseCode(result)
	if !readOnly && code == "" {
		return nil, &siteError{Code: "mutation_unverified", Message: "VPN 找回密码写操作未返回 code=200，无法确认成功", Details: map[string]any{"api": path}}
	}
	if code != "" && code != "200" {
		details := map[string]any{"api": path, "remote_code": code}
		if payload != nil && payload["data"] != nil {
			details["data"] = redactSiteJSON(payload["data"])
		}
		return nil, &siteError{Code: "business_rejected", Message: "VPN 找回密码请求失败", Details: details}
	}
	if payload == nil {
		return nil, &siteError{Code: "response_error", Message: "VPN 找回密码响应不是 JSON 对象", Details: map[string]any{"api": path}}
	}
	return payload, nil
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
	fields := casLoginFields(form)
	salt := ""
	if field := form.first("input", "pwdEncryptSalt"); field != nil {
		salt = field.attr("value")
	}
	encrypted, encryptErr := encryptCASPassword(password, salt)
	if encryptErr != nil {
		return nil, encryptErr
	}
	fields = append(fields, pair{"username", account}, pair{"password", encrypted}, pair{"_eventId", "submit"}, pair{"cllt", "userNameLogin"}, pair{"dllt", "generalLogin"})
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
			return &siteError{Code: "invalid_argument", Message: "该命令不接受参数: " + arg}
		}
	}
	return nil
}

func splitInline(value string) (string, string, bool) {
	if strings.HasPrefix(value, "--") {
		if name, item, found := strings.Cut(value, "="); found {
			return name, item, true
		}
	}
	return value, "", false
}

func vpnAPIPath(base *url.URL, path string, native bool) (*url.URL, *siteError) {
	if strings.TrimSpace(path) == "" {
		return nil, &siteError{Code: "invalid_path", Message: "VPN API 路径不能为空"}
	}
	path = strings.TrimSpace(path)
	parsed, err := url.Parse(path)
	if err != nil || parsed.User != nil {
		return nil, &siteError{Code: "invalid_path", Message: "VPN API 路径格式无效"}
	}
	if parsed.IsAbs() || parsed.Host != "" {
		if !strings.EqualFold(parsed.Hostname(), base.Hostname()) {
			return nil, &siteError{Code: "invalid_path", Message: "VPN API 必须保持当前 VPN 主机"}
		}
	} else {
		requestPath := parsed.Path
		for i := 0; i < 2; i++ {
			requestPath, _ = url.PathUnescape(requestPath)
		}
		if strings.Contains(requestPath, "\\") {
			return nil, &siteError{Code: "invalid_path", Message: "VPN API 路径无效"}
		}
		for _, part := range strings.Split(requestPath, "/") {
			if part == "." || part == ".." {
				return nil, &siteError{Code: "invalid_path", Message: "VPN API 路径不能包含目录跳转"}
			}
		}
		if native {
			if strings.HasPrefix(parsed.Path, "/enclient/") {
				parsed.Path = strings.TrimPrefix(parsed.Path, "/enclient")
			}
			if !strings.HasPrefix(parsed.Path, "/api/") {
				parsed.Path = "/api/v1/" + strings.TrimPrefix(parsed.Path, "/")
			}
		} else if !strings.HasPrefix(parsed.Path, "/enclient/") {
			parsed.Path = "/enclient/" + strings.TrimPrefix(parsed.Path, "/")
		}
		parsed.Scheme, parsed.Host = base.Scheme, base.Host
	}
	return parsed, nil
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
	submitted, _ := remote["submitted"].(bool)
	return map[string]any{"ok": true, "submitted": submitted, "confirmed": true, "evidence": "confirmed", "logged_out": true, "native": native, "remote": remote}, nil
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
