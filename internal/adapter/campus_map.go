package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const campusMapService = "campus-map"

type campusMapCampus struct {
	name       string
	campusCode int
	zoneID     int
}

func (a NativeSite) executeCampusMap(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return map[string]any{
			"ok": true, "submitted": false, "confirmed": true, "evidence": "live campus map APIs",
			"service": campusMapService, "operations": []string{"zones", "types", "points", "point", "search", "panoramas", "stats"},
		}, nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "zones":
		return a.campusMapZones(ctx, cookie)
	case "types", "categories":
		return a.campusMapTypes(ctx, args[1:], cookie)
	case "points":
		return a.campusMapPoints(ctx, args[1:], cookie)
	case "point", "detail":
		return a.campusMapPoint(ctx, args[1:], cookie)
	case "search":
		return a.campusMapSearch(ctx, args[1:], cookie)
	case "panoramas", "roam":
		return a.campusMapPanoramas(ctx, args[1:], cookie)
	case "stats":
		return a.campusMapStats(ctx, args[1:], cookie)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "campus-map 只支持 zones、types、points、point、search、panoramas、stats、catalog"}
	}
}

func (a NativeSite) campusMapGet(ctx context.Context, path string, params []pair, cookie string) (map[string]any, *siteError) {
	return a.businessGet(ctx, campusMapService, path, params, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true})
}

func (a NativeSite) campusMapPost(ctx context.Context, path string, body any, headers []pair, cookie string) (map[string]any, *siteError) {
	return a.businessPostJSON(ctx, campusMapService, path, body, businessRequestOptions{cookieFile: cookie, headers: headers, allowBusinessFailure: true})
}

func campusMapPayload(result map[string]any) (map[string]any, *siteError) {
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return nil, parseErr
	}
	if remoteCode, exists := payload["code"]; exists {
		code := fmt.Sprint(remoteCode)
		if code != "0" && code != "200" {
			return nil, &siteError{Code: "business_rejected", Message: firstNonEmpty(campusMapString(payload, "message"), campusMapString(payload, "msg"), "校园地图接口返回失败"), Details: map[string]any{"remote_code": remoteCode}}
		}
	}
	return payload, nil
}

func campusMapScope(args []string) (campusMapCampus, *siteError) {
	value, _, valueErr := businessValue(args, "--campus")
	if valueErr != nil {
		return campusMapCampus{}, valueErr
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "yuntang", "云塘", "云塘校区":
		return campusMapCampus{name: "云塘校区", campusCode: 3, zoneID: 1}, nil
	case "jinpenling", "金盆岭", "金盆岭校区":
		return campusMapCampus{name: "金盆岭校区", campusCode: 4, zoneID: 4}, nil
	case "jinpenling2", "金盆岭2":
		return campusMapCampus{name: "金盆岭2", campusCode: 5, zoneID: 5}, nil
	default:
		code, parseErr := strconv.Atoi(strings.TrimSpace(value))
		if parseErr != nil || code < 1 {
			return campusMapCampus{}, &siteError{Code: "invalid_argument", Message: "--campus 只能是云塘、金盆岭、金盆岭2或正整数校区编号"}
		}
		return campusMapCampus{name: value, campusCode: code, zoneID: code}, nil
	}
}

func (a NativeSite) campusMapZones(ctx context.Context, cookie string) (map[string]any, *siteError) {
	result, requestErr := a.campusMapGet(ctx, "/cmgis-server/map/v2/zone/page", []pair{{"page", "0"}, {"pageSize", "1000"}}, cookie)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := campusMapPayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "校园地图校区响应缺少列表"}
	}
	rows := campusMapMaps(data["content"])
	zones := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		zones = append(zones, campusMapZone(row))
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "校区地图接口返回结构化配置", "service": campusMapService, "operation": "zones", "zones": zones, "total": len(zones)}, nil
}

func (a NativeSite) campusMapTypes(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	scope, scopeErr := campusMapScope(args)
	if scopeErr != nil {
		return nil, scopeErr
	}
	category, _, valueErr := businessValue(args, "--category")
	if valueErr != nil {
		return nil, valueErr
	}
	rows, rowsErr := a.campusMapTypeRows(ctx, scope, strings.TrimSpace(category), cookie)
	if rowsErr != nil {
		return nil, rowsErr
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, campusMapType(row))
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "公共点分类接口返回结构化数据", "service": campusMapService, "operation": "types", "campus": scope.name, "data": items, "total": len(items)}, nil
}

