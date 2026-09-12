package adapter

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const campusNetworkService = "campus-network"

var campusNetworkMAC = regexp.MustCompile(`^[0-9A-Fa-f]{12}$`)

type campusNetworkPeriod struct {
	data  []pair
	label string
}

type campusNetworkPackageField struct {
	key      string
	flag     string
	selectID string
	name     string
}

var campusNetworkPackageFields = []campusNetworkPackageField{
	{key: "user_type", flag: "--user-type", selectID: "userpackageid", name: "fldname"},
	{key: "network", flag: "--network", selectID: "cooid", name: "fldcooperation"},
	{key: "bandwidth", flag: "--bandwidth", selectID: "bandwidthid", name: "fldbandwidth"},
	{key: "login_method", flag: "--login-method", selectID: "loginwayid", name: "fldloginway"},
	{key: "billing_type", flag: "--billing-type", selectID: "typeid", name: "fldtype"},
	{key: "terminals", flag: "--terminals", selectID: "loginnumbersid", name: "fldloginnumber"},
	{key: "cost", flag: "--cost", selectID: "costsid", name: "fldcost"},
}

type campusNetworkOnlineEntry struct {
	sessionID string
	values    []string
}

func (a NativeSite) executeCampusNetwork(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return businessCatalogFilter(campusNetworkService), nil
	}
	if args[0] == "login" || args[0] == "logout" {
		return a.executeSSOServiceCommand(ctx, args, campusNetworkService, "校园网自助服务")
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "status":
		return a.campusNetworkStatus(ctx, cookie)
	case "profile":
		return a.campusNetworkProfile(ctx, cookie)
	case "bills", "billing":
		return a.campusNetworkBills(ctx, args[1:], cookie)
	case "usage", "online-usage":
		return a.campusNetworkPeriodQuery(ctx, "usage", args[1:], cookie)
	case "operations", "operator-log":
		return a.campusNetworkPeriodQuery(ctx, "operations", args[1:], cookie)
	case "payments":
		return a.campusNetworkPeriodQuery(ctx, "payments", args[1:], cookie)
	case "online":
		return a.campusNetworkOnline(ctx, cookie)
	case "disconnect", "offline":
		return a.campusNetworkDisconnect(ctx, args[1:], cookie)
	case "devices":
		return a.campusNetworkDevices(ctx, cookie)
	case "set-devices", "bind-devices":
		return a.campusNetworkSetDevices(ctx, args[1:], cookie)
	case "unbind-device":
		return a.campusNetworkUnbindDevice(ctx, args[1:], cookie)
	case "package-options":
		return a.campusNetworkPackageOptions(ctx, cookie)
	case "package", "reserve-package":
		return a.campusNetworkReservePackage(ctx, args[1:], cookie)
	case "update-profile":
		return a.campusNetworkUpdateProfile(ctx, args[1:], cookie)
	case "change-password":
		return a.campusNetworkChangePassword(ctx, args[1:], cookie)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "campus-network 不支持该子命令: " + args[0]}
	}
}

func (a NativeSite) campusNetworkPage(ctx context.Context, path, cookie string) (*pageNode, string, *siteError) {
	result, requestErr := a.businessGet(ctx, campusNetworkService, path, nil, businessRequestOptions{cookieFile: cookie, require: true})
	if requestErr != nil {
		return nil, "", requestErr
	}
	body := businessBody(result)
	if strings.TrimSpace(body) == "" {
		return nil, "", &siteError{Code: "parse_error", Message: "校园网自助服务响应为空"}
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, "", &siteError{Code: "parse_error", Message: "校园网自助服务页面解析失败: " + parseErr.Error()}
	}
	return document, safeResponseURL(result), nil
}

func campusNetworkResult(operation string, data any) map[string]any {
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "server_page",
		"service": campusNetworkService, "operation": operation, "data": data,
	}
}

func (a NativeSite) campusNetworkStatus(ctx context.Context, cookie string) (map[string]any, *siteError) {
	_, responseURL, pageErr := a.campusNetworkPage(ctx, "/Self/nav_main", cookie)
	if pageErr != nil {
		return nil, pageErr
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "authenticated_page",
		"service": campusNetworkService, "operation": "status", "authenticated": true,
		"url": safeSiteURL(mustParseURL(responseURL)),
	}, nil
}

