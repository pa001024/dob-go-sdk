// wasm 计算 parity：Engine（内嵌 dob-calc-core）与真机期望逐项一致。
//
// 输入与纯 Go 链路同源（../python/tests/golden/）；wazero 为硬依赖、wasm 随包嵌入，
// 无需跳过。覆盖：7 线上 BD 的 target + 全量 attrs + 全部自定义变量（含 DOT），
// shard 42 表达式（含 DOT/治疗/条件 BUFF 路径）。
package dob

import (
	"sort"
	"strings"
	"testing"
)

func wasmOnlineEngines(t *testing.T) (map[string]*Engine, map[string]map[string]any) {
	t.Helper()
	tables := onlineTables(t)
	expected := loadGolden(t, "online_expected.json").(map[string]any)["expected"].(map[string]any)
	engines := map[string]*Engine{}
	payloads := map[string]map[string]any{}
	for _, file := range onlineFiles(t) {
		payload := loadGolden(t, "online/"+file).(map[string]any)
		key := strings.TrimSuffix(file, ".json")
		settings, _ := payload["settings"].(map[string]any)
		charID := int(Num(payload["charId"]))
		state, err := BuildState(charID, settings, tables, BuildOptions{})
		if err != nil {
			t.Fatal(err)
		}
		engines[key] = NewEngine(state, tables)
		payloads[key] = payload
		_ = expected
	}
	return engines, payloads
}

func wasmExpected(t *testing.T) map[string]any {
	t.Helper()
	return loadGolden(t, "online_expected.json").(map[string]any)["expected"].(map[string]any)
}

func TestWasmTargets(t *testing.T) {
	engines, _ := wasmOnlineEngines(t)
	expected := wasmExpected(t)
	keys := []string{}
	for k := range engines {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		want := expected[key].(map[string]any)
		assertClose(t, key+" target", engines[key].Calculate(""), Num(want["target"]))
	}
}

func TestWasmAttrs(t *testing.T) {
	engines, _ := wasmOnlineEngines(t)
	expected := wasmExpected(t)
	keys := []string{}
	for k := range engines {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		wantAttrs := AsMap(expected[key].(map[string]any)["attrs"])
		gotAttrs := engines[key].CalculateWeaponAttributes()
		for attr, wv := range wantAttrs {
			if attr == "weapon" || !IsNum(wv) {
				continue
			}
			if !isClose(Num(gotAttrs[attr]), Num(wv)) {
				t.Errorf("%s attrs.%s: wasm=%v ref=%v", key, attr, Num(gotAttrs[attr]), Num(wv))
			}
		}
	}
}

func TestWasmVariables(t *testing.T) {
	engines, _ := wasmOnlineEngines(t)
	expected := wasmExpected(t)
	keys := []string{}
	for k := range engines {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		for name, wv := range AsMap(expected[key].(map[string]any)["vars"]) {
			if !IsNum(wv) {
				continue
			}
			got := engines[key].Calculate(name)
			if !isClose(got, Num(wv)) {
				t.Errorf("%s var %s: wasm=%v ref=%v", key, name, got, Num(wv))
			}
		}
	}
}

func TestWasmShardExpressions(t *testing.T) {
	data := loadGolden(t, "shardBuild1_data.json").(map[string]any)
	raw := map[string]any{}
	for k, v := range map[string]any{"chars": data["chars"], "mods": data["mods"], "buffs": data["buffs"], "weapons": data["weapons"], "pets": data["pets"], "pet_entries": data["petEntries"]} {
		raw[k] = v
	}
	var effects []any
	for _, e := range AsList(data["effects"]) {
		if e != nil {
			effects = append(effects, e)
		}
	}
	raw["effects"] = effects
	var monsters []any
	for _, e := range AsList(data["monsters"]) {
		if e != nil {
			monsters = append(monsters, e)
		}
	}
	raw["monsters"] = monsters
	tables, err := TablesFromDict(raw)
	if err != nil {
		t.Fatal(err)
	}
	bd := loadGolden(t, "shardBuild1_bd.json").(map[string]any)
	settings, _ := bd["settings"].(map[string]any)
	state, err := BuildState(int(Num(bd["charId"])), settings, tables, BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(state, tables)
	expr := loadGolden(t, "shardBuild1_expr.json").(map[string]any)
	for _, c := range AsList(expr["cases"]) {
		cm := AsMap(c)
		got := engine.Calculate(S(cm, "expr"))
		if !isClose(got, Num(cm["ts"])) {
			t.Errorf("shard %s: wasm=%v ref=%v", S(cm, "expr"), got, Num(cm["ts"]))
		}
	}
	refAttrs := AsMap(expr["expectedAttrs"])
	gotAttrs := engine.CalculateWeaponAttributes()
	for attr, wv := range refAttrs {
		if attr == "weapon" || !IsNum(wv) {
			continue
		}
		if !isClose(Num(gotAttrs[attr]), Num(wv)) {
			t.Errorf("shard attrs.%s: wasm=%v ref=%v", attr, Num(gotAttrs[attr]), Num(wv))
		}
	}
}

func TestInlinePanels(t *testing.T) {
	// 内联 panels 采用逻辑：非空采用、缺失/空/非法回退专用端点。
	if inlinePanels(nil) != nil {
		t.Errorf("nil 结果应返回 nil")
	}
	if inlinePanels(map[string]any{}) != nil {
		t.Errorf("缺失 panels 应返回 nil")
	}
	if inlinePanels(map[string]any{"panels": map[string]any{}}) != nil {
		t.Errorf("空 panels 应返回 nil")
	}
	if inlinePanels(map[string]any{"panels": []any{}}) != nil {
		t.Errorf("非法 panels 应返回 nil")
	}
	src := map[string]any{"近战": map[string]any{"攻击": 1.0}, "远程": map[string]any{"攻击": 2.0}}
	got := inlinePanels(map[string]any{"panels": src})
	if len(got) != 2 || Num(got["近战"].(map[string]any)["攻击"]) != 1.0 {
		t.Errorf("非空 panels 应原样采用: %v", got)
	}
	// 采用的是拷贝：改动返回体不应影响源 envelope。
	got["近战"].(map[string]any)["攻击"] = 9.0
	if Num(src["近战"].(map[string]any)["攻击"]) != 1.0 {
		t.Errorf("应为深拷贝")
	}
}

func TestWasmColdAPIs(t *testing.T) {
	// 冷路径 API（面板/字段/等级）自洽：槽位互通、未知槽位/技能为空。
	tables := onlineTables(t)
	payload := loadGolden(t, "online/Svmqw3LGoY.json").(map[string]any)
	settings, _ := payload["settings"].(map[string]any)
	charID := int(Num(payload["charId"]))
	state, err := BuildState(charID, settings, tables, BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(state, tables)
	panel := engine.CharPanel()
	if len(panel) == 0 || Num(panel["攻击"]) <= 0 {
		t.Fatalf("角色面板异常")
	}
	for _, slot := range []string{"melee", "ranged", "skill"} {
		if engine.WeaponPanel(slot) == nil {
			t.Errorf("面板 %s 不应为空", slot)
		}
	}
	if engine.WeaponPanel("不存在的槽位") != nil {
		t.Errorf("未知槽位应返回 nil")
	}
	fields := engine.SkillFields("射击")
	if len(fields) == 0 {
		t.Fatalf("技能字段不应为空")
	}
	if len(engine.SkillFields("不存在的技能")) != 0 {
		t.Errorf("未知技能应返回空")
	}
	levels := engine.SkillLevelsFinal()
	if len(levels) == 0 {
		t.Fatalf("技能等级不应为空")
	}
}
