package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestEquipmentProtocolAndSemanticModels(t *testing.T) {
	if encrypted, err := equipmentEncrypt("{}"); err != nil || encrypted != "3IHn7dNdev0=" {
		t.Fatalf("unexpected equipment DES payload: %q %v", encrypted, err)
	}

	var listPlain, detailPlain, favoritePlain string
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != equipmentEndpoint || request.Method != http.MethodPost {
			http.NotFound(writer, request)
			return
		}
		var body struct {
			DESJSON string `json:"DESJson"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("invalid equipment envelope: %v", err)
			return
		}
		auth, err := equipmentDecrypt(request.Header.Get("wxAuthorization"))
		if err != nil {
			t.Errorf("invalid equipment authorization: %v", err)
			return
		}
		var action struct {
			Function  string `json:"f"`
			Component string `json:"c"`
		}
		if err := json.Unmarshal([]byte(auth), &action); err != nil {
			t.Errorf("invalid equipment action: %v", err)
			return
		}
		plain, err := equipmentDecrypt(body.DESJSON)
		if err != nil {
			t.Errorf("invalid equipment request: %v", err)
			return
		}
		calls = append(calls, action.Function)
		if action.Function != "GetUIToken" && request.Header.Get("UIToken") != "test-token" {
			t.Errorf("missing UIToken for %s", action.Function)
		}
		switch action.Function {
		case "GetUIToken":
			writeEquipmentResponse(t, writer, map[string]any{"token": "test-token"})
		case "GetIndexDevBm":
			writeEquipmentResponse(t, writer, map[string]any{
				"BM":  []any{map[string]any{"bmid": "118", "name": "交通学院"}},
				"SYS": []any{map[string]any{"sysid": "22", "sysmc": "测量实验室"}},
				"GB":  []any{"中国"}, "GZRQ": []any{"2024"}, "FL": []any{}, "XKLY": []any{"交通运输工程"}, "CNAME": []any{},
			})
		case "GetDevListCols":
			writeEquipmentResponse(t, writer, []any{map[string]any{"ZDMC": "仪器", "COLNAME": "YQMC", "TEMPNAME": ""}})
		case "GetApparatusList_Nei":
			listPlain = plain
			writeEquipmentResponse(t, writer, map[string]any{
				"code": "0", "msg": "", "count": "1", "allcount": "1",
				"data": []any{map[string]any{
					"ZCBH": "EQ-1", "YQMC": "压力老化仪", "YQXH": "PR9300", "YQGG": "台",
					"SCCJ": "厂商", "GBMC": "中国", "SSBMID": "118", "SSBMMC": "交通学院",
					"SSSYSID": "22", "SSSYSMC": "测量实验室", "XZMC1": "在用", "XZMC": "空闲",
					"OpenState": "1", "zzname": "自主上机", "syname": "送样检测", "custom": "kept",
				}},
			})
		case "GetApparatusOne":
			detailPlain = plain
			writeEquipmentResponse(t, writer, []any{map[string]any{
				"ZCBH": "EQ-1", "YQMC": "压力老化仪", "YQXH": "PR9300", "YQGG": "台",
				"SCCJ": "厂商", "GBMC": "中国", "SSBMMC": "交通学院", "SSSYSMC": "测量实验室",
				"XNZB": "性能", "ZYYY": "应用", "YPYQ": "样品要求", "YQSM": "说明",
			}})
		case "AddDevsCollect":
			favoritePlain = plain
			writeEquipmentResponse(t, writer, map[string]any{"flag": "0", "msg": "收藏成功！"})
		default:
			t.Errorf("unexpected equipment action %s/%s with %s", action.Function, action.Component, plain)
			writeEquipmentResponse(t, writer, map[string]any{"code": "1", "msg": "unknown"})
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	list := runIssueJSON(t, "equipment", "list", "--keyword", "压力", "--department-id", "118", "--lab-id", "22", "--category-id", "10000", "--discipline", "交通运输工程", "--year", "2024", "--year-to", "2023", "--page", "2", "--page-size", "25", "--all")
	if list["service"] != "equipment" || list["operation"] != "list" || list["total"] != float64(1) {
		t.Fatalf("unexpected equipment list result: %#v", list)
	}
	items, ok := list["data"].([]any)
	if !ok || len(items) != 1 || items[0].(map[string]any)["id"] != "EQ-1" || items[0].(map[string]any)["availability"] != "空闲" {
		t.Fatalf("semantic instrument model missing: %#v", list["data"])
	}
	if !strings.Contains(listPlain, `"txtkey":"压力"`) || !strings.Contains(listPlain, `"SSBMID":"118"`) || !strings.Contains(listPlain, `"sssysid":"22"`) || !strings.Contains(listPlain, `"catagoryid":"10000"`) || !strings.Contains(listPlain, `"xkly":"交通运输工程"`) || !strings.Contains(listPlain, `"grrq":"2024"`) || !strings.Contains(listPlain, `"grrqa":"2023"`) || !strings.Contains(listPlain, `"isall":"1"`) || !strings.Contains(listPlain, "page:'2'") || !strings.Contains(listPlain, "limit:'25'") {
		t.Fatalf("semantic filters were not mapped to the live payload: %s", listPlain)
	}
	if _, ok := list["raw"].(map[string]any); !ok {
		t.Fatalf("raw equipment response was not preserved: %#v", list["raw"])
	}

	filters := runIssueJSON(t, "equipment", "filters")
	filterData := filters["data"].(map[string]any)
	if len(filterData["columns"].([]any)) != 1 || len(filterData["filters"].(map[string]any)["BM"].([]any)) != 1 {
		t.Fatalf("equipment dictionaries were not returned: %#v", filters)
	}

	detail := runIssueJSON(t, "equipment", "detail", "--id", "EQ-1")
	if detail["instrument"].(map[string]any)["name"] != "压力老化仪" || !strings.Contains(detailPlain, "ZCBH:'EQ-1'") || !strings.Contains(detailPlain, `"lang":"zh-CN"`) {
		t.Fatalf("equipment detail was not mapped: %#v plain=%s", detail, detailPlain)
	}
	favorite := runIssueJSON(t, "equipment", "favorite", "--id", "EQ-1", "--yes")
	if favorite["submitted"] != true || favorite["confirmed"] != true || favorite["favorite"] != true || !strings.Contains(favoritePlain, "YQBH:'EQ-1'") {
		t.Fatalf("equipment favorite was not confirmed: %#v plain=%s", favorite, favoritePlain)
	}
	for _, action := range []string{"GetUIToken", "GetApparatusList_Nei", "GetIndexDevBm", "GetDevListCols", "GetApparatusOne", "AddDevsCollect"} {
		if !containsString(calls, action) {
			t.Fatalf("equipment action was not called: %s calls=%v", action, calls)
		}
	}
}

func writeEquipmentResponse(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	plain, err := json.Marshal(value)
	if err != nil {
		t.Errorf("marshal equipment response: %v", err)
		return
	}
	encrypted, err := equipmentEncrypt(string(plain))
	if err != nil {
		t.Errorf("encrypt equipment response: %v", err)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(writer, `{"d":%q}`, encrypted)
}