func (a NativeSite) campusMapTypeRows(ctx context.Context, scope campusMapCampus, category, cookie string) ([]map[string]any, *siteError) {
	rootResult, requestErr := a.campusMapGet(ctx, "/cmips-server/situationalIntelligence/publicPointType/queryListWithDisplayParentCode", []pair{{"campusCode", strconv.Itoa(scope.campusCode)}, {"isVector", "false"}}, cookie)
	if requestErr != nil {
		return nil, requestErr
	}
	rootPayload, payloadErr := campusMapPayload(rootResult)
	if payloadErr != nil {
		return nil, payloadErr
	}
	roots := campusMapMaps(rootPayload["data"])
	if category != "" && category != "全部" {
		parent := ""
		available := make([]string, 0, len(roots))
		for _, row := range roots {
			name := campusMapString(row, "typeName")
			if name != "" {
				available = append(available, name)
			}
			if name == category {
				parent = campusMapString(row, "typeCode")
			}
		}
		if parent == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "--category 未找到校园地图分类", Details: map[string]any{"category": category, "available_categories": available}}
		}
		return a.campusMapChildTypes(ctx, scope, []string{parent}, cookie)
	}
	parents := make([]string, 0, len(roots))
	for _, row := range roots {
		if code := campusMapString(row, "typeCode"); code != "" {
			parents = append(parents, code)
		}
	}
	children, childrenErr := a.campusMapChildTypes(ctx, scope, parents, cookie)
	if childrenErr != nil {
		return nil, childrenErr
	}
	return append(roots, children...), nil
}

func (a NativeSite) campusMapChildTypes(ctx context.Context, scope campusMapCampus, parents []string, cookie string) ([]map[string]any, *siteError) {
	params := []pair{{"campusCode", strconv.Itoa(scope.campusCode)}, {"isVector", "false"}}
	for _, parent := range parents {
		params = append(params, pair{"parentCodes", parent})
	}
	params = append(params, pair{"parentCodes", "0"})
	result, requestErr := a.campusMapGet(ctx, "/cmips-server/situationalIntelligence/publicPointType/queryListWithDisplayParentCode", params, cookie)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := campusMapPayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	return campusMapMaps(payload["data"]), nil
}

func (a NativeSite) campusMapPoints(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	scope, scopeErr := campusMapScope(args)
	if scopeErr != nil {
		return nil, scopeErr
	}
	codes, codesErr := a.campusMapPointTypeCodes(ctx, args, scope, cookie)
	if codesErr != nil {
		return nil, codesErr
	}
	params := []pair{{"campusCode", strconv.Itoa(scope.campusCode)}, {"isVector", "false"}}
	for _, code := range codes {
		params = append(params, pair{"typeCodes", code})
	}
	result, requestErr := a.campusMapGet(ctx, "/cmips-server/situationalIntelligence/publicPoint/queryAllByTypeCodesCampusCode", params, cookie)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := campusMapPayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	rows := campusMapMaps(payload["data"])
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, campusMapPointModel(row))
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "公共点接口返回结构化数据", "service": campusMapService, "operation": "points", "campus": scope.name, "data": items, "total": len(items)}, nil
}

func (a NativeSite) campusMapPointTypeCodes(ctx context.Context, args []string, scope campusMapCampus, cookie string) ([]string, *siteError) {
	types, typesErr := a.campusMapTypeRows(ctx, scope, "", cookie)
	if typesErr != nil {
		return nil, typesErr
	}
	names, namesErr := businessValues(args, "--type")
	if namesErr != nil {
		return nil, namesErr
	}
	direct, directErr := businessValues(args, "--type-code")
	if directErr != nil {
		return nil, directErr
	}
	if len(names) == 0 && len(direct) == 0 {
		codes := make([]string, 0, len(types))
		for _, row := range types {
			if campusMapString(row, "parentCode") != "" {
				codes = appendUniqueString(codes, campusMapString(row, "typeCode"))
			}
		}
		return codes, nil
	}
	codes := make([]string, 0, len(names)+len(direct))
	for _, value := range direct {
		code, parseErr := strconv.Atoi(strings.TrimSpace(value))
		if parseErr != nil || code < 1 {
			return nil, &siteError{Code: "invalid_argument", Message: "--type-code 必须是正整数"}
		}
		codes = appendUniqueString(codes, strconv.Itoa(code))
	}
	available := make([]string, 0, len(types))
	for _, row := range types {
		available = append(available, campusMapString(row, "typeName"))
	}
	for _, name := range names {
		found := false
		for _, row := range types {
			if campusMapString(row, "typeName") == strings.TrimSpace(name) {
				codes = appendUniqueString(codes, campusMapString(row, "typeCode"))
				found = true
				break
			}
		}
		if !found {
			return nil, &siteError{Code: "invalid_argument", Message: "--type 未找到校园地图公共点分类", Details: map[string]any{"type": name, "available_types": available}}
		}
	}
	return codes, nil
}

