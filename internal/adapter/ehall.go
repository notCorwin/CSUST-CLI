package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const ehallHomePath = "/index.html"

func (a NativeSite) executeEhall(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 {
		return a.ehallServices(ctx, "")
	}
	cookie, _, valueErr := businessValue(args, "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	if args[0] == "services" || args[0] == "catalog" || strings.HasPrefix(args[0], "--") {
		return a.ehallServices(ctx, cookie)
	}
	if args[0] == "service" || args[0] == "detail" {
		id, requiredErr := businessRequired(args[1:], "--id", "ehall service 必须提供 --id")
		if requiredErr != nil {
			return nil, requiredErr
		}
		return a.ehallService(ctx, id, cookie)
	}
	if args[0] == "health" {
		id, requiredErr := businessRequired(args[1:], "--id", "ehall health 必须提供 --id")
		if requiredErr != nil {
			return nil, requiredErr
		}
		return a.ehallHealth(ctx, id, cookie)
	}
	if args[0] == "me" || args[0] == "identity" {
		return a.ehallMe(ctx, cookie)
	}
	return nil, &siteError{Code: "invalid_argument", Message: "ehall 只支持 services、service、detail、health、me、catalog"}
}

func (a NativeSite) ehallServices(ctx context.Context, cookie string) (map[string]any, *siteError) {
	base, _, resolveErr := resolveSite(siteRequest{Service: "ehall", CookieFile: cookie})
	if resolveErr != nil {
		return nil, resolveErr
	}
	originalURL := (&url.URL{Scheme: base.Scheme, Host: base.Host, Path: ehallHomePath, Fragment: "/"}).String()
	options := ehallRequestOptions(cookie)
	pageResult, requestErr := a.businessGet(ctx, "ehall", "/getPageView", []pair{
		{name: "pageCode", value: ""},
		{name: "originalUrl", value: originalURL},
		{name: "lang", value: "zh_CN"},
	}, options)
	if requestErr != nil {
		return nil, requestErr
	}
	pageData, dataErr := ehallEnvelopeData(pageResult)
	if dataErr != nil {
		return nil, dataErr
	}
	pageContext, ok := pageData["pageContext"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "eHall 页面响应缺少 pageContext"}
	}
	pageInfo, ok := pageContext["pageInfoEntity"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "eHall 页面响应缺少 pageInfoEntity"}
	}
	layout, layoutErr := ehallCardLayout(pageInfo["cardLayout"])
	if layoutErr != nil {
		return nil, layoutErr
	}
	card := findEhallCard(layout, "SYS_CARD_SERVICEBUS")
	if card == nil {
		return nil, &siteError{Code: "parse_error", Message: "eHall 页面未找到服务直通车卡片"}
	}
	cardID, _ := card["cardId"].(string)
	cardWid, _ := card["cardWid"].(string)
	if cardID == "" || cardWid == "" {
		return nil, &siteError{Code: "parse_error", Message: "eHall 服务直通车卡片缺少标识"}
	}
	renderPath := "/execCardMethod/" + url.PathEscape(cardWid) + "/" + url.PathEscape(cardID)
	renderResult, requestErr := a.businessPostJSON(ctx, "ehall", renderPath, map[string]any{
		"cardId":  cardID,
		"cardWid": cardWid,
		"method":  "renderData",
		"param":   map[string]any{"fromMaster": true},
	}, options)
	if requestErr != nil {
		return nil, requestErr
	}
	renderData, dataErr := ehallEnvelopeData(renderResult)
	if dataErr != nil {
		return nil, dataErr
	}
	services, ok := renderData["appData"].([]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "eHall 服务直通车响应缺少 appData"}
	}
	categories, _ := renderData["classifyData"].([]any)
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed",
		"service": "ehall", "operation": "services", "scope": "available-to-current-user",
		"source": base.Scheme + "://" + base.Host,
		"api":    map[string]any{"page_view": "/getPageView", "render_card": renderPath},
		"card":   map[string]any{"card_id": cardID, "card_wid": cardWid},
		"portal": map[string]any{
			"site_wid": pageData["siteWid"], "portal_name": pageData["portalName"],
			"page_title": pageData["pageTitle"], "portal_domain": pageData["portalDomain"],
		},
		"services": services, "service_count": len(services),
		"categories": categories, "category_count": len(categories),
		"data": renderData,
	}, nil
}

