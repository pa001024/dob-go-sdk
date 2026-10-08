// 在线快照用例（无网：httptest 假后端 + fixture 表）。
package dob

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func fixtureTables(t *testing.T) (*GameDataTables, map[string]any) {
	t.Helper()
	data := loadGolden(t, "shardBuild1_data.json").(map[string]any)
	bd := loadGolden(t, "shardBuild1_bd.json").(map[string]any)
	return tablesFor(t, data), bd
}

// fakeBuildServer 回放 build 查询的 GraphQL 假后端。
func fakeBuildServer(builds map[string]any) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		vars, _ := req["variables"].(map[string]any)
		id, _ := vars["id"].(string)
		var build any
		if b, ok := builds[id]; ok {
			build = b
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"build": build}})
	}))
}

func demoBuilds(t *testing.T, bd map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(AsMap(bd["settings"]))
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{
		"demo": map[string]any{
			"id": "demo", "title": "演示构筑", "charId": Num(bd["charId"]), "charSettings": string(raw),
		},
	}
}

func TestSnapshotFromOnline(t *testing.T) {
	tables, bd := fixtureTables(t)
	expr := loadGolden(t, "shardBuild1_expr.json").(map[string]any)
	srv := fakeBuildServer(demoBuilds(t, bd))
	defer srv.Close()
	backend := NewBackendClient(srv.URL, 0)
	snapshot, err := SnapshotFromOnline("demo", SnapshotOptions{Backend: backend, Source: tables})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Bdid != "demo" || snapshot.Title != "演示构筑" {
		t.Errorf("快照元信息: %v %v", snapshot.Bdid, snapshot.Title)
	}
	var target float64
	for _, c := range expr["cases"].([]any) {
		if cm, ok := c.(map[string]any); ok && cm["expr"] == S(AsMap(bd["settings"]), "targetFunction") {
			target = Num(cm["ts"])
		}
	}
	assertClose(t, "target", snapshot.Calculate(""), target)
	assertClose(t, "attrs.攻击", Num(snapshot.Attrs()["攻击"]), Num(AsMap(expr["expectedAttrs"])["攻击"]))
	if got := snapshot.Evaluate("10 + 4 * 5"); got != 30 {
		t.Errorf("evaluate: %v", got)
	}
	panel := snapshot.CharPanel()
	if _, ok := panel["weapon"]; ok {
		t.Errorf("char_panel 不应含 weapon")
	}
	if _, ok := panel["攻击"]; !ok {
		t.Errorf("char_panel 缺攻击")
	}
	if len(snapshot.SkillLevelsFinal()) != 5 {
		t.Errorf("技能数: %d", len(snapshot.SkillLevelsFinal()))
	}
	summary := snapshot.Summary()
	if summary["bdid"] != "demo" {
		t.Errorf("summary: %v", summary)
	}
	assertClose(t, "summary.targetValue", Num(summary["targetValue"]), target)
}

func TestSnapshotFromSettings(t *testing.T) {
	tables, bd := fixtureTables(t)
	settings, _ := bd["settings"].(map[string]any)
	snapshot, err := SnapshotFromSettings(int(Num(bd["charId"])), settings, tables, BuildOptions{
		SkillLevels: []int{10, 10, 10}, HasLevels: true, Traces: 7, HasTraces: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]float64{}
	for _, pair := range snapshot.SkillLevelsFinal() {
		byName[pair[0].(string)] = Num(pair[1])
	}
	if byName["以坚忍之名"] != 12 {
		t.Errorf("got %v", byName)
	}
	if snapshot.WeaponPanel("melee") == nil {
		t.Errorf("melee 面板缺失")
	}
	if snapshot.WeaponPanel("不存在") != nil {
		t.Errorf("未知槽位应为 nil")
	}
	if len(snapshot.SkillFields("不存在的技能")) != 0 {
		t.Errorf("未知技能应为空")
	}
}

func TestSnapshotMissingBuild(t *testing.T) {
	tables, _ := fixtureTables(t)
	srv := fakeBuildServer(map[string]any{})
	defer srv.Close()
	_, err := SnapshotFromOnline("nope", SnapshotOptions{Backend: NewBackendClient(srv.URL, 0), Source: tables})
	if err == nil {
		t.Errorf("缺失构筑应报错")
	}
}

func TestSnapshotBadSettings(t *testing.T) {
	tables, _ := fixtureTables(t)
	srv := fakeBuildServer(map[string]any{"bad": map[string]any{"id": "bad", "charId": 1501, "charSettings": "{oops"}})
	defer srv.Close()
	_, err := SnapshotFromOnline("bad", SnapshotOptions{Backend: NewBackendClient(srv.URL, 0), Source: tables})
	if err == nil {
		t.Errorf("坏设置应报错")
	}
}
