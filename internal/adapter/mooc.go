package adapter

import (
	"context"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const moocService = "mooc"

var moocPageCountPattern = regexp.MustCompile(`page\.showPage\(\s*\d+\s*,\s*(\d+)`)

func (a NativeSite) executeMooc(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return businessCatalogFilter(moocService), nil
	}
	cookie, _, valueErr := businessValue(args, "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "courses", "search", "list":
		return a.moocCourses(ctx, args[1:], cookie)
	case "departments", "department":
		return a.moocDepartments(ctx, cookie)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "mooc 只支持 courses、departments、catalog"}
	}
}

func (a NativeSite) moocCourses(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	keyword, found, valueErr := businessValue(args, "--keyword")
	if valueErr != nil {
		return nil, valueErr
	}
	if !found {
		keyword, _, valueErr = businessValue(args, "--query")
		if valueErr != nil {
			return nil, valueErr
		}
	}
	departmentID, _, valueErr := businessValue(args, "--department-id")
	if valueErr != nil {
		return nil, valueErr
	}
	department, _, valueErr := businessValue(args, "--department")
	if valueErr != nil {
		return nil, valueErr
	}
	if strings.TrimSpace(department) != "" {
		departments, departmentErr := a.moocDepartmentMap(ctx, cookie)
		if departmentErr != nil {
			return nil, departmentErr
		}
		mappedID := departments[strings.TrimSpace(department)]
		if mappedID == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "--department 未找到该院系", Details: map[string]any{"department": department}}
		}
		departmentID = mappedID
	}
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	pageSize, pageSizeErr := businessInt(args, "--page-size", 10)
	if pageSizeErr != nil {
		return nil, pageSizeErr
	}
	sortValue := strings.ToLower(strings.TrimSpace(flagValue(args, "--sort")))
	sortName, sortParam := "created", "createTime"
	switch sortValue {
	case "", "created", "create-time", "date":
	case "views", "view-count", "clicks":
		sortName, sortParam = "views", "viewTimes"
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "--sort 只能是 created 或 views"}
	}
	order, orderName, orderErr := moocOrder(flagValue(args, "--order"))
	if orderErr != nil {
		return nil, orderErr
	}
	params := []pair{
		{"creatorUserId", "0"}, {"pageSize", strconv.Itoa(pageSize)}, {"keyword", strings.TrimSpace(keyword)},
		{"departmentId", firstNonEmpty(strings.TrimSpace(departmentID), "0")}, {"order", order}, {"sort", sortParam}, {"pageNum", strconv.Itoa(page)},
	}
	result, requestErr := a.businessGet(ctx, moocService, "/portal/courseNetwork/list", params, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	body, bodyErr := libraryRemoteBody(result)
	if bodyErr != nil {
		return nil, bodyErr
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "MOOC 本校课程页面解析失败: " + parseErr.Error()}
	}
	items := moocCourseItems(document)
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "MOOC 本校课程页面返回课程表和分页脚本",
		"service":  moocService, "operation": "courses", "keyword": strings.TrimSpace(keyword),
		"department": strings.TrimSpace(department), "department_id": firstNonEmpty(strings.TrimSpace(departmentID), "0"),
		"sort": sortName, "order": orderName, "page": page, "page_size": pageSize, "total_pages": moocPageCount(document), "data": items,
	}, nil
}

func moocOrder(value string) (string, string, *siteError) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "desc", "descending", "降序":
		return "0", "desc", nil
	case "asc", "ascending", "升序":
		return "1", "asc", nil
	default:
		return "", "", &siteError{Code: "invalid_argument", Message: "--order 只能是 asc 或 desc"}
	}
}

func (a NativeSite) moocDepartments(ctx context.Context, cookie string) (map[string]any, *siteError) {
	departments, err := a.moocDepartmentMap(ctx, cookie)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(departments))
	for name, id := range departments {
		items = append(items, map[string]any{"id": id, "name": name})
	}
	sort.Slice(items, func(i, j int) bool { return items[i]["name"].(string) < items[j]["name"].(string) })
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "MOOC 本校课程页面的院系筛选项", "service": moocService, "operation": "departments", "data": items,
	}, nil
}

func (a NativeSite) moocDepartmentMap(ctx context.Context, cookie string) (map[string]string, *siteError) {
	result, requestErr := a.businessGet(ctx, moocService, "/portal/courseNetwork/list", nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	body, bodyErr := libraryRemoteBody(result)
	if bodyErr != nil {
		return nil, bodyErr
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "MOOC 院系筛选项解析失败: " + parseErr.Error()}
	}
	departments := make(map[string]string)
	for _, list := range document.findAll("ul") {
		if list.attr("name") != "department" {
			continue
		}
		for _, item := range list.findAll("li") {
			name := strings.TrimSpace(pageDisplayText(item))
			id := strings.TrimSpace(item.attr("value"))
			if name != "" && id != "" && name != "全部" {
				departments[name] = id
			}
		}
	}
	if len(departments) == 0 {
		return nil, &siteError{Code: "parse_error", Message: "MOOC 页面缺少院系筛选项"}
	}
	return departments, nil
}

func moocCourseItems(document *pageNode) []map[string]any {
	table := libraryRemoteNodeWithClass(document, "Wmtable")
	if table == nil {
		return []map[string]any{}
	}
	items := make([]map[string]any, 0)
	for _, row := range table.findAll("tr") {
		if strings.Contains(" "+row.attr("class")+" ", " Wmtr1 ") {
			continue
		}
		cells := make([]*pageNode, 0, 7)
		for _, child := range row.children {
			if child.tag == "td" {
				cells = append(cells, child)
			}
		}
		if len(cells) < 7 {
			continue
		}
		id := strings.TrimSpace(pageDisplayText(cells[0]))
		if id == "" {
			continue
		}
		course := map[string]any{
			"id": id, "title": "", "department": strings.TrimSpace(pageDisplayText(cells[3])),
			"instructor": strings.TrimSpace(pageDisplayText(cells[4])), "views": moocNumber(pageDisplayText(cells[5])),
			"created_at":  strings.TrimSpace(pageDisplayText(cells[6])),
			"access_path": "/fyportal/tomoocportal?courseid=" + url.QueryEscape(id), "external_platform": "chaoxing",
		}
		if title := libraryRemoteNodeWithClass(cells[2], "Limitlogin"); title != nil {
			course["title"] = strings.TrimSpace(pageDisplayText(title))
		}
		if link := libraryRemoteNodeWithClass(cells[2], "Limitlogin"); link != nil && link.attr("href") != "" {
			course["public_access_url"] = link.attr("href")
		}
		items = append(items, course)
	}
	return items
}

func moocPageCount(document *pageNode) int {
	for _, script := range document.findAll("script") {
		if match := moocPageCountPattern.FindStringSubmatch(script.rawText()); len(match) > 1 {
			pages, _ := strconv.Atoi(match[1])
			return pages
		}
	}
	return 0
}

func moocNumber(value string) any {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return strings.TrimSpace(value)
	}
	return parsed
}
