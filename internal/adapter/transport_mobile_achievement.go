package adapter

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func transportMobileAchievementTypeArg(args []string, current string, required bool) (string, *siteError) {
	value, found, valueErr := businessValue(args, "--type")
	if valueErr != nil {
		return "", valueErr
	}
	if !found {
		value = current
	}
	value = strings.TrimSpace(value)
	if value == "" && required {
		return "", &siteError{Code: "invalid_argument", Message: "--type 必须提供 patent/software/paper/competition"}
	}
	switch strings.ToLower(value) {
	case "patent", "专利":
		return "专利", nil
	case "software", "软著", "software-copyright":
		return "软著", nil
	case "paper", "论文":
		return "论文", nil
	case "competition", "竞赛":
		return "竞赛", nil
	default:
		return "", &siteError{Code: "invalid_argument", Message: "--type 必须是 patent/专利、software/软著、paper/论文 或 competition/竞赛"}
	}
}

func transportMobileAchievementDateArg(args []string, current any, required bool) (time.Time, *siteError) {
	value, found, valueErr := businessValue(args, "--completed-at")
	if valueErr != nil {
		return time.Time{}, valueErr
	}
	if !found {
		if current == nil {
			value = ""
		} else {
			value = fmt.Sprint(current)
		}
	}
	value = strings.TrimSpace(value)
	if len(value) >= len("2006-01-02") {
		value = value[:len("2006-01-02")]
	}
	if value == "" && required {
		return time.Time{}, &siteError{Code: "invalid_argument", Message: "--completed-at 必须提供 YYYY-MM-DD 日期"}
	}
	parsed, parseErr := time.Parse("2006-01-02", value)
	if parseErr != nil {
		return time.Time{}, &siteError{Code: "invalid_argument", Message: "--completed-at 必须是 YYYY-MM-DD 日期"}
	}
	return parsed.UTC(), nil
}

func transportMobileAchievementDateText(value any) string {
	text := strings.TrimSpace(fmt.Sprint(value))
	if len(text) >= len("2006-01-02") {
		return text[:len("2006-01-02")]
	}
	return text
}

func transportMobileAchievementBoolArg(args []string, positive, negative string, current bool) (bool, *siteError) {
	if businessBool(args, positive) && businessBool(args, negative) {
		return false, &siteError{Code: "invalid_argument", Message: positive + " 与 " + negative + " 不能同时使用"}
	}
	if businessBool(args, positive) {
		return true, nil
	}
	if businessBool(args, negative) {
		return false, nil
	}
	return current, nil
}

func transportMobileAchievementStringSlice(value any) []string {
	result := []string{}
	add := func(item any) {
		text := strings.TrimSpace(fmt.Sprint(item))
		if text != "" {
			result = append(result, text)
		}
	}
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			add(item)
		}
	case []string:
		for _, item := range typed {
			add(item)
		}
	case nil:
	default:
		add(typed)
	}
	return result
}

func transportMobileAchievementStringSliceArg(args []string, flag string, current any, required bool) ([]string, *siteError) {
	values, valueErr := businessValues(args, flag)
	if valueErr != nil {
		return nil, valueErr
	}
	if len(values) == 0 {
		values = transportMobileAchievementStringSlice(current)
	} else {
		items := make([]string, 0, len(values))
		for _, value := range values {
			for _, item := range strings.Split(value, ",") {
				if item = strings.TrimSpace(item); item != "" {
					items = append(items, item)
				}
			}
		}
		values = items
	}
	if required && len(values) == 0 {
		return nil, &siteError{Code: "invalid_argument", Message: flag + " 至少提供一个值"}
	}
	return values, nil
}

func transportMobileAchievementPeople(value any, teacher bool) []map[string]any {
	result := []map[string]any{}
	for _, person := range transportMobileMaps(value) {
		department := transportMobileID(person["department"])
		item := map[string]any{
			"name":       transportMobileText(person, "name"),
			"code":       transportMobileText(person, "code"),
			"department": department,
		}
		if teacher {
			item["isExternal"] = transportMobileBool(person["isExternal"])
			if item["isExternal"] == true {
				item["department"] = nil
			}
		}
		result = append(result, item)
	}
	return result
}

