package adapter

import (
	"context"
	"strings"
)

func (a NativeSite) runQualityPublic(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 {
		return nil, &siteError{Code: "invalid_argument", Message: "quality public 缺少子命令"}
	}
	switch args[0] {
	case "get":
		request, err := parseGatewayRequest(args[1:], true)
		if err != nil {
			return nil, err
		}
		if err := validateQualityPublicPath(request.path); err != nil {
			return nil, err
		}
		request.public = true
		return a.runGatewayRequest(ctx, qualityServiceName, request)
	case "post":
		request, err := parseGatewayRequest(args[1:], false)
		if err != nil {
			return nil, err
		}
		if err := validateQualityPublicPath(request.path); err != nil {
			return nil, err
		}
		if !request.yes {
			return nil, &siteError{Code: "confirmation_required", Message: "公开 POST 可能发送找回密码邮件，请加 --yes"}
		}
		request.method, request.public = "POST", true
		return a.runGatewayRequest(ctx, qualityServiceName, request)
	case "action":
		path, err := gatewayPathArg(args[1:])
		if err != nil {
			return nil, err
		}
		if err := validateQualityPublicPath(path); err != nil {
			return nil, err
		}
		return a.runGatewayPageAction(ctx, qualityServiceName, "action", args[1:], true)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "未知 quality public 子命令: " + args[0]}
	}
}

func gatewayPathArg(args []string) (string, *siteError) {
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		if arg != "--path" {
			continue
		}
		if inline {
			return value, nil
		}
		if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
			return "", &siteError{Code: "invalid_argument", Message: "--path 缺少参数值"}
		}
		return args[index+1], nil
	}
	return "", &siteError{Code: "invalid_argument", Message: "必须提供 --path"}
}

func validateQualityPublicPath(path string) *siteError {
	for _, item := range webPublicRoutes {
		if item["path"] == path {
			return nil
		}
	}
	return &siteError{Code: "invalid_path", Message: "公开页面路径不在允许目录中: " + path}
}
