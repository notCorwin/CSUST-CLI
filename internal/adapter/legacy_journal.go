package adapter

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

func (a NativeSite) executeLegacyHighwayJournal(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "home" {
		cookieArgs := args
		if len(cookieArgs) > 0 {
			cookieArgs = cookieArgs[1:]
		}
		cookie, _, valueErr := businessValue(cookieArgs, "--cookie-file")
		if valueErr != nil {
			return nil, valueErr
		}
		result, requestErr := a.businessGet(ctx, "journal-highway-legacy", "/", nil, businessRequestOptions{cookieFile: cookie})
		if requestErr != nil {
			return nil, requestErr
		}
		result = sitePageResult(result)
		result["service"], result["operation"] = "journal-highway-legacy", "home"
		return result, nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "issue":
		volume, issue, err := legacyJournalLocation(args[1:])
		if err != nil {
			return nil, err
		}
		return a.legacyJournalPage(ctx, "/journal/vol"+strconv.Itoa(volume)+"/iss"+strconv.Itoa(issue), "issue", cookie, map[string]any{"volume": volume, "issue": issue})
	case "article":
		volume, issue, err := legacyJournalLocation(args[1:])
		if err != nil {
			return nil, err
		}
		number, numberErr := businessInt(args[1:], "--article", 0)
		if numberErr != nil {
			return nil, numberErr
		}
		if number == 0 {
			return nil, &siteError{Code: "invalid_argument", Message: "article 必须提供 --article 文章序号"}
		}
		return a.legacyJournalPage(ctx, fmt.Sprintf("/journal/vol%d/iss%d/%d", volume, issue, number), "article", cookie, map[string]any{"volume": volume, "issue": issue, "article": number})
	case "search":
		query, queryErr := businessRequired(args[1:], "--query", "search 必须提供 --query")
		if queryErr != nil {
			return nil, queryErr
		}
		scope := firstNonEmpty(flagValue(args[1:], "--scope"), "journal")
		fq := `virtual_ancestor_link:"https://zwgl1980.csust.edu.cn/journal"`
		switch scope {
		case "journal":
		case "repository":
			fq = `virtual_ancestor_link:"https://zwgl1980.csust.edu.cn"`
		case "all":
			fq = ""
		default:
			return nil, &siteError{Code: "invalid_argument", Message: "--scope 只能是 journal、repository 或 all"}
		}
		params := []pair{{"q", query}}
		if fq != "" {
			params = append(params, pair{"fq", fq})
		}
		result, requestErr := a.businessGet(ctx, "journal-highway-legacy", "/do/search/", params, businessRequestOptions{cookieFile: cookie})
		if requestErr != nil {
			return nil, requestErr
		}
		result = sitePageResult(result)
		result["service"], result["operation"], result["query"], result["scope"] = "journal-highway-legacy", "search", query, scope
		return result, nil
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "旧版中外公路只支持 home、issue、article、search"}
	}
}

func legacyJournalLocation(args []string) (int, int, *siteError) {
	volume, volumeErr := businessInt(args, "--volume", 0)
	if volumeErr != nil {
		return 0, 0, volumeErr
	}
	if volume == 0 {
		return 0, 0, &siteError{Code: "invalid_argument", Message: "必须提供 --volume 卷号"}
	}
	issue, issueErr := businessInt(args, "--issue", 0)
	if issueErr != nil {
		return 0, 0, issueErr
	}
	if issue == 0 {
		return 0, 0, &siteError{Code: "invalid_argument", Message: "必须提供 --issue 期号"}
	}
	return volume, issue, nil
}

func (a NativeSite) legacyJournalPage(ctx context.Context, path, operation, cookie string, fields map[string]any) (map[string]any, *siteError) {
	result, requestErr := a.businessGet(ctx, "journal-highway-legacy", path, nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	body := businessBody(result)
	download := ""
	if document, parseErr := parsePage(body); parseErr == nil {
		for _, link := range document.findAll("a") {
			href := link.attr("href")
			if !strings.Contains(href, "cgi/viewcontent.cgi") {
				continue
			}
			base, _ := url.Parse("https://zwgl1980.csust.edu.cn")
			parsed, _ := url.Parse(href)
			download = base.ResolveReference(parsed).String()
			break
		}
	}
	result = sitePageResult(result)
	result["service"], result["operation"] = "journal-highway-legacy", operation
	for key, value := range fields {
		result[key] = value
	}
	if operation == "article" {
		links := map[string]string{"abstract": "https://zwgl1980.csust.edu.cn" + path}
		if download != "" {
			links["download"] = download
		}
		result["links"] = links
	}
	return result, nil
}