func (a NativeSite) campusNetworkProfile(ctx context.Context, cookie string) (map[string]any, *siteError) {
	document, _, pageErr := a.campusNetworkPage(ctx, "/Self/nav_getUserInfo", cookie)
	if pageErr != nil {
		return nil, pageErr
	}
	return campusNetworkResult("profile", campusNetworkProfileData(document)), nil
}

func campusNetworkProfileData(document *pageNode) []map[string]any {
	items := make([]map[string]any, 0)
	seen := map[string]bool{}
	for _, table := range document.findAll("table") {
		for _, row := range table.findAll("tr") {
			cells := campusNetworkVisibleCells(row)
			if len(cells) != 2 {
				continue
			}
			label := strings.TrimSpace(pageDisplayText(cells[0]))
			value := strings.TrimSpace(pageDisplayText(cells[1]))
			if label == "" || strings.EqualFold(label, "帐户信息") {
				continue
			}
			key := campusNetworkProfileKey(label)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			items = append(items, map[string]any{"name": key, "label": label, "value": campusNetworkSafeValue(label, value)})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i]["name"].(string) < items[j]["name"].(string) })
	return items
}

func campusNetworkProfileValues(document *pageNode) map[string]string {
	values := map[string]string{}
	for _, table := range document.findAll("table") {
		for _, row := range table.findAll("tr") {
			cells := campusNetworkVisibleCells(row)
			if len(cells) != 2 {
				continue
			}
			key := campusNetworkProfileKey(pageDisplayText(cells[0]))
			if key != "" {
				values[key] = strings.TrimSpace(pageDisplayText(cells[1]))
			}
		}
	}
	return values
}

func campusNetworkProfileKey(label string) string {
	label = strings.TrimSpace(strings.TrimRight(label, ":："))
	switch label {
	case "余额（元）":
		return "balance"
	case "本月时长（分钟）":
		return "monthly_minutes"
	case "本月流量（MB）":
		return "monthly_megabytes"
	case "时长/流量计费（元）":
		return "usage_fee"
	case "用户类别":
		return "user_type"
	case "安装地址":
		return "installation_address"
	case "布线资料":
		return "wiring_information"
	case "联系电话":
		return "phone"
	case "证件号码":
		return "id_number"
	case "电子邮箱":
		return "email"
	case "账单地址":
		return "billing_address"
	case "失效日期":
		return "expires_at"
	case "账号", "账号（登录名）":
		return "account"
	case "套餐":
		return "package"
	case "状态":
		return "status"
	case "防伪信息":
		return "anti_fraud"
	default:
		return label
	}
}

