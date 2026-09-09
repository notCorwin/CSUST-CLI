package adapter

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func parseQualityLoginOptions(args []string) (loginOptions, string, *siteError) {
	options := loginOptions{}
	vpnCaptchaInfo := ""
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		if arg == "--json" {
			continue
		}
		if !inline {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return loginOptions{}, "", &siteError{Code: "invalid_argument", Message: arg + " 缺少参数值"}
			}
			index++
			value = args[index]
		}
		switch arg {
		case "--username":
			options.username = value
		case "--captcha":
			options.captcha = value
		case "--captcha-image":
			options.captchaImage = value
		case "--vpn-captcha-info":
			vpnCaptchaInfo = value
		default:
			return loginOptions{}, "", &siteError{Code: "invalid_argument", Message: "quality login 参数无效: " + arg}
		}
	}
	return options, vpnCaptchaInfo, nil
}

func (a NativeSite) runQualityLogin(ctx context.Context, args []string) (map[string]any, *siteError) {
	options, vpnCaptchaInfo, parseErr := parseQualityLoginOptions(args)
	if parseErr != nil {
		return nil, parseErr
	}
	_, _, _, _, discoverErr := a.discoverGateway(ctx, qualityServiceName)
	if discoverErr != nil && discoverErr.Code == "login_required" {
		loginArgs := []string{"--auth", "cas"}
		if options.username != "" {
			loginArgs = append(loginArgs, "--username", options.username)
		}
		if vpnCaptchaInfo != "" {
			loginArgs = append(loginArgs, "--captcha-info", vpnCaptchaInfo)
		}
		if _, loginErr := a.runVPNLogin(ctx, loginArgs); loginErr != nil {
			return nil, loginErr
		}
		_, _, _, _, discoverErr = a.discoverGateway(ctx, qualityServiceName)
	}
	if discoverErr != nil {
		return nil, discoverErr
	}
	if _, landingErr := a.executeGateway(ctx, qualityServiceName, gatewayRequest{path: "/", method: "GET"}, false); landingErr != nil {
		return nil, landingErr
	}
	seedRequest := gatewayRequest{path: "/Logon.do?method=logon&flag=sess", method: "POST", raw: true, readOnly: true, yes: true}
	seedResult, seedErr := a.executeGateway(ctx, qualityServiceName, seedRequest, false)
	if seedErr != nil {
		return nil, seedErr
	}
	seed, _ := seedResult["raw_body"].(string)
	tempDir, tempErr := os.MkdirTemp("", "csust-quality-captcha-")
	if tempErr != nil {
		return nil, &siteError{Code: "captcha_write_failed", Message: "无法创建验证码临时目录: " + tempErr.Error()}
	}
	defer os.RemoveAll(tempDir)
	captchaPath := filepath.Join(tempDir, "captcha.bin")
	if options.captchaImage != "" {
		captchaPath = expandUserPath(options.captchaImage)
	}
	captchaRequest := gatewayRequest{path: "/verifycode.servlet", method: "GET", output: captchaPath}
	if _, captchaErr := a.executeGateway(ctx, qualityServiceName, captchaRequest, false); captchaErr != nil {
		return nil, captchaErr
	}
	if strings.TrimSpace(options.captcha) == "" {
		details := map[string]any{}
		if options.captchaImage != "" {
			details["captcha_image"] = captchaPath
		}
		return nil, &siteError{Code: "captcha_required", Message: "教学质量保障系统需要验证码，请提供 --captcha", Details: details}
	}
	account, password, credentialErr := credentialsGo(options.username, "")
	if credentialErr != nil {
		return nil, credentialErr
	}
	encoded, encodedErr := generateEncodedGo(account, password, strings.TrimSpace(seed))
	if encodedErr != nil {
		return nil, encodedErr
	}
	loginResult, loginErr := a.executeGateway(ctx, qualityServiceName, gatewayRequest{path: "/Logon.do?method=logon", method: "POST", data: []pair{{"userAccount", ""}, {"userPassword", ""}, {"RANDOMCODE", options.captcha}, {"encoded", encoded}, {"_csrf", ""}}, raw: true, readOnly: true, yes: true}, false)
	if loginErr != nil {
		return nil, loginErr
	}
	if body, ok := loginResult["raw_body"].(string); ok {
		if failure := loginFailure(body); failure != nil {
			return nil, failure
		}
	}
	probe, probeErr := a.executeGateway(ctx, qualityServiceName, gatewayRequest{path: "/jsxsd/framework/xsMain.jsp", method: "GET"}, false)
	if probeErr != nil {
		return nil, &siteError{Code: "authentication_failed", Message: "教学质量保障系统未建立有效会话", Details: map[string]any{"cause": probeErr.Code}}
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed", "username": account, "auth": "vpn-cas+quality-portal", "service": qualityServiceName, "system": "教学质量保障系统", "probe": probe["response"]}, nil
}

func (a NativeSite) runQualityLogout(ctx context.Context, args []string) (map[string]any, *siteError) {
	if err := onlyJSONArgs(args); err != nil {
		return nil, err
	}
	_, cookie, session, connectionErr := vpnConnection(false)
	if connectionErr != nil {
		return nil, connectionErr
	}
	remote := map[string]any{"skipped": true}
	active := false
	if _, statErr := os.Stat(cookie); statErr == nil {
		active = true
	}
	if session["token"] != nil {
		active = true
	}
	var remoteErr *siteError
	if active {
		path := "/jsxsd/xk/LoginToXk?method=exit&tktime=" + strconv.FormatInt(time.Now().UnixMilli(), 10)
		remote, remoteErr = a.runGatewayRequest(ctx, qualityServiceName, gatewayRequest{path: path, method: "GET", yes: true, forceMutating: true})
	}
	removeErr := removeCookieFile(cookie)
	if remoteErr != nil {
		return nil, remoteErr
	}
	if removeErr != nil {
		return nil, &siteError{Code: "cookie_write_failed", Message: removeErr.Error()}
	}
	submitted, _ := remote["submitted"].(bool)
	return map[string]any{"ok": true, "submitted": submitted, "confirmed": true, "evidence": "confirmed", "logged_out": true, "service": qualityServiceName, "remote": remote}, nil
}
