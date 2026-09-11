package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
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
	if args[0] == "favorites" {
		return a.ehallFavorites(ctx, cookie)
	}
	if args[0] == "service-item-favorites" || args[0] == "item-favorites" {
		return a.ehallServiceItemFavorites(ctx, cookie)
	}
	if args[0] == "service-item-favorite" {
		if len(args) == 1 {
			return a.ehallServiceItemFavorites(ctx, cookie)
		}
		if args[1] == "add" || args[1] == "remove" {
			return a.ehallServiceItemFavoriteMutation(ctx, args[1], args[2:], cookie)
		}
	}
	if args[0] == "message-count" {
		return a.ehallMessageCount(ctx, cookie)
	}
	if args[0] == "notifications" {
		return a.ehallNotifications(ctx, cookie)
	}
	if args[0] == "service-cycles" {
		return a.ehallServiceCycles(ctx, cookie)
	}
	if args[0] == "mail-status" || args[0] == "mail" {
		return a.ehallMailStatus(ctx, cookie)
	}
	if args[0] == "news" {
		return a.ehallNews(ctx, args[1:], cookie)
	}
	if args[0] == "rating" || args[0] == "service-rating" {
		return a.ehallServiceRating(ctx, args[1:], cookie)
	}
	if args[0] == "favorite" {
		if len(args) == 1 {
			return a.ehallFavorites(ctx, cookie)
		}
		if args[1] == "add" || args[1] == "remove" {
			return a.ehallFavoriteMutation(ctx, args[1], args[2:], cookie)
		}
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
	return nil, &siteError{Code: "invalid_argument", Message: "ehall 只支持 services、favorites、service-item-favorites、service-item-favorite add/remove、message-count、notifications、service-cycles、mail-status、news、rating、favorite add/remove、service、detail、health、me、catalog"}
}