func campusNetworkSensitiveLabel(label string) bool {
	value := strings.ToLower(strings.TrimSpace(label))
	for _, term := range []string{"账号", "姓名", "电话", "证件", "邮箱", "地址", "布线", "mac", "ip", "主机名"} {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}

func campusNetworkSafeValue(label, value string) string {
	if campusNetworkSensitiveLabel(label) {
		return "<redacted>"
	}
	return value
}

func campusNetworkVisibleCells(row *pageNode) []*pageNode {
	cells := make([]*pageNode, 0)
	if row == nil {
		return cells
	}
	for _, child := range row.children {
		if child.tag != "th" && child.tag != "td" {
			continue
		}
		if strings.Contains(strings.ReplaceAll(strings.ToLower(child.attr("style")), " ", ""), "display:none") {
			continue
		}
		cells = append(cells, child)
	}
	return cells
}

func campusNetworkTableRows(table *pageNode) ([]string, [][]string) {
	rows := make([][]string, 0)
	hasHeader := false
	for _, row := range table.findAll("tr") {
		cells := campusNetworkVisibleCells(row)
		if len(cells) == 0 {
			continue
		}
		values := make([]string, 0, len(cells))
		for _, cell := range cells {
			values = append(values, strings.TrimSpace(pageDisplayText(cell)))
		}
		rows = append(rows, values)
		if len(row.findAll("th")) > 0 {
			hasHeader = true
		}
	}
	if !hasHeader || len(rows) < 1 {
		return nil, nil
	}
	return rows[0], rows[1:]
}

func campusNetworkTables(document *pageNode) []map[string]any {
	tables := make([]map[string]any, 0)
	for _, table := range document.findAll("table") {
		headers, rows := campusNetworkTableRows(table)
		if len(headers) == 0 {
			continue
		}
		safeRows := make([][]string, 0, len(rows))
		for _, row := range rows {
			values := make([]string, len(headers))
			for index := range headers {
				if index < len(row) {
					values[index] = campusNetworkSafeValue(headers[index], row[index])
				}
			}
			safeRows = append(safeRows, values)
		}
		tables = append(tables, map[string]any{"headers": headers, "rows": safeRows})
	}
	return tables
}

func (a NativeSite) campusNetworkBills(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	year, yearErr := businessInt(args, "--year", time.Now().Year())
	if yearErr != nil {
		return nil, yearErr
	}
	result, requestErr := a.campusNetworkPostPage(ctx, "/Self/MonthPayAction", []pair{{"type", "1"}, {"year", strconv.Itoa(year)}}, cookie)
	if requestErr != nil {
		return nil, requestErr
	}
	return campusNetworkResult("bills", map[string]any{"year": year, "tables": campusNetworkTables(result)}), nil
}

func campusNetworkPeriodOptions(args []string, operation string) (campusNetworkPeriod, *siteError) {
	date, dateFound, err := businessValue(args, "--date")
	if err != nil {
		return campusNetworkPeriod{}, err
	}
	from, fromFound, err := businessValue(args, "--from")
	if err != nil {
		return campusNetworkPeriod{}, err
	}
	to, toFound, err := businessValue(args, "--to")
	if err != nil {
		return campusNetworkPeriod{}, err
	}
	month, monthFound, err := businessValue(args, "--month")
	if err != nil {
		return campusNetworkPeriod{}, err
	}
	selected := 0
	for _, found := range []bool{dateFound, fromFound || toFound, monthFound} {
		if found {
			selected++
		}
	}
	if selected > 1 || fromFound != toFound {
		return campusNetworkPeriod{}, &siteError{Code: "invalid_argument", Message: "时间范围参数只能选择 --date、--from/--to 或 --month"}
	}
	if dateFound {
		if !campusNetworkDate(date) {
			return campusNetworkPeriod{}, &siteError{Code: "invalid_argument", Message: "--date 必须是 YYYY-MM-DD"}
		}
		return campusNetworkPeriod{data: []pair{{"type", "4"}, {"startDate", date}, {"endDate", date}}, label: date}, nil
	}
	if fromFound {
		if !campusNetworkDate(from) || !campusNetworkDate(to) || from > to {
			return campusNetworkPeriod{}, &siteError{Code: "invalid_argument", Message: "--from 和 --to 必须是有效的 YYYY-MM-DD，且开始日期不能晚于结束日期"}
		}
		return campusNetworkPeriod{data: []pair{{"type", "4"}, {"startDate", from}, {"endDate", to}}, label: from + " -- " + to}, nil
	}
	if monthFound {
		year, monthNumber, monthErr := campusNetworkMonth(month)
		if monthErr != nil {
			return campusNetworkPeriod{}, monthErr
		}
		if operation == "usage" {
			return campusNetworkPeriod{data: []pair{{"type", "3"}, {"month", fmt.Sprintf("CHECKER.TBLUSERLOGIN%04d%02d", year, monthNumber)}}, label: fmt.Sprintf("%04d-%02d", year, monthNumber)}, nil
		}
		return campusNetworkPeriod{data: []pair{{"type", "3"}, {"month", fmt.Sprintf("%04d-%d", year, monthNumber)}}, label: fmt.Sprintf("%04d-%02d", year, monthNumber)}, nil
	}
	return campusNetworkPeriod{data: []pair{{"type", "1"}}, label: "today"}, nil
}

func campusNetworkDate(value string) bool {
	_, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	return err == nil
}

func campusNetworkMonth(value string) (int, int, *siteError) {
	parts := strings.Split(strings.TrimSpace(value), "-")
	if len(parts) != 2 || len(parts[0]) != 4 {
		return 0, 0, &siteError{Code: "invalid_argument", Message: "--month 必须是 YYYY-MM"}
	}
	year, yearErr := strconv.Atoi(parts[0])
	month, monthErr := strconv.Atoi(parts[1])
	if yearErr != nil || monthErr != nil || year < 1900 || month < 1 || month > 12 {
		return 0, 0, &siteError{Code: "invalid_argument", Message: "--month 必须是有效的 YYYY-MM"}
	}
	return year, month, nil
}

func (a NativeSite) campusNetworkPeriodQuery(ctx context.Context, operation string, args []string, cookie string) (map[string]any, *siteError) {
	period, periodErr := campusNetworkPeriodOptions(args, operation)
	if periodErr != nil {
		return nil, periodErr
	}
	path := map[string]string{"usage": "/Self/UserLoginLogAction", "operations": "/Self/AdminOpLogAction", "payments": "/Self/UserPayAction"}[operation]
	result, requestErr := a.campusNetworkPostPage(ctx, path, period.data, cookie)
	if requestErr != nil {
		return nil, requestErr
	}
	return campusNetworkResult(operation, map[string]any{"period": period.label, "tables": campusNetworkTables(result)}), nil
}

func (a NativeSite) campusNetworkPostPage(ctx context.Context, path string, data []pair, cookie string) (*pageNode, *siteError) {
	result, requestErr := businessRequest(ctx, campusNetworkService, "POST", path, nil, data, nil, businessRequestOptions{cookieFile: cookie, require: true, allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	body := businessBody(result)
	if strings.TrimSpace(body) == "" {
		return nil, &siteError{Code: "parse_error", Message: "校园网自助服务查询响应为空"}
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "校园网自助服务查询页面解析失败: " + parseErr.Error()}
	}
	return document, nil
}

func (a NativeSite) campusNetworkOnline(ctx context.Context, cookie string) (map[string]any, *siteError) {
	document, _, pageErr := a.campusNetworkPage(ctx, "/Self/nav_offLine", cookie)
	if pageErr != nil {
		return nil, pageErr
	}
	entries := campusNetworkOnlineEntries(document)
	items := make([]map[string]any, 0, len(entries))
	for index, entry := range entries {
		values := entry.values
		item := map[string]any{"index": index + 1}
		for fieldIndex, field := range []string{"ipv4", "ipv6", "mac", "hostname", "terminal_type", "model", "action"} {
			if fieldIndex < len(values) {
				item[field] = campusNetworkSafeValue(field, values[fieldIndex])
			}
		}
		items = append(items, item)
	}
	return campusNetworkResult("online", map[string]any{"items": items, "total": len(items)}), nil
}

func campusNetworkOnlineEntries(document *pageNode) []campusNetworkOnlineEntry {
	entries := make([]campusNetworkOnlineEntry, 0)
	for _, table := range document.findAll("table") {
		headers, rows := campusNetworkTableRows(table)
		if len(headers) < 3 || !strings.Contains(strings.Join(headers, " "), "在线IPv4") {
			continue
		}
		for _, row := range table.findAll("tr") {
			visible := campusNetworkVisibleCells(row)
			if len(visible) < 6 || strings.Contains(pageDisplayText(visible[0]), "在线IPv4") {
				continue
			}
			values := make([]string, 0, len(visible))
			for _, cell := range visible {
				values = append(values, strings.TrimSpace(pageDisplayText(cell)))
			}
			hidden := ""
			for _, child := range row.children {
				if (child.tag == "td" || child.tag == "th") && strings.Contains(strings.ReplaceAll(strings.ToLower(child.attr("style")), " ", ""), "display:none") {
					hidden = strings.TrimSpace(pageDisplayText(child))
					break
				}
			}
			entries = append(entries, campusNetworkOnlineEntry{sessionID: hidden, values: values})
		}
		_ = rows
	}
	return entries
}

func (a NativeSite) campusNetworkDisconnect(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	index, indexErr := businessInt(args, "--index", 0)
	if indexErr != nil {
		return nil, indexErr
	}
	if index == 0 {
		return nil, &siteError{Code: "invalid_argument", Message: "disconnect 必须提供正整数 --index"}
	}
	document, _, pageErr := a.campusNetworkPage(ctx, "/Self/nav_offLine", cookie)
	if pageErr != nil {
		return nil, pageErr
	}
	entries := campusNetworkOnlineEntries(document)
	if index > len(entries) || entries[index-1].sessionID == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "--index 不在当前在线设备列表中"}
	}
	result, requestErr := a.execute(ctx, siteRequest{
		Service: campusNetworkService, Method: "GET", Path: "/Self/tooffline",
		Params:     []pair{{"t", strconv.FormatInt(time.Now().UnixNano(), 10)}, {"fldsessionid", entries[index-1].sessionID}},
		CookieFile: cookie, RequireLogin: true, AllowBusinessFailure: true, ReadOnly: false, Yes: true, RawJSON: true,
	})
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := businessJSONMap(result)
	if payloadErr != nil || strings.ToLower(fmt.Sprint(payload["outmessage"])) != "true" {
		if payloadErr != nil {
			return nil, &siteError{Code: "mutation_unverified", Message: "强制离线响应无法确认", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "invalid-response"}}
		}
		return nil, &siteError{Code: "mutation_rejected", Message: "强制离线被校园网服务拒绝", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "outmessage-false"}}
	}
	readback, _, readbackErr := a.campusNetworkPage(ctx, "/Self/nav_offLine", cookie)
	if readbackErr != nil {
		return nil, &siteError{Code: "mutation_unverified", Message: "强制离线成功反馈已返回，但在线列表回读失败", Details: map[string]any{"submitted": true, "confirmed": false, "cause": readbackErr.Code}}
	}
	for _, entry := range campusNetworkOnlineEntries(readback) {
		if entry.sessionID == entries[index-1].sessionID {
			return nil, &siteError{Code: "mutation_unverified", Message: "强制离线成功反馈已返回，但设备仍在线", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "online-readback"}}
		}
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "outmessage-and-online-readback", "service": campusNetworkService, "operation": "disconnect", "index": index}, nil
}

