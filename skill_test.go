// 技能等级 trio/溯源 + 面板 API 用例。
package dob

import (
	"testing"
)

func shardTables(t *testing.T) (*GameDataTables, map[string]any) {
	t.Helper()
	data := loadGolden(t, "shardBuild1_data.json").(map[string]any)
	bd := loadGolden(t, "shardBuild1_bd.json").(map[string]any)
	return tablesFor(t, data), bd
}

func charTraces(t *testing.T, tables *GameDataTables, bd map[string]any) []any {
	t.Helper()
	charID := int(Num(bd["charId"]))
	raw, ok := tables.CharByID[IDKey(charID)]
	if !ok {
		t.Fatalf("角色缺失")
	}
	return AsList(raw["溯源"])
}

func TestParseTraceBonus(t *testing.T) {
	tables, bd := shardTables(t)
	traces := charTraces(t, tables, bd)
	bonus := ParseTraceLevels(traces, 7)
	if bonus["以坚忍之名"] != 2 || bonus["我不忍啦！"] != 2 || bonus["哼！！"] != 2 {
		t.Errorf("溯源 7 解析错误: %v", bonus)
	}
	b5 := ParseTraceLevels(traces, 5)
	if len(b5) != len(bonus) {
		t.Errorf("溯源 5 应与 7 相同: %v", b5)
	}
	if len(ParseTraceLevels(traces, 2)) != 0 || len(ParseTraceLevels(traces, 0)) != 0 {
		t.Errorf("溯源 2/0 应为空")
	}
	b99 := ParseTraceLevels(traces, 99)
	if len(b99) != len(bonus) {
		t.Errorf("溯源 99 应钳制: %v", b99)
	}
	if len(ParseTraceLevels(traces, -3)) != 0 {
		t.Errorf("溯源 -3 应为空")
	}
}

func TestFinalLevelsTrioTraces(t *testing.T) {
	tables, bd := shardTables(t)
	settings, _ := bd["settings"].(map[string]any)
	state, err := BuildState(int(Num(bd["charId"])), settings, tables, BuildOptions{SkillLevels: []int{10, 10, 10}, HasLevels: true, Traces: 7, HasTraces: true})
	if err != nil {
		t.Fatalf("装配失败: %v", err)
	}
	byName := map[string]any{}
	for _, pair := range NewEngine(state, tables).SkillLevelsFinal() {
		byName[pair[0].(string)] = pair[1]
	}
	for _, name := range []string{"以坚忍之名", "我不忍啦！", "哼！！"} {
		if Num(byName[name]) != 12 {
			t.Errorf("%s 应为 12，实际 %v", name, byName[name])
		}
	}
}

func TestFinalLevelsClamp(t *testing.T) {
	tables, bd := shardTables(t)
	settings, _ := bd["settings"].(map[string]any)
	state, err := BuildState(int(Num(bd["charId"])), settings, tables, BuildOptions{SkillLevels: []int{11, 9, 8}, HasLevels: true, Traces: 7, HasTraces: true})
	if err != nil {
		t.Fatalf("装配失败: %v", err)
	}
	byName := map[string]any{}
	for _, pair := range NewEngine(state, tables).SkillLevelsFinal() {
		byName[pair[0].(string)] = pair[1]
	}
	if Num(byName["以坚忍之名"]) != 12 || Num(byName["我不忍啦！"]) != 11 || Num(byName["哼！！"]) != 10 {
		t.Errorf("钳制错误: %v", byName)
	}
}

func TestDefaultLevelsMatchTS(t *testing.T) {
	tables, bd := shardTables(t)
	settings, _ := bd["settings"].(map[string]any)
	state, err := BuildState(int(Num(bd["charId"])), settings, tables, BuildOptions{})
	if err != nil {
		t.Fatalf("装配失败: %v", err)
	}
	finals := NewEngine(state, tables).SkillLevelsFinal()
	for i := 0; i < 3 && i < len(finals); i++ {
		if Num(finals[i][1]) != 12 {
			t.Errorf("默认等级应为 12: %v", finals)
		}
	}
}