func transportMobileAchievementPeopleArg(args []string, flag string, current any, teacher, required bool) ([]map[string]any, *siteError) {
	values, valueErr := businessValues(args, flag)
	if valueErr != nil {
		return nil, valueErr
	}
	if len(values) == 0 {
		people := transportMobileAchievementPeople(current, teacher)
		if required && len(people) == 0 {
			return nil, &siteError{Code: "invalid_argument", Message: flag + " 至少提供一人"}
		}
		return people, nil
	}
	people := make([]map[string]any, 0, len(values))
	for _, value := range values {
		parts := strings.Split(value, "|")
		if len(parts) < 3 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, &siteError{Code: "invalid_argument", Message: flag + " 格式必须是 姓名|学号或工号|学院编号；校外教师可用 姓名|工号||external"}
		}
		department := strings.TrimSpace(parts[2])
		external := teacher && len(parts) > 3 && strings.EqualFold(strings.TrimSpace(parts[3]), "external")
		if !external && department == "" {
			return nil, &siteError{Code: "invalid_argument", Message: flag + " 的学院编号不能为空；校外教师请追加 |external"}
		}
		item := map[string]any{"name": strings.TrimSpace(parts[0]), "code": strings.TrimSpace(parts[1]), "department": department}
		if teacher {
			item["isExternal"] = external
			if external {
				item["department"] = nil
			}
		}
		people = append(people, item)
	}
	return people, nil
}

func transportMobileAchievementPeopleEqual(left, right any, teacher bool) bool {
	leftPeople := transportMobileAchievementPeople(left, teacher)
	rightPeople := transportMobileAchievementPeople(right, teacher)
	if len(leftPeople) != len(rightPeople) {
		return false
	}
	for index := range leftPeople {
		for _, key := range []string{"name", "code"} {
			if fmt.Sprint(leftPeople[index][key]) != fmt.Sprint(rightPeople[index][key]) {
				return false
			}
		}
		if transportMobileID(leftPeople[index]["department"]) != transportMobileID(rightPeople[index]["department"]) {
			return false
		}
		if teacher && transportMobileBool(leftPeople[index]["isExternal"]) != transportMobileBool(rightPeople[index]["isExternal"]) {
			return false
		}
	}
	return true
}

func transportMobileAchievementEventID(value any) string {
	if event, ok := value.(map[string]any); ok {
		if key := transportMobileText(event, "key"); key != "" {
			return key
		}
	}
	return transportMobileID(value)
}

func transportMobileAchievementFiles(ctx context.Context, args []string, token, cookie string, current any) ([]string, int, *siteError) {
	fileIDs := transportMobileFinanceItemFileIDs(current)
	explicit, valueErr := businessValues(args, "--file-id")
	if valueErr != nil {
		return nil, 0, valueErr
	}
	if businessBool(args, "--clear-files") && len(explicit) > 0 {
		return nil, 0, &siteError{Code: "invalid_argument", Message: "--clear-files 不能与 --file-id 一起使用"}
	}
	if businessBool(args, "--clear-files") {
		fileIDs = []string{}
	} else if len(explicit) > 0 {
		fileIDs = []string{}
		for _, value := range explicit {
			for _, item := range strings.Split(value, ",") {
				if item = strings.TrimSpace(item); item != "" {
					fileIDs = append(fileIDs, item)
				}
			}
		}
	}
	uploaded, uploadErr := (NativeSite{}).transportMobileUploadFinanceFiles(ctx, args, token, cookie)
	if uploadErr != nil {
		return nil, 0, uploadErr
	}
	fileIDs = append(fileIDs, uploaded...)
	return fileIDs, len(uploaded), nil
}