func (a NativeSite) campusNetworkDevices(ctx context.Context, cookie string) (map[string]any, *siteError) {
	document, _, pageErr := a.campusNetworkPage(ctx, "/Self/nav_SetMac", cookie)
	if pageErr != nil {
		return nil, pageErr
	}
	return campusNetworkDeviceResult("devices", campusNetworkMACValues(document)), nil
}

func campusNetworkMACValues(document *pageNode) []string {
	values := make([]string, 0, 5)
	for _, node := range document.findAll("input") {
		if node.attr("name") == "macs" {
			values = append(values, strings.TrimSpace(node.attr("value")))
		}
	}
	return values
}

func campusNetworkFormField(form *pageNode, name string) *pageNode {
	if form == nil {
		return nil
	}
	for _, node := range form.findAll("input") {
		if node.attr("name") == name {
			return node
		}
	}
	return nil
}

func campusNetworkDeviceResult(operation string, values []string) map[string]any {
	items := make([]map[string]any, 0, len(values))
	for index, value := range values {
		items = append(items, map[string]any{"index": index + 1, "mac": value, "configured": value != ""})
	}
	return campusNetworkResult(operation, map[string]any{"items": items, "total": len(items)})
}

func normalizeCampusNetworkMAC(value string) (string, *siteError) {
	value = strings.ToUpper(strings.NewReplacer(":", "", "-", "").Replace(strings.TrimSpace(value)))
	if !campusNetworkMAC.MatchString(value) {
		return "", &siteError{Code: "invalid_argument", Message: "MAC 地址必须是 12 位十六进制字符，可带冒号或短横线"}
	}
	return value, nil
}