func (a NativeSite) ehallServices(ctx context.Context, cookie string) (map[string]any, *siteError) {
	base, pageData, layout, pageErr := a.ehallPageView(ctx, cookie)
	if pageErr != nil {
		return nil, pageErr
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
	renderData, dataErr := a.ehallCardMethod(ctx, cardID, cardWid, "renderData", map[string]any{"fromMaster": true}, cookie)
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
		"api":    map[string]any{"page_view": "/getPageView", "render_card": ehallCardMethodPath(cardID, cardWid)},
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

func (a NativeSite) ehallPageView(ctx context.Context, cookie string) (*url.URL, map[string]any, any, *siteError) {
	base, _, resolveErr := resolveSite(siteRequest{Service: "ehall", CookieFile: cookie})
	if resolveErr != nil {
		return nil, nil, nil, resolveErr
	}
	originalURL := (&url.URL{Scheme: base.Scheme, Host: base.Host, Path: ehallHomePath, Fragment: "/"}).String()
	options := ehallRequestOptions(cookie)
	options.headers = append(options.headers, pair{name: "Referer", value: originalURL})
	pageResult, requestErr := a.businessGet(ctx, "ehall", "/getPageView", []pair{
		{name: "pageCode", value: ""},
		{name: "originalUrl", value: originalURL},
		{name: "lang", value: "zh_CN"},
	}, options)
	if requestErr != nil {
		return nil, nil, nil, requestErr
	}
	pageData, dataErr := ehallEnvelopeData(pageResult)
	if dataErr != nil {
		return nil, nil, nil, dataErr
	}
	pageContext, ok := pageData["pageContext"].(map[string]any)
	if !ok {
		return nil, nil, nil, &siteError{Code: "parse_error", Message: "eHall 页面响应缺少 pageContext"}
	}
	pageInfo, ok := pageContext["pageInfoEntity"].(map[string]any)
	if !ok {
		return nil, nil, nil, &siteError{Code: "parse_error", Message: "eHall 页面响应缺少 pageInfoEntity"}
	}
	layout, layoutErr := ehallCardLayout(pageInfo["cardLayout"])
	if layoutErr != nil {
		return nil, nil, nil, layoutErr
	}
	return base, pageData, layout, nil
}

func (a NativeSite) ehallCardMethod(ctx context.Context, cardID, cardWid, method string, param map[string]any, cookie string) (map[string]any, *siteError) {
	result, requestErr := a.businessPostJSON(ctx, "ehall", ehallCardMethodPath(cardID, cardWid), map[string]any{
		"cardId": cardID, "cardWid": cardWid, "method": method, "param": param,
	}, ehallRequestOptions(cookie))
	if requestErr != nil {
		return nil, requestErr
	}
	return ehallEnvelopeData(result)
}

func ehallCardMethodPath(cardID, cardWid string) string {
	return "/execCardMethod/" + url.PathEscape(cardWid) + "/" + url.PathEscape(cardID)
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

func (a NativeSite) ehallFavorites(ctx context.Context, cookie string) (map[string]any, *siteError) {
	result, requestErr := a.businessPostJSON(ctx, "ehall", "/queryFolderAndService", map[string]any{}, ehallRequestOptions(cookie))
	if requestErr != nil {
		return nil, requestErr
	}
	value, dataErr := ehallEnvelopeValue(result)
	if dataErr != nil {
		return nil, dataErr
	}
	folders, ok := value.([]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "eHall 收藏夹响应不是数组"}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed",
		"service": "ehall", "operation": "favorites", "scope": "current-user",
		"folders": folders, "folder_count": len(folders),
	}, nil
}

func (a NativeSite) ehallServiceItemFavorites(ctx context.Context, cookie string) (map[string]any, *siteError) {
	result, requestErr := a.businessPostJSON(ctx, "ehall", "/queryFolderAndItem", map[string]any{}, ehallRequestOptions(cookie))
	if requestErr != nil {
		return nil, requestErr
	}
	value, dataErr := ehallEnvelopeValue(result)
	if dataErr != nil {
		return nil, dataErr
	}
	folders, ok := value.([]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "eHall 服务项收藏夹响应不是数组"}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed",
		"service": "ehall", "operation": "service-item-favorites", "scope": "current-user",
		"folders": folders, "folder_count": len(folders),
	}, nil
}

func (a NativeSite) ehallServiceItemFavoriteMutation(ctx context.Context, operation string, args []string, cookie string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "修改 eHall 服务项收藏状态必须加 --yes"}
	}
	id, requiredErr := businessRequired(args, "--item-id", "ehall service-item-favorite 必须提供 --item-id")
	if requiredErr != nil {
		return nil, requiredErr
	}
	operate, wantFavorite := "0", false
	if operation == "add" {
		operate, wantFavorite = "1", true
	}
	params := []pair{{name: "id", value: id}, {name: "operate", value: operate}}
	if folder, found, valueErr := businessValue(args, "--folder-id"); valueErr != nil {
		return nil, valueErr
	} else if found {
		if strings.TrimSpace(folder) == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "--folder-id 不能为空"}
		}
		params = append(params, pair{name: "folderWid", value: folder})
	}
	result, requestErr := a.execute(ctx, siteRequest{
		Service: "ehall", Method: "GET", Path: "/collectServiceItem", Params: params,
		Headers: ehallRequestOptions(cookie).headers, CookieFile: cookie, AllowSSO: true,
		RequireLogin: true, AllowBusinessFailure: true, ReadOnly: false, Yes: true, RawJSON: true,
	})
	if requestErr != nil {
		return nil, requestErr
	}
	value, dataErr := ehallEnvelopeValue(result)
	if dataErr != nil {
		if dataErr.Code == "business_rejected" {
			dataErr.Code = "mutation_rejected"
			dataErr.Details = map[string]any{"submitted": true, "confirmed": false, "evidence": "response", "item_id": id}
		}
		return nil, dataErr
	}
	return map[string]any{
		"ok": true, "submitted": true, "confirmed": true, "evidence": "response",
		"service": "ehall", "operation": "service-item-favorite-" + operation, "item_id": id,
		"favorite": wantFavorite, "api_data": value, "api": "/collectServiceItem",
	}, nil
}