func (a NativeSite) ehallService(ctx context.Context, id, cookie string) (map[string]any, *siteError) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "ehall service --id 不能为空"}
	}
	options := ehallRequestOptions(cookie)
	infoResult, requestErr := a.businessGet(ctx, "ehall", "/queryServiceByWid/"+url.PathEscape(id)+"/0", nil, options)
	if requestErr != nil {
		return nil, requestErr
	}
	infoData, dataErr := ehallEnvelopeData(infoResult)
	if dataErr != nil {
		return nil, dataErr
	}
	accessResult, requestErr := a.businessGet(ctx, "ehall", "/serviceShow", []pair{
		{name: "isMobile", value: "0"}, {name: "serviceId", value: id},
	}, options)
	if requestErr != nil {
		return nil, requestErr
	}
	accessData, dataErr := ehallEnvelopeData(accessResult)
	if dataErr != nil {
		return nil, dataErr
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed",
		"service": "ehall", "operation": "service", "service_id": id,
		"service_info": infoData["serviceInfo"], "access": accessData,
		"login": infoData["login"], "is_login": infoData["isLogin"], "local_lang": infoData["localLang"],
	}, nil
}

func (a NativeSite) ehallHealth(ctx context.Context, id, cookie string) (map[string]any, *siteError) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "ehall health --id 不能为空"}
	}
	result, requestErr := a.businessGet(ctx, "ehall", "/service/getHealthInfo", []pair{{name: "serviceWid", value: id}}, ehallRequestOptions(cookie))
	if requestErr != nil {
		return nil, requestErr
	}
	data, dataErr := ehallEnvelopeData(result)
	if dataErr != nil {
		return nil, dataErr
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed",
		"service": "ehall", "operation": "health", "service_id": id,
		"health": data,
	}, nil
}

func (a NativeSite) ehallMe(ctx context.Context, cookie string) (map[string]any, *siteError) {
	result, requestErr := a.businessGet(ctx, "ehall", "/getLoginUserAndGuest", nil, ehallRequestOptions(cookie))
	if requestErr != nil {
		return nil, requestErr
	}
	data, dataErr := ehallEnvelopeData(result)
	if dataErr != nil {
		return nil, dataErr
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed",
		"service": "ehall", "operation": "me", "logged_in": true,
		"user": map[string]any{
			"id": data["wid"], "account": data["userAccount"], "name": data["userName"],
			"category": data["categoryName"], "category_id": data["categoryWid"],
			"department": data["deptName"], "department_id": data["deptWid"],
			"groups": data["groups"], "organizations": data["orgs"],
			"preferred_language": data["preferredLanguage"], "portal_language": data["portalDefaultLang"],
		},
		"data": data,
	}, nil
}

func ehallRequestOptions(cookie string) businessRequestOptions {
	return businessRequestOptions{
		cookieFile: cookie, allowSSO: true, require: true,
		headers: []pair{{name: "X-Requested-With", value: "XMLHttpRequest"}, {name: "localeLang", value: "zh_CN"}},
	}
}

func ehallEnvelopeData(result map[string]any) (map[string]any, *siteError) {
	value, ok := businessData(result)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "eHall 响应不是 JSON"}
	}
	envelope, ok := value.(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "eHall 响应结构无效"}
	}
	if codeValue, exists := envelope["errcode"]; exists && fmt.Sprint(codeValue) != "0" {
		message, _ := envelope["errmsg"].(string)
		if message == "" {
			message = "eHall 接口返回失败"
		}
		return nil, &siteError{Code: "business_rejected", Message: message, Details: map[string]any{"api_code": envelope["errcode"]}}
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "eHall 响应缺少 data 对象"}
	}
	return data, nil
}

func ehallCardLayout(value any) (any, *siteError) {
	if layout, ok := value.(string); ok {
		var decoded any
		if err := json.Unmarshal([]byte(layout), &decoded); err != nil {
			return nil, &siteError{Code: "parse_error", Message: "eHall 卡片布局不是有效 JSON: " + err.Error()}
		}
		return decoded, nil
	}
	if value == nil {
		return nil, &siteError{Code: "parse_error", Message: "eHall 页面缺少卡片布局"}
	}
	return value, nil
}

func findEhallCard(value any, cardID string) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		if typed["cardId"] == cardID {
			return typed
		}
		for _, nested := range typed {
			if card := findEhallCard(nested, cardID); card != nil {
				return card
			}
		}
	case []any:
		for _, nested := range typed {
			if card := findEhallCard(nested, cardID); card != nil {
				return card
			}
		}
	}
	return nil
}
