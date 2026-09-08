package adapter

import "testing"

func TestTextbookParserSelectsOneRowAndAction(t *testing.T) {
	source := `<form action="/save" method="post"><input type="hidden" name="token" value="server-token"><table id="jccx"><tr><th>课程</th><th>教材</th><th>状态</th><th>操作</th></tr><tr><td>线代</td><td>教材 A</td><td>未订</td><td><input type="hidden" name="id" value="7"><button name="op" value="subscribe">选订</button></td></tr></table></form>`
	document, err := parsePage(source)
	if err != nil {
		t.Fatal(err)
	}
	items, parseErr := parseTextbookPage(document, "http://xk.csust.edu.cn/jsxsd/nxsjc/jccx")
	if parseErr != nil || len(items) != 1 {
		t.Fatalf("unexpected textbook items: %#v %v", items, parseErr)
	}
	if items[0]["course"] != "线代" || items[0]["title"] != "教材 A" || items[0]["status"] != "未订" {
		t.Fatalf("unexpected item: %#v", items[0])
	}
	if actions, ok := items[0]["actions"].([]string); !ok || len(actions) != 1 || actions[0] != "subscribe" {
		t.Fatalf("unexpected actions: %#v", items[0]["actions"])
	}
	row := textbookRowsPage(document)[0]
	action := textbookActionNode(row, "subscribe")
	if action == nil {
		t.Fatal("subscribe action was not found")
	}
	fields := textbookFormFields(pageFormOwner(row, document), row, action, document)
	if len(fields) != 3 || fields[0].name != "token" || fields[1].name != "id" || fields[2].name != "op" {
		t.Fatalf("unexpected fields: %#v", fields)
	}
}
