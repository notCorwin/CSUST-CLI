package adapter

import (
	"context"
	"fmt"
	"strings"
)

func (a NativeSite) runAcademicEvaluation(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 {
		return nil, &siteError{Code: "invalid_argument", Message: "academic evaluation 缺少子命令"}
	}
	operation := args[0]
	batchSelector, courseID, courseName, yes, suggestion, clearSuggestion := "", "", "", false, "", false
	answers := []pair{}
	for index := 1; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		if arg == "--json" {
			continue
		}
		if arg == "--yes" {
			yes = true
			continue
		}
		if arg == "--clear-suggestion" {
			clearSuggestion = true
			continue
		}
		if !inline {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return nil, &siteError{Code: "invalid_argument", Message: arg + " 缺少参数值"}
			}
			index++
			value = args[index]
		}
		switch arg {
		case "--batch":
			batchSelector = value
		case "--course-id":
			courseID = value
		case "--course-name":
			courseName = value
		case "--answer":
			item, err := splitPair(value, "--answer")
			if err != nil {
				return nil, err
			}
			answers = append(answers, item)
		case "--suggestion":
			suggestion = value
		default:
			return nil, &siteError{Code: "invalid_argument", Message: "academic evaluation 参数无效: " + arg}
		}
	}
	path := ""
	if operation != "batches" {
		var targetErr *siteError
		path, targetErr = a.academicEvaluationTarget(ctx, operation, batchSelector, courseID, courseName)
		if targetErr != nil {
			return nil, targetErr
		}
	}
	if (operation == "save" || operation == "submit") && !yes {
		return nil, &siteError{Code: "confirmation_required", Message: "保存或提交评价会修改账号数据，请加 --yes"}
	}

	if operation == "batches" {
		document, pageURL, err := a.academicEvaluationPage(ctx, "/jsxsd/xspj/xspj_find.do")
		if err != nil {
			return nil, err
		}
		return academicWrap(parseQualityEvaluationBatches(document, pageURL)), nil
	}
	document, pageURL, err := a.academicEvaluationPage(ctx, path)
	if err != nil {
		return nil, err
	}
	switch operation {
	case "courses":
		return academicWrap(parseQualityEvaluationCourses(document, pageURL)), nil
	case "form":
		form := parseQualityEvaluationForm(document, pageURL, false)
		questions, _ := form["questions"].([]map[string]any)
		if form["action"] == "" || form["read_only"] == true && len(questions) == 0 {
			return nil, &siteError{Code: "parse_error", Message: "未找到课程评价表"}
		}
		return academicWrap(form), nil
	case "save", "submit":
		form := parseQualityEvaluationForm(document, pageURL, true)
		if form["action"] == "" {
			return nil, &siteError{Code: "parse_error", Message: "未找到课程评价表"}
		}
		if form["read_only"] == true {
			return nil, &siteError{Code: "already_submitted", Message: "该评价已经提交，不能再修改"}
		}
		data, buildErr := qualityEvaluationFields(document, pageURL, answers, suggestion, clearSuggestion, operation)
		if buildErr != nil {
			return nil, buildErr
		}
		formAction := fmt.Sprint(form["action"])
		target, targetErr := validatePageActionTarget(formAction, pageURL, false)
		if targetErr != nil {
			return nil, targetErr
		}
		actionPath := target.EscapedPath()
		if target.RawQuery != "" {
			actionPath += "?" + target.RawQuery
		}
		result, submitErr := a.executeAcademicRequestWithRecovery(ctx, siteRequest{
			Service:      "academic",
			Path:         actionPath,
			Method:       "POST",
			CookieFile:   academicCookiePath(),
			Data:         data,
			Headers:      []pair{{"Referer", pageURL}},
			RequireLogin: true,
			Yes:          true,
		})
		if submitErr != nil {
			return nil, submitErr
		}
		result = sitePageResult(result)
		message := ""
		if response, ok := result["response"].(map[string]any); ok {
			message = fmt.Sprint(response["text"])
		}
		return academicWrap(map[string]any{
			"operation": operation,
			"message":   message,
			"request":   map[string]any{"method": "POST", "path": actionPath, "fields": fieldNames(data)},
			"response":  result["response"],
			"submitted": true,
			"confirmed": true,
			"evidence":  operation + "-response-confirmed",
		}), nil
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "未知评价子命令: " + operation}
	}
}

func (a NativeSite) academicEvaluationTarget(ctx context.Context, operation, batchSelector, courseID, courseName string) (string, *siteError) {
	body, pageURL, err := a.academicPage(ctx, "GET", "/jsxsd/xspj/xspj_find.do", nil, nil)
	if err != nil {
		return "", err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return "", &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	batchResult := parseQualityEvaluationBatches(document, pageURL)
	batches, _ := batchResult["items"].([]map[string]any)
	batch, selectErr := selectEvaluationItem(batches, batchSelector, "sequence", "semester", "category", "name")
	if selectErr != nil {
		return "", selectErr
	}
	batchPath, pathErr := academicPath(fmt.Sprint(batch["path"]))
	if pathErr != nil {
		return "", pathErr
	}
	if operation == "courses" {
		return batchPath, nil
	}
	courseBody, courseURL, courseErr := a.academicPage(ctx, "GET", batchPath, nil, nil)
	if courseErr != nil {
		return "", courseErr
	}
	courseDocument, courseParseErr := parsePage(courseBody)
	if courseParseErr != nil {
		return "", &siteError{Code: "parse_error", Message: courseParseErr.Error()}
	}
	courseResult := parseQualityEvaluationCourses(courseDocument, courseURL)
	courses, _ := courseResult["items"].([]map[string]any)
	selector := firstNonEmpty(courseID, courseName)
	course, courseSelectErr := selectEvaluationItem(courses, selector, "course_id", "course", "name")
	if courseSelectErr != nil {
		return "", courseSelectErr
	}
	return academicPath(fmt.Sprint(course["path"]))
}

func selectEvaluationItem(items []map[string]any, selector string, fields ...string) (map[string]any, *siteError) {
	selector = strings.TrimSpace(selector)
	if len(items) == 0 {
		return nil, &siteError{Code: "not_found", Message: "评价列表为空"}
	}
	if selector == "" {
		if len(items) == 1 {
			return items[0], nil
		}
		return nil, &siteError{Code: "ambiguous_target", Message: "评价目标不唯一，请提供 --batch 或 --course-id/--course-name"}
	}
	needle := strings.ToLower(selector)
	matches := make([]map[string]any, 0, 1)
	for _, item := range items {
		if fmt.Sprint(item["index"]) == selector {
			matches = append(matches, item)
			continue
		}
		for _, field := range fields {
			value := strings.ToLower(strings.TrimSpace(fmt.Sprint(item[field])))
			if value == needle || strings.Contains(value, needle) {
				matches = append(matches, item)
				break
			}
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return nil, &siteError{Code: "ambiguous_target", Message: "评价目标匹配多个结果，请使用序号或更具体的名称"}
	}
	return nil, &siteError{Code: "not_found", Message: "评价目标不存在: " + selector}
}

func (a NativeSite) academicEvaluationPage(ctx context.Context, path string) (*pageNode, string, *siteError) {
	body, pageURL, err := a.academicPage(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, "", err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, "", &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	return document, pageURL, nil
}