func (a NativeSite) ehallMessageCount(ctx context.Context, cookie string) (map[string]any, *siteError) {
	result, requestErr := a.businessGet(ctx, "ehall", "/getMessageCount", nil, ehallRequestOptions(cookie))
	if requestErr != nil {
		return nil, requestErr
	}
	value, dataErr := ehallEnvelopeValue(result)
	if dataErr != nil {
		return nil, dataErr
	}
	count, parseErr := strconv.Atoi(strings.TrimSpace(fmt.Sprint(value)))
	if parseErr != nil || count < 0 {
		return nil, &siteError{Code: "parse_error", Message: "eHall 消息数量不是非负整数"}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed",
		"service": "ehall", "operation": "message-count", "message_count": count, "data": value,
	}, nil
}

func (a NativeSite) ehallNotifications(ctx context.Context, cookie string) (map[string]any, *siteError) {
	result, requestErr := a.businessGet(ctx, "ehall", "/userNotify/getNewsNotify", nil, ehallRequestOptions(cookie))
	if requestErr != nil {
		return nil, requestErr
	}
	value, dataErr := ehallEnvelopeValue(result)
	if dataErr != nil {
		return nil, dataErr
	}
	notifications, ok := value.([]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "eHall 通知响应不是数组"}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed",
		"service": "ehall", "operation": "notifications", "notifications": notifications,
		"notification_count": len(notifications), "data": value,
	}, nil
}

func (a NativeSite) ehallServiceCycles(ctx context.Context, cookie string) (map[string]any, *siteError) {
	result, requestErr := a.businessGet(ctx, "ehall", "/userNotify/getRecommendCycle", nil, ehallRequestOptions(cookie))
	if requestErr != nil {
		return nil, requestErr
	}
	value, dataErr := ehallEnvelopeValue(result)
	if dataErr != nil {
		return nil, dataErr
	}
	cycles, ok := value.([]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "eHall 服务周期响应不是数组"}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed",
		"service": "ehall", "operation": "service-cycles", "cycles": cycles,
		"cycle_count": len(cycles), "data": value,
	}, nil
}

func (a NativeSite) ehallMailStatus(ctx context.Context, cookie string) (map[string]any, *siteError) {
	_, _, layout, pageViewErr := a.ehallPageView(ctx, cookie)
	if pageViewErr != nil {
		return nil, pageViewErr
	}
	card := findEhallCard(layout, "CUS_CARD_TENCENTMAIL")
	if card == nil {
		return nil, &siteError{Code: "not_found", Message: "eHall 页面未找到企业邮箱卡片"}
	}
	cardID, _ := card["cardId"].(string)
	cardWid, _ := card["cardWid"].(string)
	if cardID == "" || cardWid == "" {
		return nil, &siteError{Code: "parse_error", Message: "eHall 企业邮箱卡片缺少标识"}
	}
	registration, registrationErr := a.ehallCardMethod(ctx, cardID, cardWid, "ifRegister", map[string]any{}, cookie)
	if registrationErr != nil {
		return nil, registrationErr
	}
	registrationCode, ok := registration["errcode"]
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "eHall 企业邮箱注册响应缺少状态码"}
	}
	registered := fmt.Sprint(registrationCode) == "0"
	var unread any
	var unreadData map[string]any
	if registered {
		unreadData, registrationErr = a.ehallCardMethod(ctx, cardID, cardWid, "unReadMail", map[string]any{}, cookie)
		if registrationErr != nil {
			return nil, registrationErr
		}
		if code, exists := unreadData["errcode"]; !exists || fmt.Sprint(code) != "0" {
			return nil, &siteError{Code: "business_rejected", Message: "eHall 企业邮箱未读数量查询失败", Details: map[string]any{"api_code": unreadData["errcode"]}}
		}
		count, parseErr := strconv.Atoi(strings.TrimSpace(fmt.Sprint(unreadData["count"])))
		if parseErr != nil || count < 0 {
			return nil, &siteError{Code: "parse_error", Message: "eHall 企业邮箱未读数量不是非负整数"}
		}
		unread = count
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed",
		"service": "ehall", "operation": "mail-status", "registered": registered,
		"unread_count": unread, "card": map[string]any{"card_id": cardID, "card_wid": cardWid},
		"entrypoints": map[string]string{
			"mail":           "http://txyj.csust.edu.cn/Mail/LoginUrl",
			"password_reset": "http://txyj.csust.edu.cn/Mail/ChangePass",
			"register":       "http://txyj.csust.edu.cn/Mail/Enroll",
		},
		"registration": registration, "unread": unreadData,
		"api": map[string]any{"page_view": "/getPageView", "card": ehallCardMethodPath(cardID, cardWid), "registration": "ifRegister", "unread": "unReadMail"},
	}, nil
}