func (a NativeSite) campusNetworkSetDevices(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	values, valuesErr := businessValues(args, "--mac")
	if valuesErr != nil {
		return nil, valuesErr
	}
	if len(values) == 0 || len(values) > 5 {
		return nil, &siteError{Code: "invalid_argument", Message: "set-devices 必须提供 1 到 5 个 --mac"}
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		mac, macErr := normalizeCampusNetworkMAC(value)
		if macErr != nil {
			return nil, macErr
		}
		normalized = append(normalized, mac)
	}
	return a.campusNetworkSaveDevices(ctx, normalized, cookie, "set-devices")
}

func (a NativeSite) campusNetworkUnbindDevice(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	index, indexErr := businessInt(args, "--index", 0)
	if indexErr != nil {
		return nil, indexErr
	}
	document, _, pageErr := a.campusNetworkPage(ctx, "/Self/nav_SetMac", cookie)
	if pageErr != nil {
		return nil, pageErr
	}
	values := campusNetworkMACValues(document)
	if index == 0 || index > len(values) || values[index-1] == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "--index 不在已绑定设备列表中"}
	}
	values[index-1] = ""
	return a.campusNetworkSaveDevices(ctx, values, cookie, "unbind-device")
}

func (a NativeSite) campusNetworkSaveDevices(ctx context.Context, values []string, cookie, operation string) (map[string]any, *siteError) {
	document, responseURL, pageErr := a.campusNetworkPage(ctx, "/Self/nav_SetMac", cookie)
	if pageErr != nil {
		return nil, pageErr
	}
	form := document.first("form", "")
	if form == nil {
		return nil, &siteError{Code: "protocol_unconfirmed", Message: "设备绑定页面缺少表单"}
	}
	fields := pageFormFields(form, nil, document)
	fields = campusNetworkSetRepeatedFields(fields, "macs", values)
	_, mutationErr := a.campusNetworkMutation(ctx, "/Self/setMacAction", fields, responseURL, cookie)
	if mutationErr != nil {
		return nil, mutationErr
	}
	readback, _, readbackErr := a.campusNetworkPage(ctx, "/Self/nav_SetMac", cookie)
	if readbackErr != nil || !sameCampusNetworkValues(values, campusNetworkMACValues(readback)) {
		cause := "device-list-mismatch"
		if readbackErr != nil {
			cause = readbackErr.Code
		}
		return nil, &siteError{Code: "mutation_unverified", Message: "设备绑定成功反馈已返回，但设备列表回读不一致", Details: map[string]any{"submitted": true, "confirmed": false, "cause": cause}}
	}
	return campusNetworkDeviceResult(operation, values), nil
}

