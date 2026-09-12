package adapter

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type mailPageConfig struct {
	provider *url.URL
	fallback *url.URL
	domain   string
	resetURL string
}

var (
	mailEntryHostPattern    = regexp.MustCompile(`(?:var\s+)?entryHost\s*=\s*["']([^"']+)["']`)
	mailFallbackHostPattern = regexp.MustCompile(`(?:var\s+)?entrybjhost\s*=\s*["']([^"']+)["']`)
	mailDomainPattern       = regexp.MustCompile(`(?is)name=["']domain["'][^>]*>.*?<option[^>]*value=["']([^"']+)["']`)
	mailResetPattern        = regexp.MustCompile(`(?is)attr-ch=["']重置密码["'][^>]*href=["']([^"']+)["']`)
)

func (a NativeSite) executeMail(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "status" {
		statusArgs := args
		if len(statusArgs) > 0 {
			statusArgs = statusArgs[1:]
		}
		cookie, _, valueErr := businessValue(statusArgs, "--cookie-file")
		if valueErr != nil {
			return nil, valueErr
		}
		return a.mailStatus(ctx, cookie)
	}
	if args[0] == "catalog" {
		return businessCatalogFilter("mail"), nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "login":
		return a.mailLogin(ctx, args[1:], cookie)
	case "logout":
		return a.mailLogout(ctx, cookie)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "mail 只支持 status、catalog、login、logout"}
	}
}

func (a NativeSite) mailStatus(ctx context.Context, cookie string) (map[string]any, *siteError) {
	page, requestErr := a.businessGet(ctx, "mail", "/", nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	config, configErr := mailConfig(businessBody(page))
	if configErr != nil {
		return nil, configErr
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "live-mail-login-form-and-provider-config",
		"service": "mail", "operation": "status", "domain": config.domain,
		"login_url": safeResponseURL(page), "provider": config.provider.String(),
		"password_reset": config.resetURL,
		"api":            map[string]string{"prelogin": "/login/prelogin.jsp", "login": "/login/domainEntLogin", "captcha": "/login/getverifycode.jsp"},
	}, nil
}

func mailConfig(body string) (mailPageConfig, *siteError) {
	providerText := lastMailMatch(mailEntryHostPattern, body)
	if providerText == "" {
		return mailPageConfig{}, &siteError{Code: "parse_error", Message: "企业邮箱页面缺少登录节点配置"}
	}
	provider, parseErr := url.Parse(providerText)
	if parseErr != nil || provider.Host == "" || (provider.Scheme != "http" && provider.Scheme != "https") {
		return mailPageConfig{}, &siteError{Code: "parse_error", Message: "企业邮箱登录节点地址无效"}
	}
	fallback := provider
	if fallbackText := lastMailMatch(mailFallbackHostPattern, body); fallbackText != "" {
		if parsed, err := url.Parse(fallbackText); err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https") {
			fallback = parsed
		}
	}
	domain := "csust.edu.cn"
	if match := mailDomainPattern.FindStringSubmatch(body); len(match) == 2 && strings.TrimSpace(match[1]) != "" {
		domain = strings.TrimSpace(match[1])
	}
	resetURL := ""
	if match := mailResetPattern.FindStringSubmatch(body); len(match) == 2 {
		resetURL = strings.TrimSpace(match[1])
	}
	return mailPageConfig{provider: provider, fallback: fallback, domain: domain, resetURL: resetURL}, nil
}

func lastMailMatch(pattern *regexp.Regexp, body string) string {
	matches := pattern.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return ""
	}
	return strings.TrimSpace(matches[len(matches)-1][1])
}

func mailProviderTarget(base *url.URL, path string) *url.URL {
	target := *base
	target.Path, target.RawPath, target.RawQuery, target.Fragment = path, "", "", ""
	return &target
}

func (a NativeSite) mailProviderGet(ctx context.Context, base *url.URL, path string, params []pair, cookie string, output string) (map[string]any, *siteError) {
	return a.execute(ctx, siteRequest{
		Target: mailProviderTarget(base, path), Method: "GET", Params: params, CookieFile: cookie,
		Output: output, ReadOnly: true, Yes: true,
	})
}