func (a NativeSite) ehallNews(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	channelFilter, found, valueErr := businessValue(args, "--channel")
	if valueErr != nil {
		return nil, valueErr
	}
	channelFilter = strings.TrimSpace(channelFilter)
	if found && channelFilter == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "--channel 不能为空"}
	}
	_, _, layout, pageViewErr := a.ehallPageView(ctx, cookie)
	if pageViewErr != nil {
		return nil, pageViewErr
	}
	cards := findEhallCards(layout, "SYS_CARD_NEWSANNOUNCEMENT")
	if len(cards) == 0 {
		return nil, &siteError{Code: "parse_error", Message: "eHall 页面未找到新闻通知公告卡片"}
	}
	resultCards := make([]map[string]any, 0, len(cards))
	itemCount := 0
	choices := []string{}
	for _, card := range cards {
		cardID, _ := card["cardId"].(string)
		cardWid, _ := card["cardWid"].(string)
		if cardID == "" || cardWid == "" {
			return nil, &siteError{Code: "parse_error", Message: "eHall 新闻卡片缺少标识"}
		}
		config, configErr := a.ehallCardMethod(ctx, cardID, cardWid, "getNewsConfig", map[string]any{}, cookie)
		if configErr != nil {
			return nil, configErr
		}
		channelData, channelErr := a.ehallCardMethod(ctx, cardID, cardWid, "getConfiguredAndSubscribedChannel", map[string]any{}, cookie)
		if channelErr != nil {
			return nil, channelErr
		}
		configured, _ := channelData["configuredChannel"].([]any)
		subscribed, _ := channelData["subscribedChannel"].([]any)
		selected := make([]any, 0, len(subscribed))
		channelIDs, programIDs := make([]string, 0), make([]string, 0)
		for _, raw := range subscribed {
			entry, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			name := strings.TrimSpace(fmt.Sprint(entry["name"]))
			if name != "" && !containsFold(choices, name) {
				choices = append(choices, name)
			}
			if channelFilter != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(channelFilter)) {
				continue
			}
			selected = append(selected, entry)
			id := strings.TrimSpace(fmt.Sprint(entry["wid"]))
			if id == "" {
				continue
			}
			if fmt.Sprint(entry["type"]) == "1" {
				programIDs = append(programIDs, id)
			} else {
				channelIDs = append(channelIDs, id)
			}
		}
		if channelFilter != "" && len(selected) == 0 {
			continue
		}
		newsData, newsErr := a.ehallCardMethod(ctx, cardID, cardWid, "getChannelNews", map[string]any{
			"channelIds": strings.Join(channelIDs, ","), "programIds": strings.Join(programIDs, ","), "pageNumber": page,
		}, cookie)
		if newsErr != nil {
			return nil, newsErr
		}
		datas, ok := newsData["datas"].(map[string]any)
		if !ok {
			return nil, &siteError{Code: "parse_error", Message: "eHall 新闻响应缺少 datas"}
		}
		items, ok := datas["data"].([]any)
		if !ok {
			if datas["data"] == nil {
				items = []any{}
			} else {
				return nil, &siteError{Code: "parse_error", Message: "eHall 新闻响应缺少 data 数组"}
			}
		}
		itemCount += len(items)
		resultCards = append(resultCards, map[string]any{
			"card_id": cardID, "card_wid": cardWid, "title": ehallCardTitle(card),
			"config": config, "configured_channels": configured, "channels": subscribed, "selected_channels": selected,
			"subscription_data": channelData,
			"items":             items, "item_count": len(items), "total_count": datas["totalSize"],
			"page_size": datas["pageSize"], "page": datas["pageNumber"], "data": newsData,
		})
	}
	if channelFilter != "" && len(resultCards) == 0 {
		return nil, &siteError{Code: "not_found", Message: "没有匹配的 eHall 新闻订阅栏目", Details: map[string]any{"channel": channelFilter, "choices": choices}}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed",
		"service": "ehall", "operation": "news", "page": page, "channel": nullableString(channelFilter),
		"cards": resultCards, "card_count": len(resultCards), "item_count": itemCount,
		"api": map[string]any{"page_view": "/getPageView", "card_config": "getNewsConfig", "subscriptions": "getConfiguredAndSubscribedChannel", "news": "getChannelNews"},
	}, nil
}