func campusNetworkSetRepeatedFields(fields []pair, name string, values []string) []pair {
	result := make([]pair, 0, len(fields)+len(values))
	valueIndex := 0
	found := false
	for _, field := range fields {
		if field.name != name {
			result = append(result, field)
			continue
		}
		found = true
		value := ""
		if valueIndex < len(values) {
			value = values[valueIndex]
		}
		result = append(result, pair{name, value})
		valueIndex++
	}
	if !found {
		for _, value := range values {
			result = append(result, pair{name, value})
		}
	}
	return result
}

func sameCampusNetworkValues(want, got []string) bool {
	for len(got) < len(want) {
		got = append(got, "")
	}
	if len(got) > len(want) {
		got = got[:len(want)]
	}
	for index := range want {
		if strings.ToUpper(strings.TrimSpace(want[index])) != strings.ToUpper(strings.TrimSpace(got[index])) {
			return false
		}
	}
	return true
}

func (a NativeSite) campusNetworkPackageOptions(ctx context.Context, cookie string) (map[string]any, *siteError) {
	document, _, pageErr := a.campusNetworkPage(ctx, "/Self/nav_servicedefaultbook", cookie)
	if pageErr != nil {
		return nil, pageErr
	}
	data := map[string]any{}
	for _, field := range campusNetworkPackageFields {
		selectNode := document.first("select", field.selectID)
		if selectNode == nil {
			continue
		}
		choices := make([]map[string]any, 0)
		selected := ""
		for _, option := range selectNode.findAll("option") {
			label := strings.TrimSpace(pageDisplayText(option))
			if option.has("selected") {
				selected = label
			}
			choices = append(choices, map[string]any{"label": label, "value": option.attr("value")})
		}
		data[field.key] = map[string]any{"selected": selected, "choices": choices}
	}
	return campusNetworkResult("package-options", data), nil
}