func (a NativeSite) mailPrelogin(ctx context.Context, base *url.URL, account, domain, captcha, cookie string) (map[string]any, *siteError) {
	result, requestErr := a.mailProviderGet(ctx, base, "/login/prelogin.jsp", []pair{
		{"uid", account + "@" + domain}, {"code", captcha}, {"callback", "csust_cli_" + strconv.FormatInt(time.Now().UnixNano(), 10)},
	}, cookie, "")
	if requestErr != nil {
		return nil, requestErr
	}
	body := strings.TrimSpace(businessBody(result))
	start, end := strings.IndexByte(body, '('), strings.LastIndexByte(body, ')')
	if start < 1 || end <= start {
		return nil, &siteError{Code: "parse_error", Message: "企业邮箱预登录响应不是 JSONP"}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(body[start+1:end]), &payload); err != nil {
		return nil, &siteError{Code: "parse_error", Message: "企业邮箱预登录响应无效: " + err.Error()}
	}
	return payload, nil
}

func mailPayloadData(payload map[string]any) map[string]any {
	data, _ := payload["data"].(map[string]any)
	return data
}

func mailPayloadCode(payload map[string]any) string {
	return strings.TrimSpace(fmt.Sprint(payload["code"]))
}

func mailBool(value any) bool {
	if parsed, ok := value.(bool); ok {
		return parsed
	}
	return strings.EqualFold(strings.TrimSpace(fmt.Sprint(value)), "true") || fmt.Sprint(value) == "1"
}

func mailRSAPublicKey(data map[string]any) (*rsa.PublicKey, *siteError) {
	modulusText := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(fmt.Sprint(data["modulus"])), "0x"), "0X")
	exponentText := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(fmt.Sprint(data["exponent"])), "0x"), "0X")
	modulus, ok := new(big.Int).SetString(modulusText, 16)
	if !ok || modulus.Sign() <= 0 {
		return nil, &siteError{Code: "parse_error", Message: "企业邮箱预登录未返回有效 RSA 模数"}
	}
	exponent, ok := new(big.Int).SetString(exponentText, 16)
	if !ok || !exponent.IsInt64() || exponent.Int64() < 3 {
		return nil, &siteError{Code: "parse_error", Message: "企业邮箱预登录未返回有效 RSA 指数"}
	}
	return &rsa.PublicKey{N: modulus, E: int(exponent.Int64())}, nil
}

func (a NativeSite) mailCaptcha(ctx context.Context, provider *url.URL, cookie, output string) (string, *siteError) {
	if output == "" {
		_, cookiePath, resolveErr := resolveSite(siteRequest{Service: "mail", CookieFile: cookie})
		if resolveErr != nil {
			return "", resolveErr
		}
		output = filepath.Join(filepath.Dir(cookiePath), "mail-captcha.png")
	}
	output = expandUserPath(output)
	if _, requestErr := a.mailProviderGet(ctx, provider, "/login/getverifycode.jsp", []pair{
		{"all_secure", "1"}, {"rnd", strconv.FormatInt(time.Now().UnixMilli(), 10)},
	}, cookie, output); requestErr != nil {
		return "", requestErr
	}
	return output, nil
}

func mailAccount(value, domain string) (string, *siteError) {
	value = strings.TrimSpace(value)
	if at := strings.LastIndex(value, "@"); at > 0 {
		if !strings.EqualFold(strings.TrimSpace(value[at+1:]), domain) {
			return "", &siteError{Code: "invalid_argument", Message: "--username 的邮箱域名必须是 " + domain}
		}
		value = strings.TrimSpace(value[:at])
	}
	if value == "" {
		return "", &siteError{Code: "credentials_required", Message: "缺少企业邮箱账号"}
	}
	return value, nil
}