func (a NativeSite) ehallServiceRating(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	id, requiredErr := businessRequired(args, "--id", "ehall rating 必须提供 --id")
	if requiredErr != nil {
		return nil, requiredErr
	}
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	pageSize, pageSizeErr := businessInt(args, "--page-size", 10)
	if pageSizeErr != nil {
		return nil, pageSizeErr
	}
	options := ehallRequestOptions(cookie)
	modeResult, requestErr := a.businessGet(ctx, "ehall", "/appAppraise/getServiceAppraiseMode", nil, options)
	if requestErr != nil {
		return nil, requestErr
	}
	mode, modeErr := ehallEnvelopeValue(modeResult)
	if modeErr != nil {
		return nil, modeErr
	}
	modeName := strings.TrimSpace(fmt.Sprint(mode))
	if modeName == "" || modeName == "<nil>" {
		return nil, &siteError{Code: "parse_error", Message: "eHall 服务评价响应缺少评价模式"}
	}
	summaryResult, requestErr := a.businessGet(ctx, "ehall", "/appAppraise/appraiseSummary", []pair{{name: "appId", value: id}}, options)
	if requestErr != nil {
		return nil, requestErr
	}
	summary, summaryErr := ehallEnvelopeData(summaryResult)
	if summaryErr != nil {
		return nil, summaryErr
	}
	phraseResult, requestErr := a.businessGet(ctx, "ehall", "/appAppraise/getUserCommentPhraseList", []pair{{name: "serviceWid", value: id}}, options)
	if requestErr != nil {
		return nil, requestErr
	}
	phrases, phraseErr := ehallEnvelopeData(phraseResult)
	if phraseErr != nil {
		return nil, phraseErr
	}
	reviewParams := []pair{
		{name: "appId", value: id}, {name: "pageSize", value: strconv.Itoa(pageSize)},
		{name: "pageNum", value: strconv.Itoa(page)}, {name: "scoreLevel", value: "0"},
	}
	if modeName == "2" {
		reviewParams = append(reviewParams, pair{name: "commentPhrase", value: ""})
	} else {
		reviewParams = append(reviewParams, pair{name: "appraiseType", value: ""})
	}
	reviewResult, requestErr := a.businessGet(ctx, "ehall", "/appAppraise/queryAppraiseByPageNew", reviewParams, options)
	if requestErr != nil {
		return nil, requestErr
	}
	reviews, reviewsErr := ehallEnvelopeData(reviewResult)
	if reviewsErr != nil {
		return nil, reviewsErr
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed",
		"service": "ehall", "operation": "service-rating", "service_id": id,
		"mode": mode, "summary": summary, "comment_phrases": phrases, "reviews": reviews,
		"page": page, "page_size": pageSize,
		"api": map[string]any{
			"mode": "/appAppraise/getServiceAppraiseMode", "summary": "/appAppraise/appraiseSummary",
			"phrases": "/appAppraise/getUserCommentPhraseList", "reviews": "/appAppraise/queryAppraiseByPageNew",
		},
	}, nil
}

func ehallCardTitle(card map[string]any) string {
	if title, ok := card["layoutCardTitle"].(map[string]any); ok {
		if value, ok := title["cardTitle"].(string); ok {
			return value
		}
	}
	if value, ok := card["cardName"].(string); ok {
		return value
	}
	return ""
}

func containsFold(values []string, wanted string) bool {
	for _, value := range values {
		if strings.EqualFold(value, wanted) {
			return true
		}
	}
	return false
}