func (a NativeSite) campusMapPoint(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	id, requiredErr := businessRequired(args, "--id", "point detail 必须提供 --id")
	if requiredErr != nil {
		return nil, requiredErr
	}
	result, requestErr := a.campusMapGet(ctx, "/cmips-server/situationalIntelligence/loadPublicPointDetail/"+url.PathEscape(strings.TrimSpace(id)), nil, cookie)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := campusMapPayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	row, ok := payload["data"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "校园地图公共点详情缺少数据"}
	}
	point := campusMapPointModel(row)
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "公共点详情接口返回结构化数据", "service": campusMapService, "operation": "point", "id": id, "point": point, "data": point}, nil
}

func (a NativeSite) campusMapSearch(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	keyword, requiredErr := businessRequired(args, "--keyword", "search 必须提供 --keyword")
	if requiredErr != nil {
		return nil, requiredErr
	}
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	pageSize, pageSizeErr := businessInt(args, "--page-size", 20)
	if pageSizeErr != nil {
		return nil, pageSizeErr
	}
	scope, scopeErr := campusMapScope(args)
	if scopeErr != nil {
		return nil, scopeErr
	}
	token, tokenErr := a.campusMapToken(ctx, cookie)
	if tokenErr != nil {
		return nil, tokenErr
	}
	body := map[string]any{"keywords": keyword, "page": page, "pageSize": pageSize, "zoneid": scope.zoneID, "querytype": "key", "cmccrToken": "", "systemType": []string{"map", "org", "pubpoi", "panorama"}}
	result, requestErr := a.campusMapPost(ctx, "/cmgis-server/map/v2/search", body, []pair{{"Authorization", "Basic " + token}}, cookie)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := campusMapPayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "校园地图搜索响应缺少结果列表"}
	}
	rows := campusMapMaps(data["list"])
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, campusMapSearchItem(row))
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "地图搜索接口返回结构化结果", "service": campusMapService, "operation": "search", "campus": scope.name, "keyword": keyword, "page": page, "page_size": pageSize, "data": items, "total": data["total"]}, nil
}

func (a NativeSite) campusMapToken(ctx context.Context, cookie string) (string, *siteError) {
	result, requestErr := a.campusMapPost(ctx, "/cmccr-server/center/store/batch/query", []map[string]string{{"storeName": "runConfig", "key": "baseConfig"}}, nil, cookie)
	if requestErr != nil {
		return "", requestErr
	}
	payload, payloadErr := campusMapPayload(result)
	if payloadErr != nil {
		return "", payloadErr
	}
	rows := campusMapMaps(payload["data"])
	if len(rows) == 0 {
		return "", &siteError{Code: "parse_error", Message: "校园地图配置缺少搜索凭据"}
	}
	content, ok := rows[0]["contentValue"].(map[string]any)
	if !ok {
		return "", &siteError{Code: "parse_error", Message: "校园地图配置格式无效"}
	}
	token := campusMapString(content, "map_token")
	if token == "" {
		return "", &siteError{Code: "parse_error", Message: "校园地图配置缺少搜索凭据"}
	}
	return token, nil
}

func (a NativeSite) campusMapPanoramas(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	scope, scopeErr := campusMapScope(args)
	if scopeErr != nil {
		return nil, scopeErr
	}
	kind, _, valueErr := businessValue(args, "--kind")
	if valueErr != nil {
		return nil, valueErr
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		kind = "all"
	}
	if kind != "all" && kind != "aerial" && kind != "panorama" {
		return nil, &siteError{Code: "invalid_argument", Message: "--kind 只能是 all、aerial 或 panorama"}
	}
	items := make([]map[string]any, 0)
	if kind == "all" || kind == "aerial" {
		result, requestErr := a.campusMapGet(ctx, "/cmips-server/roam/search", []pair{{"roamType", "1"}, {"campusCode", strconv.Itoa(scope.campusCode)}, {"page", "0"}, {"pageSize", "1000"}}, cookie)
		if requestErr != nil {
			return nil, requestErr
		}
		payload, payloadErr := campusMapPayload(result)
		if payloadErr != nil {
			return nil, payloadErr
		}
		if data, ok := payload["data"].(map[string]any); ok {
			for _, row := range campusMapMaps(data["content"]) {
				items = append(items, campusMapRoam(row, "aerial"))
			}
		}
	}
	if kind == "all" || kind == "panorama" {
		result, requestErr := a.campusMapGet(ctx, "/cmips-server/roam/listQuery", []pair{{"roamType", "2"}, {"campusCode", strconv.Itoa(scope.campusCode)}}, cookie)
		if requestErr != nil {
			return nil, requestErr
		}
		payload, payloadErr := campusMapPayload(result)
		if payloadErr != nil {
			return nil, payloadErr
		}
		for _, row := range campusMapMaps(payload["data"]) {
			items = append(items, campusMapRoam(row, "panorama"))
		}
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "校园漫游接口返回结构化资源", "service": campusMapService, "operation": "panoramas", "campus": scope.name, "kind": kind, "data": items, "total": len(items)}, nil
}

