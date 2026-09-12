package adapter

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

const visitReservationService = "sqyrjd"

const visitReservationPublicPath = "/engine2/m/0/1914391/2728822?p=376911&t=5944357&currentBranch=0"

var visitReservationAssignment = regexp.MustCompile(`(?m)\bvar\s+([A-Za-z_$][\w$]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([0-9]+))\s*;`)

func (a NativeSite) executeVisitReservation(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "status" {
		statusArgs := args
		if len(statusArgs) > 0 {
			statusArgs = statusArgs[1:]
		}
		cookie, _, valueErr := businessValue(statusArgs, "--cookie-file")
		if valueErr != nil {
			return nil, valueErr
		}
		return a.executeServiceStatusWithOptions(ctx, visitReservationService, "status", "三全育人教育基地入馆预约", businessRequestOptions{cookieFile: cookie})
	}
	if args[0] == "catalog" {
		result := businessCatalogFilter("visit-reservation")
		result["operations"] = []string{"status", "guide"}
		return result, nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	if args[0] == "guide" || args[0] == "booking-info" {
		return a.visitReservationGuide(ctx, cookie)
	}
	return nil, &siteError{Code: "invalid_argument", Message: "visit-reservation 只支持 status、guide、catalog"}
}

func (a NativeSite) visitReservationGuide(ctx context.Context, cookie string) (map[string]any, *siteError) {
	options := businessRequestOptions{cookieFile: cookie}
	home, requestErr := a.businessGet(ctx, visitReservationService, "/", nil, options)
	if requestErr != nil {
		return nil, requestErr
	}
	homeURL := safeResponseURL(home)
	document, parseErr := parsePage(businessBody(home))
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "入馆预约首页解析失败: " + parseErr.Error()}
	}
	target := visitReservationGuideTarget(document, homeURL)
	if target == "" {
		target = visitReservationPublicPath
	}
	entry, requestErr := a.businessGet(ctx, visitReservationService, target, nil, options)
	if requestErr != nil {
		return nil, requestErr
	}
	entrySource := businessBody(entry)
	engineInstanceID, ok := visitReservationAssignmentValue(entrySource, "engineInstanceId")
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "入馆预约说明页面缺少业务实例"}
	}
	sign, ok := visitReservationAssignmentValue(entrySource, "sign")
	if !ok || sign == "" {
		return nil, &siteError{Code: "parse_error", Message: "入馆预约说明页面缺少请求签名"}
	}
	typeID, ok := visitReservationAssignmentValue(entrySource, "typeId")
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "入馆预约说明页面缺少说明类型"}
	}
	pageID, ok := visitReservationAssignmentValue(entrySource, "pageId")
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "入馆预约说明页面缺少页面标识"}
	}
	detailPath := visitReservationDetailPath(engineInstanceID, typeID, pageID, sign)
	detail, requestErr := a.businessGet(ctx, visitReservationService, detailPath, nil, options)
	if requestErr != nil {
		return nil, requestErr
	}
	introduction, introductionErr := visitReservationIntroduction(businessBody(detail))
	if introductionErr != nil {
		return nil, introductionErr
	}
	guideDocument, parseErr := parsePage(introduction)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "入馆预约说明内容解析失败: " + parseErr.Error()}
	}
	instructions := strings.TrimSpace(pageDisplayText(guideDocument))
	if instructions == "" {
		return nil, &siteError{Code: "parse_error", Message: "入馆预约说明页面没有可读内容"}
	}
	methods := make([]string, 0, 2)
	if strings.Contains(instructions, "小程序") {
		methods = append(methods, "mini-program")
	}
	if strings.Contains(instructions, "公众号") {
		methods = append(methods, "wechat-public-account")
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "sqyrjd 公开入馆预约说明页面；页面无直接网页表单",
		"service":  visitReservationService, "operation": "guide",
		"booking_methods": methods, "offline_booking": strings.Contains(instructions, "线下预约"),
		"direct_web_form":           len(guideDocument.findAll("form")) > 0,
		"requires_external_channel": true, "instructions": instructions,
		"guide_path": visitReservationPath(target),
	}, nil
}

func visitReservationAssignmentValue(source, name string) (string, bool) {
	for _, match := range visitReservationAssignment.FindAllStringSubmatch(source, -1) {
		if match[1] != name {
			continue
		}
		for _, value := range match[2:] {
			if value != "" {
				return value, true
			}
		}
		return "", true
	}
	return "", false
}

func visitReservationDetailPath(engineInstanceID, typeID, pageID, sign string) string {
	query := url.Values{
		"engineInstanceId": {engineInstanceID}, "sign": {sign}, "typeId": {typeID},
		"topTypeId": {""}, "sw": {""}, "currentBranch": {"0"},
		"websiteId": {"208789"}, "pageId": {pageID},
	}
	return "/engine2/general/1914391/more/detail?" + query.Encode()
}

func visitReservationIntroduction(source string) (string, *siteError) {
	index := strings.Index(source, "generalDataType: {")
	if index < 0 {
		return "", &siteError{Code: "parse_error", Message: "入馆预约详情响应缺少说明数据"}
	}
	var data map[string]any
	if err := json.NewDecoder(strings.NewReader(source[index+len("generalDataType: "):])).Decode(&data); err != nil {
		return "", &siteError{Code: "parse_error", Message: "入馆预约详情数据解析失败: " + err.Error()}
	}
	introduction, ok := data["introduction"].(string)
	if !ok || strings.TrimSpace(introduction) == "" {
		return "", &siteError{Code: "parse_error", Message: "入馆预约详情没有说明内容"}
	}
	return introduction, nil
}

func visitReservationGuideTarget(document *pageNode, pageURL string) string {
	for _, link := range document.findAll("a") {
		if !strings.Contains(strings.TrimSpace(pageDisplayText(link)), "入馆预约") {
			continue
		}
		targetValue := resolvePageURL(pageURL, link.attr("href"))
		if !officialSameOrigin(pageURL, targetValue) {
			continue
		}
		target, err := url.Parse(targetValue)
		if err != nil || target.Path == "" {
			continue
		}
		return target.RequestURI()
	}
	return ""
}

func visitReservationPath(value string) string {
	target, err := url.Parse(value)
	if err != nil || target.Path == "" {
		return "/"
	}
	return target.EscapedPath()
}