func campusNetworkPackageChoice(document *pageNode, field campusNetworkPackageField, desired string) (string, string, *siteError) {
	selectNode := document.first("select", field.selectID)
	if selectNode == nil {
		return "", "", &siteError{Code: "protocol_unconfirmed", Message: "套餐页面缺少" + field.key + "选项"}
	}
	for _, option := range selectNode.findAll("option") {
		label := strings.TrimSpace(pageDisplayText(option))
		if (desired == "" && option.has("selected")) || (desired != "" && (strings.EqualFold(label, strings.TrimSpace(desired)) || option.attr("value") == strings.TrimSpace(desired))) {
			return option.attr("value"), label, nil
		}
	}
	return "", "", &siteError{Code: "invalid_argument", Message: field.flag + " 未找到可用选项: " + desired}
}

func (a NativeSite) campusNetworkReservePackage(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	document, responseURL, pageErr := a.campusNetworkPage(ctx, "/Self/nav_servicedefaultbook", cookie)
	if pageErr != nil {
		return nil, pageErr
	}
	form := document.first("form", "form1")
	if form == nil {
		return nil, &siteError{Code: "protocol_unconfirmed", Message: "套餐预约页面缺少表单"}
	}
	fields := pageFormFields(form, nil, document)
	labels := make([]string, 0, len(campusNetworkPackageFields))
	selections := map[string]string{}
	for _, field := range campusNetworkPackageFields {
		desired, _, valueErr := businessValue(args, field.flag)
		if valueErr != nil {
			return nil, valueErr
		}
		value, label, choiceErr := campusNetworkPackageChoice(document, field, desired)
		if choiceErr != nil {
			return nil, choiceErr
		}
		fields = setFormField(fields, field.name, value)
		labels = append(labels, label)
		selections[field.key] = label
	}
	fields = setFormField(fields, "name", strings.Join(labels, ""))
	if _, mutationErr := a.campusNetworkMutation(ctx, "/Self/selfservicebookAction.action", fields, responseURL, cookie); mutationErr != nil {
		return nil, mutationErr
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "server_success_feedback", "service": campusNetworkService, "operation": "package", "package": strings.Join(labels, ""), "selections": selections}, nil
}

func (a NativeSite) campusNetworkUpdateProfile(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	document, responseURL, pageErr := a.campusNetworkPage(ctx, "/Self/nav_changeUserInfo", cookie)
	if pageErr != nil {
		return nil, pageErr
	}
	form := document.first("form", "")
	if form == nil {
		return nil, &siteError{Code: "protocol_unconfirmed", Message: "个人资料页面缺少表单"}
	}
	fields := pageFormFields(form, nil, document)
	updates := []struct {
		flag, name, label string
	}{
		{"--phone", "user.flduserphone", "phone"},
		{"--email", "user.flduseremail", "email"},
		{"--install-address", "user.fldinstalllocal", "installation_address"},
		{"--anti-fraud", "user.tblregisteconfig.fldcheckcode", "anti_fraud"},
	}
	changed := make([]string, 0)
	requested := map[string]string{}
	for _, update := range updates {
		value, found, valueErr := businessValue(args, update.flag)
		if valueErr != nil {
			return nil, valueErr
		}
		if !found {
			continue
		}
		if strings.ContainsAny(value, "\r\n") {
			return nil, &siteError{Code: "invalid_argument", Message: update.flag + " 不能包含换行"}
		}
		fields = setFormField(fields, update.name, value)
		changed = append(changed, update.label)
		requested[update.label] = value
	}
	if len(changed) == 0 {
		return nil, &siteError{Code: "invalid_argument", Message: "update-profile 至少需要一个资料字段"}
	}
	if _, mutationErr := a.campusNetworkMutation(ctx, "/Self/ChangeUserMessageAction.action", fields, responseURL, cookie); mutationErr != nil {
		return nil, mutationErr
	}
	readback, _, readbackErr := a.campusNetworkPage(ctx, "/Self/nav_getUserInfo", cookie)
	if readbackErr != nil {
		return nil, &siteError{Code: "mutation_unverified", Message: "资料更新成功反馈已返回，但个人资料回读失败", Details: map[string]any{"submitted": true, "confirmed": false, "cause": readbackErr.Code}}
	}
	values := campusNetworkProfileValues(readback)
	for key, want := range requested {
		if values[key] != want {
			return nil, &siteError{Code: "mutation_unverified", Message: "资料更新成功反馈已返回，但回读结果不一致", Details: map[string]any{"submitted": true, "confirmed": false, "field": key}}
		}
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "server_success_and_profile_readback", "service": campusNetworkService, "operation": "update-profile", "updated": changed}, nil
}