func (a NativeSite) campusMapStats(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	scope, scopeErr := campusMapScope(args)
	if scopeErr != nil {
		return nil, scopeErr
	}
	result, requestErr := a.campusMapGet(ctx, "/cmips-server/situationalIntelligence/publicPointType/countListWithDisplayParentCode", []pair{{"campusCode", strconv.Itoa(scope.campusCode)}, {"isVector", "false"}}, cookie)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := campusMapPayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "校园地图统计响应格式无效"}
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "校园地图统计接口返回结构化数据", "service": campusMapService, "operation": "stats", "campus": scope.name, "total_points": data["totalPoint"], "total_thematic": data["totalThematicInfo"]}, nil
}

func campusMapZone(row map[string]any) map[string]any {
	return map[string]any{
		"id": row["id"], "name": row["name"], "center": row["center"], "bbox": row["bbox"], "polygon_bbox": row["polygonBBox"],
		"min_zoom": row["minZoom"], "max_zoom": row["maxZoom"], "default_zoom": row["defaultZoom"], "style_id": row["styleId"],
		"is_2d": row["is2D"], "raster_type": row["rasterType"], "has_route_cache": row["hasRouteCache"], "has_map_cache": row["hasMapCache"], "icon": row["icon"],
	}
}

func campusMapType(row map[string]any) map[string]any {
	return map[string]any{"type_code": row["typeCode"], "parent_code": row["parentCode"], "name": row["typeName"], "point_count": row["mapPointCount"], "searchable": row["search"], "displayable": row["display"], "default_display": row["defaultDisplay"], "hot": row["hot"], "hot_order": row["hotOrder"]}
}

func campusMapPointModel(row map[string]any) map[string]any {
	model := map[string]any{
		"id": row["pointCode"], "type_code": row["typeCode"], "name": row["pointName"], "campus_code": row["campusCode"],
		"location": row["location"], "brief": row["brief"], "contact": row["contact"], "order": row["orderId"],
		"hot": row["gnsHot"], "building": row["buildingName"], "audio_url": row["audioUrl"], "video_url": row["videoUrl"],
	}
	if coordinates := campusMapCoordinateString(campusMapString(row, "lngLatString"), campusMapString(row, "rasterLngLatString")); coordinates != nil {
		model["coordinates"] = coordinates
	}
	if pointType, ok := row["mapPointType"].(map[string]any); ok {
		model["type"] = map[string]any{"code": pointType["typeCode"], "parent_code": pointType["parentCode"], "name": pointType["typeName"]}
	}
	if images := campusMapImages(row["mapPointImgList"]); len(images) > 0 {
		model["images"] = images
	}
	return model
}

func campusMapSearchItem(row map[string]any) map[string]any {
	return map[string]any{"id": row["id"], "name": row["name"], "system_type": row["systemType"], "category": row["category"], "parent_id": row["parentId"], "center": row["center"], "geometry": row["geometry"], "content": row["content"], "create_time": row["createTime"], "distance": row["distance"]}
}

func campusMapRoam(row map[string]any, kind string) map[string]any {
	item := map[string]any{"id": row["roamId"], "name": row["roamName"], "kind": kind, "campus": row["campusName"], "campus_code": row["campusCode"], "location": row["location"], "url": row["roamnUrl"], "update_time": row["updateTime"], "order": row["orderId"], "building": row["buildingName"]}
	for _, key := range []string{"lngLat", "rasterLngLat"} {
		if value := campusMapString(row, key); value != "" {
			var point map[string]any
			if json.Unmarshal([]byte(value), &point) == nil && point["coordinates"] != nil {
				item["coordinates"] = point["coordinates"]
				break
			}
		}
	}
	return item
}

func campusMapMaps(value any) []map[string]any {
	rows, _ := value.([]any)
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if mapped, ok := row.(map[string]any); ok {
			result = append(result, mapped)
		}
	}
	return result
}

func campusMapString(row map[string]any, key string) string {
	value, ok := row[key]
	if !ok || value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "<nil>" {
		return ""
	}
	return text
}

func campusMapCoordinateString(values ...string) []float64 {
	for _, value := range values {
		parts := strings.Split(value, ",")
		if len(parts) != 2 {
			continue
		}
		longitude, longitudeErr := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		latitude, latitudeErr := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if longitudeErr == nil && latitudeErr == nil {
			return []float64{longitude, latitude}
		}
	}
	return nil
}

func campusMapImages(value any) []map[string]any {
	rows := campusMapMaps(value)
	images := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		images = append(images, map[string]any{"id": row["imgId"], "name": row["imgName"], "url": row["imgUrl"], "order": row["orderId"]})
	}
	return images
}

func appendUniqueString(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