func transportMobileAchievementMatches(row, entity map[string]any) bool {
	for _, key := range []string{"type", "name", "studentNo", "studentName", "no", "journal", "event", "award", "groupType", "studentType", "remark"} {
		left, right := transportMobileValue(row, key), entity[key]
		if key == "event" {
			if transportMobileAchievementEventID(left) != transportMobileAchievementEventID(right) {
				return false
			}
			continue
		}
		if transportMobileText(row, key) != strings.TrimSpace(fmt.Sprint(right)) {
			return false
		}
	}
	if transportMobileBool(row["isFirstAuthor"]) != transportMobileBool(entity["isFirstAuthor"]) || transportMobileBool(row["hasSpecialAward"]) != transportMobileBool(entity["hasSpecialAward"]) {
		return false
	}
	if transportMobileAchievementDateText(transportMobileValue(row, "dateDone")) != transportMobileAchievementDateText(entity["dateDone"]) {
		return false
	}
	if !transportMobileJSONEqual(transportMobileAchievementStringSlice(row["indexed"]), transportMobileAchievementStringSlice(entity["indexed"])) || !transportMobileJSONEqual(transportMobileFinanceItemFileIDs(row["file"]), transportMobileFinanceItemFileIDs(entity["file"])) {
		return false
	}
	return transportMobileAchievementPeopleEqual(row["student"], entity["student"], false) && transportMobileAchievementPeopleEqual(row["teacher"], entity["teacher"], true)
}

func (a NativeSite) transportMobileAchievementDetail(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	id, idErr := businessRequired(args, "--id", "achievement 必须提供 --id")
	if idErr != nil {
		return nil, idErr
	}
	token, tokenErr := a.transportMobileRequiredToken(args, cookie)
	if tokenErr != nil {
		return nil, tokenErr
	}
	row, payload, rowErr := a.transportMobileRecordRow(ctx, strings.TrimSpace(id), token, cookie, transportMobileTables["achievements"], "成果")
	if rowErr != nil {
		return nil, rowErr
	}
	result := transportMobileResult("achievement", "成果详情通过列表接口精确回读")
	result["id"], result["data"], result["achievement"], result["raw"] = strings.TrimSpace(id), transportMobileAchievement(row), transportMobileAchievement(row), redactSiteJSON(payload)
	return result, nil
}