func settingsWith(settings map[string]any, overrides map[string]any) map[string]any {
	out := CloneMap(settings)
	for k, v := range overrides {
		out[k] = v
	}
	return out
}

func TestCharPanel(t *testing.T) {
	tables, bd := shardTables(t)
	point2 := loadGolden(t, "point2.json").(map[string]any)
	settings, _ := bd["settings"].(map[string]any)
	settings = settingsWith(settings, map[string]any{"charLevel": 70, "charSkillLevel": 10})
	state, err := BuildState(int(Num(bd["charId"])), settings, tables, BuildOptions{})
	if err != nil {
		t.Fatalf("装配失败: %v", err)
	}
	panel := NewEngine(state, tables).CharPanel()
	if _, ok := panel["weapon"]; ok {
		t.Errorf("char_panel 不应含 weapon")
	}
	for key, wv := range point2["attrs"].(map[string]any) {
		if key == "weapon" || !IsNum(wv) {
			continue
		}
		g, ok := panel[key]
		if !ok || !IsNum(g) || !isClose(Num(g), Num(wv)) {
			t.Errorf("char_panel.%s: mine=%v ref=%v", key, g, wv)
		}
	}
}

func TestWeaponPanel(t *testing.T) {
	tables, bd := shardTables(t)
	settings, _ := bd["settings"].(map[string]any)
	state, err := BuildState(int(Num(bd["charId"])), settings, tables, BuildOptions{})
	if err != nil {
		t.Fatalf("装配失败: %v", err)
	}
	engine := NewEngine(state, tables)
	standalone := engine.WeaponPanels()
	for _, pair := range [][2]string{{"melee", "近战"}, {"ranged", "远程"}, {"skill", "同律"}} {
		got := engine.WeaponPanel(pair[0])
		want, _ := standalone[pair[1]].(map[string]any)
		if !mapsEqual(got, want) {
			t.Errorf("weapon_panel(%s) 与上下文面板不一致", pair[0])
		}
	}
	if engine.WeaponPanel("不存在的槽位") != nil {
		t.Errorf("未知槽位应返回 nil")
	}
}

func TestSkillFields(t *testing.T) {
	tables, bd := shardTables(t)
	settings, _ := bd["settings"].(map[string]any)
	state, err := BuildState(int(Num(bd["charId"])), settings, tables, BuildOptions{SkillLevels: []int{10, 10, 10}, HasLevels: true, Traces: 7, HasTraces: true})
	if err != nil {
		t.Fatalf("装配失败: %v", err)
	}
	engine := NewEngine(state, tables)
	fields := engine.SkillFields("以坚忍之名")
	if len(fields) == 0 {
		t.Fatalf("技能字段为空")
	}
	found := false
	for _, f := range fields {
		if S(f, "名称") == "伤害" {
			found = true
			if !IsNum(f["值"]) {
				t.Errorf("伤害值非数值: %v", f["值"])
			}
		}
	}
	if !found {
		t.Errorf("缺少 伤害 字段")
	}
	if len(engine.SkillFields("不存在的技能")) != 0 {
		t.Errorf("未知技能应为空")
	}
}

func TestPoint2Target(t *testing.T) {
	tables, bd := shardTables(t)
	point2 := loadGolden(t, "point2.json").(map[string]any)
	settings, _ := bd["settings"].(map[string]any)
	settings = settingsWith(settings, map[string]any{"charLevel": 70, "charSkillLevel": 10})
	state, err := BuildState(int(Num(bd["charId"])), settings, tables, BuildOptions{})
	if err != nil {
		t.Fatalf("装配失败: %v", err)
	}
	got := NewEngine(state, tables).Calculate("")
	assertClose(t, "point2 target", got, Num(point2["target"]))
}