func (a NativeSite) campusNetworkChangePassword(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	password, passwordErr := businessSecret(args, "--new-password", "CSUST_CAMPUS_NETWORK_NEW_PASSWORD")
	if passwordErr != nil {
		return nil, passwordErr
	}
	confirmation, found, valueErr := businessValue(args, "--password-confirm")
	if valueErr != nil {
		return nil, valueErr
	}
	if !found {
		confirmation = os.Getenv("CSUST_CAMPUS_NETWORK_PASSWORD_CONFIRM")
	}
	if confirmation == "" {
		return nil, &siteError{Code: "credentials_required", Message: "请使用 --password-confirm 或环境变量 CSUST_CAMPUS_NETWORK_PASSWORD_CONFIRM"}
	}
	if password != confirmation {
		return nil, &siteError{Code: "invalid_argument", Message: "新密码两次输入不一致"}
	}
	if len(password) < 8 || len(password) > 16 || strings.ContainsAny(password, "\r\n") {
		return nil, &siteError{Code: "invalid_argument", Message: "新密码必须是 8-16 位且不能包含换行"}
	}
	document, responseURL, pageErr := a.campusNetworkPage(ctx, "/Self/nav_changePsw", cookie)
	if pageErr != nil {
		return nil, pageErr
	}
	form := document.first("form", "ChangePswAction")
	if form == nil {
		return nil, &siteError{Code: "protocol_unconfirmed", Message: "修改密码页面缺少表单"}
	}
	fields := pageFormFields(form, nil, document)
	currentPassword := campusNetworkFormField(form, "user.flduserpassword")
	if currentPassword == nil || currentPassword.attr("value") == "" {
		return nil, &siteError{Code: "protocol_unconfirmed", Message: "修改密码页面缺少服务端当前密码字段"}
	}
	fields = setFormField(fields, "user.fldmd5hehai", password)
	fields = setFormField(fields, "user.fldextend", confirmation)
	if _, mutationErr := a.campusNetworkMutation(ctx, "/Self/ChangePswAction.action", fields, responseURL, cookie); mutationErr != nil {
		return nil, mutationErr
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "server_success_feedback", "service": campusNetworkService, "operation": "change-password"}, nil
}

func (a NativeSite) campusNetworkMutation(ctx context.Context, path string, data []pair, referer, cookie string) (map[string]any, *siteError) {
	headers := []pair{}
	if referer != "" {
		headers = append(headers, pair{"Referer", referer})
	}
	result, requestErr := businessRequest(ctx, campusNetworkService, "POST", path, nil, data, headers, businessRequestOptions{cookieFile: cookie, require: true, allowBusinessFailure: true}, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	state, known := campusNetworkResponseState(result)
	if !known {
		return nil, &siteError{Code: "mutation_unverified", Message: "校园网写请求已发送但没有成功反馈", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "unknown"}}
	}
	if !state {
		return nil, &siteError{Code: "mutation_rejected", Message: "校园网写请求被服务拒绝", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "server_failure"}}
	}
	return result, nil
}

func campusNetworkResponseState(result map[string]any) (bool, bool) {
	if value, ok := businessData(result); ok {
		return jsonBusinessState(value)
	}
	response, _ := result["response"].(map[string]any)
	contentType, _ := response["content_type"].(string)
	state, known, _ := businessState([]byte(businessBody(result)), contentType)
	return state, known
}