func (a NativeSite) transportMobileAchievementSave(ctx context.Context, args []string, cookie, mode string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "保存交通移动端成果会改变远端数据，请加 --yes"}
	}
	token, tokenErr := a.transportMobileRequiredToken(args, cookie)
	if tokenErr != nil {
		return nil, tokenErr
	}
	var existing map[string]any
	id := ""
	if mode == "update" {
		id, tokenErr = businessRequired(args, "--id", "achievement-update 必须提供 --id")
		if tokenErr != nil {
			return nil, tokenErr
		}
		id = strings.TrimSpace(id)
		var rowErr *siteError
		existing, _, rowErr = a.transportMobileRecordRow(ctx, id, token, cookie, transportMobileTables["achievements"], "成果")
		if rowErr != nil {
			return nil, rowErr
		}
		if strings.TrimSpace(fmt.Sprint(existing["version"])) == "" {
			return nil, &siteError{Code: "protocol_unconfirmed", Message: "成果缺少并发版本号，无法安全修改"}
		}
	}

	currentType := transportMobileText(existing, "type")
	typeValue, typeErr := transportMobileAchievementTypeArg(args, currentType, mode == "create")
	if typeErr != nil {
		return nil, typeErr
	}
	if mode == "update" && currentType != "" && typeValue != currentType {
		return nil, &siteError{Code: "invalid_argument", Message: "成果类型在网页编辑流程中不可更换"}
	}
	name, nameErr := transportMobileStringArg(args, "--name", transportMobileText(existing, "name"), mode == "create")
	if nameErr != nil {
		return nil, nameErr
	}
	studentNo, studentNoErr := transportMobileStringArg(args, "--student-no", transportMobileText(existing, "studentNo"), typeValue != "竞赛")
	if studentNoErr != nil {
		return nil, studentNoErr
	}
	studentName, studentNameErr := transportMobileStringArg(args, "--student-name", transportMobileText(existing, "studentName"), typeValue != "竞赛")
	if studentNameErr != nil {
		return nil, studentNameErr
	}
	authorizationNumber, authorizationErr := transportMobileStringArg(args, "--authorization-number", transportMobileText(existing, "no"), typeValue == "专利" || typeValue == "软著")
	if authorizationErr != nil {
		return nil, authorizationErr
	}
	firstAuthor, firstAuthorErr := transportMobileAchievementBoolArg(args, "--first-author", "--not-first-author", transportMobileBool(existing["isFirstAuthor"]))
	if firstAuthorErr != nil {
		return nil, firstAuthorErr
	}
	journal, journalErr := transportMobileStringArg(args, "--journal", transportMobileText(existing, "journal"), typeValue == "论文")
	if journalErr != nil {
		return nil, journalErr
	}
	indexed, indexedErr := transportMobileAchievementStringSliceArg(args, "--index", existing["indexed"], typeValue == "论文")
	if indexedErr != nil {
		return nil, indexedErr
	}
	event, eventErr := transportMobileStringArg(args, "--event", transportMobileAchievementEventID(existing["event"]), typeValue == "竞赛")
	if eventErr != nil {
		return nil, eventErr
	}
	award, awardErr := transportMobileStringArg(args, "--award-level", transportMobileText(existing, "award"), typeValue == "竞赛")
	if awardErr != nil {
		return nil, awardErr
	}
	groupType, groupErr := transportMobileStringArg(args, "--group-type", transportMobileText(existing, "groupType"), false)
	if groupErr != nil {
		return nil, groupErr
	}
	if groupType == "" {
		groupType = "团队"
	}
	studentType, studentTypeErr := transportMobileStringArg(args, "--student-type", transportMobileText(existing, "studentType"), false)
	if studentTypeErr != nil {
		return nil, studentTypeErr
	}
	if studentType == "" {
		studentType = "本科生"
	}
	specialAward, specialAwardErr := transportMobileAchievementBoolArg(args, "--special-award", "--no-special-award", transportMobileBool(existing["hasSpecialAward"]))
	if specialAwardErr != nil {
		return nil, specialAwardErr
	}
	completedAt, completedAtErr := transportMobileAchievementDateArg(args, existing["dateDone"], true)
	if completedAtErr != nil {
		return nil, completedAtErr
	}
	remark, remarkErr := transportMobileStringArg(args, "--remark", transportMobileText(existing, "remark"), false)
	if remarkErr != nil {
		return nil, remarkErr
	}
	students, studentsErr := transportMobileAchievementPeopleArg(args, "--student", existing["student"], false, typeValue == "竞赛")
	if studentsErr != nil {
		return nil, studentsErr
	}
	teachers, teachersErr := transportMobileAchievementPeopleArg(args, "--teacher", existing["teacher"], true, typeValue == "竞赛")
	if teachersErr != nil {
		return nil, teachersErr
	}
	fileIDs, uploaded, filesErr := transportMobileAchievementFiles(ctx, args, token, cookie, existing["file"])
	if filesErr != nil {
		return nil, filesErr
	}
	if mode == "create" && len(fileIDs) == 0 {
		return nil, &siteError{Code: "invalid_argument", Message: "achievement-create 必须提供 --file 或 --file-id"}
	}

	entity := map[string]any{
		"type": typeValue, "name": name, "studentNo": studentNo, "studentName": studentName,
		"no": authorizationNumber, "isFirstAuthor": firstAuthor, "journal": journal, "indexed": indexed,
		"event": event, "award": award, "student": students, "teacher": teachers,
		"groupType": groupType, "studentType": studentType, "hasSpecialAward": specialAward,
		"dateDone": completedAt, "remark": remark, "file": fileIDs,
	}
	userID, userErr := a.transportMobileCurrentUserID(ctx, token, cookie)
	if userErr != nil {
		return nil, userErr
	}
	history := map[string]any{"user": userID, "date": time.Now().UTC()}
	if mode == "create" {
		entity["status"], entity["creater"], entity["dateCreate"], entity["code"] = "暂存", userID, time.Now().UTC(), "__auto__achievement"
		history["action"] = "新增成果信息"
		entity["history"] = history
	} else {
		entity["_id"], entity["version"] = id, existing["version"]
		history["action"] = "修改成果信息"
		entity["history"] = history
	}
	method, path := "POST", "/api/table/achievement"
	if mode == "update" {
		method, path = "PUT", "/api/table/achievement/"+url.PathEscape(id)
	}
	payload, requestErr := a.transportMobileCall(ctx, method, path, entity, true, token, cookie, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	if mode == "create" {
		data, ok := payload["data"].(map[string]any)
		if !ok {
			return nil, &siteError{Code: "mutation_unverified", Message: "成果创建成功反馈已返回，但缺少成果编号", Details: map[string]any{"submitted": true, "confirmed": false}}
		}
		id = transportMobileID(data)
		if id == "" {
			return nil, &siteError{Code: "mutation_unverified", Message: "成果创建成功反馈已返回，但缺少成果编号", Details: map[string]any{"submitted": true, "confirmed": false}}
		}
	}
	updated, _, readbackErr := a.transportMobileRecordRow(ctx, id, token, cookie, transportMobileTables["achievements"], "成果")
	if readbackErr != nil || !transportMobileAchievementMatches(updated, entity) {
		cause := "content-mismatch"
		if readbackErr != nil {
			cause = readbackErr.Code
		}
		return nil, &siteError{Code: "mutation_unverified", Message: "成果保存成功反馈已返回，但回读内容不一致", Details: map[string]any{"submitted": true, "confirmed": false, "id": id, "cause": cause}}
	}
	result := transportMobileResult("achievement-"+mode, "成果保存接口成功且内容回读一致")
	result["submitted"], result["id"], result["files_uploaded"], result["api_code"] = true, id, uploaded, payload["code"]
	return result, nil
}

