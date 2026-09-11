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

func TestCampusMapQueriesUseLiveContractsAndSemanticModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/cmccr-server/center/store/batch/query":
			_, _ = fmt.Fprint(writer, `{"code":0,"data":[{"contentValue":{"map_token":"test-map-token"}}]}`)
		case "/cmgis-server/map/v2/zone/page":
			_, _ = fmt.Fprint(writer, `{"code":200,"data":{"content":[{"id":1,"name":"云塘校区","center":"113,28","defaultZoom":16,"is2D":false}]}}`)
		case "/cmips-server/situationalIntelligence/publicPointType/queryListWithDisplayParentCode":
			if _, ok := request.URL.Query()["parentCodes"]; ok {
				_, _ = fmt.Fprint(writer, `{"code":200,"data":[{"typeCode":137,"parentCode":103,"typeName":"停车场","mapPointCount":1,"search":true,"display":true}]}`)
			} else {
				_, _ = fmt.Fprint(writer, `{"code":200,"data":[{"typeCode":103,"typeName":"交通服务"}]}`)
			}
		case "/cmips-server/situationalIntelligence/publicPoint/queryAllByTypeCodesCampusCode":
			if request.URL.Query().Get("typeCodes") != "137" {
				t.Errorf("public-point type mapping failed: %v", request.URL.Query())
			}
			_, _ = fmt.Fprint(writer, `{"code":200,"data":[{"pointCode":1,"typeCode":137,"pointName":"停车场","campusCode":3,"rasterLngLatString":"113.1,28.2","mapPointType":{"typeCode":137,"parentCode":103,"typeName":"停车场"}}]}`)
		case "/cmips-server/situationalIntelligence/loadPublicPointDetail/1":
			_, _ = fmt.Fprint(writer, `{"code":200,"data":{"pointCode":1,"typeCode":137,"pointName":"停车场","campusCode":3,"rasterLngLatString":"113.1,28.2","mapPointImgList":[{"imgId":1,"imgUrl":"/img.jpg"}]}}`)
		case "/cmips-server/roam/listQuery":
			_, _ = fmt.Fprint(writer, `{"code":200,"data":[{"roamId":4,"roamName":"全景","roamType":2,"campusCode":3,"campusName":"云塘校区","location":"理科楼","roamnUrl":"https://example/roam","lngLat":"{\"coordinates\":[113,28],\"type\":\"Point\"}"}]}`)
		case "/cmips-server/situationalIntelligence/publicPointType/countListWithDisplayParentCode":
			_, _ = fmt.Fprint(writer, `{"code":200,"data":{"totalPoint":1,"totalThematicInfo":0}}`)
		case "/cmgis-server/map/v2/search":
			if request.Header.Get("Authorization") != "Basic test-map-token" {
				t.Errorf("map search token was not applied internally: %q", request.Header.Get("Authorization"))
			}
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body["keywords"] != "图书馆" || body["zoneid"] != float64(1) {
				t.Errorf("unexpected map search body: %#v, %v", body, err)
			}
			_, _ = fmt.Fprint(writer, `{"code":200,"data":{"list":[{"id":"1712","name":"图书馆","systemType":"map","category":["图书馆"],"center":{"type":"Point","coordinates":[113,28]},"content":"主楼"}],"total":1}}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "campus-map.cookies.txt"))

	zones := runIssueJSON(t, "campus-map", "zones")
	if zones["total"] != float64(1) || zones["zones"].([]any)[0].(map[string]any)["name"] != "云塘校区" {
		t.Fatalf("zone model failed: %#v", zones)
	}
	types := runIssueJSON(t, "campus-map", "types", "--campus", "云塘")
	if types["total"] != float64(2) || types["campus"] != "云塘校区" {
		t.Fatalf("type model failed: %#v", types)
	}

	points := runIssueJSON(t, "campus-map", "points", "--campus", "云塘", "--type", "停车场")
	point := points["data"].([]any)[0].(map[string]any)
	if points["total"] != float64(1) || point["id"] != float64(1) || point["coordinates"].([]any)[0] != float64(113.1) {
		t.Fatalf("point model failed: %#v", points)
	}

	detail := runIssueJSON(t, "campus-map", "point", "--id", "1")
	detailPoint := detail["point"].(map[string]any)
	if detailPoint["images"].([]any)[0].(map[string]any)["url"] != "/img.jpg" {
		t.Fatalf("point detail model failed: %#v", detail)
	}

	search := runIssueJSON(t, "campus-map", "search", "--campus", "云塘", "--keyword", "图书馆")
	searchItem := search["data"].([]any)[0].(map[string]any)
	if searchItem["name"] != "图书馆" || strings.Contains(string(mustMarshalIssue(search)), "test-map-token") {
		t.Fatalf("search model or token handling failed: %#v", search)
	}

	panoramas := runIssueJSON(t, "campus-map", "panoramas", "--campus", "云塘", "--kind", "panorama")
	if panoramas["data"].([]any)[0].(map[string]any)["coordinates"].([]any)[1] != float64(28) {
		t.Fatalf("panorama model failed: %#v", panoramas)
	}
	stats := runIssueJSON(t, "campus-map", "stats", "--campus", "云塘")
	if stats["total_points"] != float64(1) || stats["total_thematic"] != float64(0) {
		t.Fatalf("map stats model failed: %#v", stats)
	}
}
