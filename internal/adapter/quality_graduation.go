package adapter

import (
	"context"
	"net/url"
	"strings"
)

func (a NativeSite) runQualityGraduation(ctx context.Context, args []string) (map[string]any, *siteError) {
	fetch, output := false, ""
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		switch arg {
		case "--json":
			continue
		case "--fetch":
			if inline {
				return nil, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			fetch = true
		case "--output":
			if !inline {
				if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
					return nil, &siteError{Code: "invalid_argument", Message: "--output 缺少参数值"}
				}
				index++
				value = args[index]
			}
			output = expandUserPath(value)
		default:
			return nil, &siteError{Code: "invalid_argument", Message: "quality graduation-design 参数无效: " + arg}
		}
	}
	if output != "" && !fetch {
		return nil, &siteError{Code: "invalid_argument", Message: "--output 需要同时指定 --fetch"}
	}
	landing, err := a.runGatewayRequest(ctx, qualityServiceName, gatewayRequest{path: "/jsxsd/framework/xsMain.jsp", method: "GET", raw: true})
	if err != nil {
		return nil, err
	}
	source, _ := landing["raw_body"].(string)
	pageURL, _ := landing["raw_url"].(string)
	document, parseErr := parsePage(source)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	target := ""
	for _, node := range document.findAll("a") {
		if !strings.Contains(strings.ToLower(node.attr("onclick")), "towptjbs") {
			continue
		}
		_, target = pageActionTarget(node, nil, document, pageURL)
		break
	}
	parsed, targetErr := validateGraduationTarget(target)
	if targetErr != nil {
		return nil, targetErr
	}
	result := map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed", "external": true, "url": safeSiteURL(parsed), "method": "GET"}
	if !fetch {
		return result, nil
	}
	response, requestErr := a.execute(ctx, siteRequest{Target: parsed, CookieFile: "", Method: "GET", Output: output, ReadOnly: true, Yes: true})
	if requestErr != nil {
		return nil, requestErr
	}
	if output != "" {
		result["download"] = response
		return result, nil
	}
	result["response"] = sitePageResult(response)["response"]
	return result, nil
}

func validateGraduationTarget(value string) (*url.URL, *siteError) {
	target, err := url.Parse(value)
	if err != nil || target.User != nil || !strings.EqualFold(target.Scheme, "https") || !strings.EqualFold(target.Host, "oauth.fanyu.com") || target.Path != "/sso/cas/10536/1004" {
		return nil, &siteError{Code: "invalid_path", Message: "毕业设计入口目标未通过安全校验"}
	}
	return target, nil
}