func (a NativeSite) transportMobileAchievementStatus(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "更改交通移动端成果状态会改变远端数据，请加 --yes"}
	}
	id, idErr := businessRequired(args, "--id", "achievement-status 必须提供 --id")
	if idErr != nil {
		return nil, idErr
	}
	action, actionErr := businessRequired(args, "--action", "achievement-status 必须提供 --action")
	if actionErr != nil {
		return nil, actionErr
	}
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "submit", "提交":
		action = "submit"
	case "approve", "审核", "通过":
		action = "approve"
	case "reject", "驳回":
		action = "reject"
	case "withdraw", "撤回":
		action = "withdraw"
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "--action 必须是 submit/approve/reject/withdraw"}
	}
	reason, reasonErr := transportMobileStringArg(args, "--reason", "", action == "reject" || action == "withdraw")
	if reasonErr != nil {
		return nil, reasonErr
	}
	token, tokenErr := a.transportMobileRequiredToken(args, cookie)
	if tokenErr != nil {
		return nil, tokenErr
	}
	id = strings.TrimSpace(id)
	row, _, rowErr := a.transportMobileRecordRow(ctx, id, token, cookie, transportMobileTables["achievements"], "成果")
	if rowErr != nil {
		return nil, rowErr
	}
	version := strings.TrimSpace(fmt.Sprint(row["version"]))
	if version == "" {
		return nil, &siteError{Code: "protocol_unconfirmed", Message: "成果缺少并发版本号，无法修改状态"}
	}
	status := map[string]string{"submit": "已提交", "approve": "已审核", "reject": "暂存", "withdraw": "暂存"}[action]
	userID, userErr := a.transportMobileCurrentUserID(ctx, token, cookie)
	if userErr != nil {
		return nil, userErr
	}
	historyAction := fmt.Sprintf("更改状态由 [%s] 至 [%s]", transportMobileText(row, "status"), status)
	if action == "reject" || action == "withdraw" {
		historyAction = fmt.Sprintf("因%s 而%s，状态由 [%s] 至 [%s]", reason, map[string]string{"reject": "驳回", "withdraw": "撤回"}[action], transportMobileText(row, "status"), status)
	}
	body := map[string]any{"version": row["version"], "status": status, "history": map[string]any{"user": userID, "date": time.Now().UTC(), "action": historyAction}}
	payload, requestErr := a.transportMobileCall(ctx, "PUT", "/api/table/achievement/"+url.PathEscape(id), body, true, token, cookie, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	updated, _, readbackErr := a.transportMobileRecordRow(ctx, id, token, cookie, transportMobileTables["achievements"], "成果")
	if readbackErr != nil || transportMobileText(updated, "status") != status {
		cause := "status-mismatch"
		if readbackErr != nil {
			cause = readbackErr.Code
		}
		return nil, &siteError{Code: "mutation_unverified", Message: "成果状态保存成功反馈已返回，但状态回读不一致", Details: map[string]any{"submitted": true, "confirmed": false, "id": id, "cause": cause}}
	}
	result := transportMobileResult("achievement-status", "成果状态接口成功且状态回读一致")
	result["submitted"], result["id"], result["action"], result["status"], result["api_code"] = true, id, action, status, payload["code"]
	return result, nil
}