func (a NativeSite) ehallFavoriteMutation(ctx context.Context, operation string, args []string, cookie string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "修改 eHall 收藏状态必须加 --yes"}
	}
	id, requiredErr := businessRequired(args, "--service-id", "ehall favorite 必须提供 --service-id")
	if requiredErr != nil {
		return nil, requiredErr
	}
	operate := "0"
	wantFavorite := false
	if operation == "add" {
		operate, wantFavorite = "1", true
	}
	params := []pair{{name: "id", value: id}, {name: "operate", value: operate}}
	if folder, found, valueErr := businessValue(args, "--folder-id"); valueErr != nil {
		return nil, valueErr
	} else if found {
		if strings.TrimSpace(folder) == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "--folder-id 不能为空"}
		}
		params = append(params, pair{name: "folderWid", value: folder})
	}
	result, requestErr := a.execute(ctx, siteRequest{
		Service: "ehall", Method: "GET", Path: "/collectService", Params: params,
		Headers: ehallRequestOptions(cookie).headers, CookieFile: cookie, AllowSSO: true,
		RequireLogin: true, AllowBusinessFailure: true, ReadOnly: false, Yes: true, RawJSON: true,
	})
	if requestErr != nil {
		return nil, requestErr
	}
	value, dataErr := ehallEnvelopeValue(result)
	if dataErr != nil {
		if dataErr.Code == "business_rejected" {
			dataErr.Code = "mutation_rejected"
			dataErr.Details = map[string]any{"submitted": true, "confirmed": false, "evidence": "response", "service_id": id}
		}
		return nil, dataErr
	}
	catalog, readbackErr := a.ehallServices(ctx, cookie)
	if readbackErr != nil {
		return nil, &siteError{Code: "mutation_unverified", Message: "收藏请求已发送但服务目录回读失败", Details: map[string]any{
			"submitted": true, "confirmed": false, "evidence": "readback-error", "service_id": id, "cause": readbackErr.Error(),
		}}
	}
	favorite, found := ehallServiceFavorite(catalog, id)
	if !found || favorite != wantFavorite {
		return nil, &siteError{Code: "mutation_unverified", Message: "收藏请求已发送但未从服务目录确认最终状态", Details: map[string]any{
			"submitted": true, "confirmed": false, "evidence": "readback", "service_id": id,
			"expected": wantFavorite, "actual": favorite, "found": found,
		}}
	}
	return map[string]any{
		"ok": true, "submitted": true, "confirmed": true, "evidence": "readback",
		"service": "ehall", "operation": "favorite-" + operation, "service_id": id,
		"favorite": favorite, "verified_by": "service-catalog.appFavorite", "api_data": value,
	}, nil
}

func ehallServiceFavorite(result map[string]any, id string) (bool, bool) {
	services, ok := result["services"].([]any)
	if !ok {
		return false, false
	}
	for _, item := range services {
		service, ok := item.(map[string]any)
		if !ok || fmt.Sprint(service["serviceId"]) != id {
			continue
		}
		favorite, ok := service["appFavorite"].(bool)
		return favorite, ok
	}
	return false, false
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
	value, dataErr := ehallEnvelopeValue(result)
	if dataErr != nil {
		return nil, dataErr
	}
	data, ok := value.(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "eHall 响应缺少 data 对象"}
	}
	return data, nil
}

func ehallEnvelopeValue(result map[string]any) (any, *siteError) {
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
	data, exists := envelope["data"]
	if !exists {
		return nil, &siteError{Code: "parse_error", Message: "eHall 响应缺少 data"}
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
	cards := findEhallCards(value, cardID)
	if len(cards) == 0 {
		return nil
	}
	return cards[0]
}

func findEhallCards(value any, cardID string) []map[string]any {
	var cards []map[string]any
	switch typed := value.(type) {
	case map[string]any:
		if typed["cardId"] == cardID {
			cards = append(cards, typed)
		}
		for _, nested := range typed {
			cards = append(cards, findEhallCards(nested, cardID)...)
		}
	case []any:
		for _, nested := range typed {
			cards = append(cards, findEhallCards(nested, cardID)...)
		}
	}
	return cards
}