func (a NativeSite) mailLogin(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	_, cookiePath, resolveErr := resolveSite(siteRequest{Service: "mail", CookieFile: cookie})
	if resolveErr != nil {
		return nil, resolveErr
	}
	accountInput, password, credentialErr := businessCredentials(args, "CSUST_MAIL_PASSWORD")
	if credentialErr != nil {
		return nil, credentialErr
	}
	page, requestErr := a.businessGet(ctx, "mail", "/", nil, businessRequestOptions{cookieFile: cookiePath})
	if requestErr != nil {
		return nil, requestErr
	}
	config, configErr := mailConfig(businessBody(page))
	if configErr != nil {
		return nil, configErr
	}
	account, accountErr := mailAccount(accountInput, config.domain)
	if accountErr != nil {
		return nil, accountErr
	}
	captcha := strings.TrimSpace(flagValue(args, "--captcha"))
	provider := config.provider
	payload, preloginErr := a.mailPrelogin(ctx, provider, account, config.domain, captcha, cookiePath)
	if preloginErr != nil {
		return nil, preloginErr
	}
	if mailPayloadCode(payload) == "404" && config.fallback.Host != provider.Host {
		provider = config.fallback
		payload, preloginErr = a.mailPrelogin(ctx, provider, account, config.domain, captcha, cookiePath)
		if preloginErr != nil {
			return nil, preloginErr
		}
	}
	data := mailPayloadData(payload)
	if mailBool(data["locked"]) {
		return nil, &siteError{Code: "authentication_failed", Message: "企业邮箱账号已被锁定", Details: map[string]any{"submitted": false, "confirmed": false, "evidence": "prelogin-response"}}
	}
	if mailPayloadCode(payload) != "200" {
		return nil, &siteError{Code: "authentication_failed", Message: "企业邮箱未找到可用登录节点", Details: map[string]any{"submitted": false, "confirmed": false, "evidence": "prelogin-response"}}
	}
	if mailBool(data["verify_code"]) {
		if captcha == "" {
			imagePath, imageErr := a.mailCaptcha(ctx, provider, cookiePath, flagValue(args, "--captcha-image"))
			if imageErr != nil {
				return nil, imageErr
			}
			return nil, &siteError{Code: "captcha_required", Message: "企业邮箱登录需要验证码，请查看图片后提供 --captcha", Details: map[string]any{"captcha_image": imagePath, "submitted": false, "confirmed": false, "evidence": "prelogin-verify-code"}}
		}
		return nil, &siteError{Code: "captcha_failed", Message: "企业邮箱验证码错误", Details: map[string]any{"submitted": false, "confirmed": false, "evidence": "prelogin-verify-code"}}
	}
	key, keyErr := mailRSAPublicKey(data)
	if keyErr != nil {
		return nil, keyErr
	}
	encrypted, encryptErr := rsa.EncryptPKCS1v15(cryptorand.Reader, key, []byte(password+"#"+fmt.Sprint(data["rand"])))
	if encryptErr != nil {
		return nil, &siteError{Code: "authentication_failed", Message: "企业邮箱密码加密失败"}
	}
	result, requestErr := a.execute(ctx, siteRequest{
		Target: mailProviderTarget(provider, "/login/domainEntLogin"), Method: "POST", CookieFile: cookiePath,
		Data: []pair{{"ch", ""}, {"pubid", fmt.Sprint(data["pubid"])}, {"passtype", "3"}, {"support_verify_code", "1"},
			{"account_name", account}, {"domain", config.domain}, {"password", hex.EncodeToString(encrypted)}, {"verify_code", captcha}, {"secure", "1"}, {"all_secure", "1"}},
		Headers: []pair{{"Referer", safeResponseURL(page)}}, ReadOnly: true, Yes: true,
	})
	if requestErr != nil {
		return nil, requestErr
	}
	body := businessBody(result)
	if failure := businessLoginFailure(body); failure != nil {
		return nil, failure
	}
	if strings.TrimSpace(body) == "" || mailLoginPage(body) {
		return nil, &siteError{Code: "authentication_failed", Message: "企业邮箱登录失败，响应仍是登录页", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "domainEntLogin-response"}}
	}
	return map[string]any{
		"ok": true, "submitted": true, "confirmed": true, "evidence": "prelogin-rsa-and-domainEntLogin-response",
		"service": "mail", "operation": "login", "username": account, "domain": config.domain,
		"provider": provider.String(), "cookie_file": cookiePath,
	}, nil
}

func mailLoginPage(body string) bool {
	return strings.Contains(body, `id="loginform"`) && strings.Contains(body, "domainEntLogin")
}

func (a NativeSite) mailLogout(ctx context.Context, cookie string) (map[string]any, *siteError) {
	_, cookiePath, resolveErr := resolveSite(siteRequest{Service: "mail", CookieFile: cookie})
	if resolveErr != nil {
		return nil, resolveErr
	}
	if removeErr := removeCookieFile(cookiePath); removeErr != nil {
		return nil, &siteError{Code: "cookie_write_failed", Message: removeErr.Error()}
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "local-cookie-removed", "service": "mail", "operation": "logout", "logged_out": true, "cookie_file": cookiePath}, nil
}
