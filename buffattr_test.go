// BUFF 结构化 attr 与 tag 条件单测（新格式 attr 子表 + 旧格式顶层折叠）。
package dob

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestBuffAttrStaticsNewShape(t *testing.T) {
	raw := map[string]any{
		"名称": "测试",
		"mx": 2,
		"attr": map[string]any{
			"攻击":    0.5,
			"暴击":    []any{0.7, 0.2},
			"技能伤害":  "0.8*技能威力",
			"@近战增伤": 0.6,
		},
	}
	inst := LevelBuff(raw, 1, 1, 1)
	if got := Num(inst["攻击"]); got != 0.5 {
		t.Errorf("静态数值: got %v want 0.5", got)
	}
	if got := Num(inst["暴击"]); got != 0.7 {
		t.Errorf("数组档位: got %v want 0.7", got)
	}
	if _, ok := inst["技能伤害"]; ok {
		t.Errorf("动态表达式不应进实例值表")
	}
	if got := Num(inst["近战增伤"]); got != 0.6 {
		t.Errorf("@ 前缀应剥离: got %v want 0.6", got)
	}
	if _, ok := inst["@近战增伤"]; ok {
		t.Errorf("不应保留 @ 原键")
	}
	if !BuffIsLayerProp(raw, "近战增伤") {
		t.Errorf("@ 层判定失败")
	}
	if BuffIsLayerProp(raw, "攻击") {
		t.Errorf("非 @ 键误判")
	}
}

func TestBuffAttrOldShapeFold(t *testing.T) {
	raw := map[string]any{"名称": "旧", "mx": 2, "攻击": 0.5, "暴击": []any{0.1, 0.2}}
	inst := LevelBuff(raw, 1, 1, 1)
	if got := Num(inst["攻击"]); got != 0.5 {
		t.Errorf("旧顶层数值折叠: got %v want 0.5", got)
	}
	if got := Num(inst["暴击"]); got != 0.1 {
		t.Errorf("旧顶层数组折叠: got %v want 0.1", got)
	}
	if !BuffIsLayerProp(map[string]any{"@近战增伤": 0.6}, "近战增伤") {
		t.Errorf("旧顶层 @ 判定失败")
	}
}

func TestBuffAttrPrecedence(t *testing.T) {
	raw := map[string]any{"名称": "冲突", "攻击": 0.1, "attr": map[string]any{"攻击": 0.5}}
	inst := LevelBuff(raw, 1, 1, 1)
	if got := Num(inst["攻击"]); got != 0.5 {
		t.Errorf("attr 应优先于顶层: got %v want 0.5", got)
	}
}

func TestLevelBuffCopiesTag(t *testing.T) {
	raw := map[string]any{"名称": "充盈条件", "tag": []any{"充盈"}, "attr": map[string]any{"独立增伤": 0.2}}
	inst := LevelBuff(raw, 1, 1, 1)
	tags, ok := inst["tag"].([]any)
	if !ok || len(tags) != 1 || tags[0] != "充盈" {
		t.Fatalf("tag 未复制: %v", inst["tag"])
	}
}

var buffShapeMeta = map[string]bool{
	"id": true, "名称": true, "描述": true, "限定": true, "品质": true, "版本": true,
	"a": true, "b": true, "dx": true, "lx": true, "mx": true, "code": true,
	"技能": true, "tag": true, "attr": true, "display": true,
}

// toNewBuffShape 把旧格式行（顶层直挂数值）转成新格式（数值进 attr），复刻 TS 迁移脚本。
func toNewBuffShape(row map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range row {
		out[k] = v
	}
	attr := map[string]any{}
	if a, ok := row["attr"].(map[string]any); ok && a != nil {
		for k, v := range a {
			attr[k] = v
		}
	}
	for k, v := range row {
		if buffShapeMeta[k] {
			continue
		}
		if _, dup := attr[k]; dup {
			delete(out, k)
			continue
		}
		if IsNum(v) {
			attr[k] = v
			delete(out, k)
			continue
		}
		if l, ok := v.([]any); ok {
			allNum := true
			for _, e := range l {
				if !IsNum(e) {
					allNum = false
					break
				}
			}
			if allNum {
				attr[k] = v
				delete(out, k)
			}
		}
	}
	if len(attr) > 0 {
		out["attr"] = attr
	}
	return out
}

func convertBuffTable(list []any) []any {
	out := make([]any, 0, len(list))
	for _, e := range list {
		if m, ok := e.(map[string]any); ok && m != nil {
			out = append(out, toNewBuffShape(m))
			continue
		}
		out = append(out, e)
	}
	return out
}

// TestNewShapeParity 碎片 BD 在新格式表下 42 表达式与 golden `ts` 逐位一致。
func TestNewShapeParity(t *testing.T) {
	bd := loadGolden(t, "shardBuild1_bd.json").(map[string]any)
	data := loadGolden(t, "shardBuild1_data.json").(map[string]any)
	expr := loadGolden(t, "shardBuild1_expr.json").(map[string]any)
	data2 := map[string]any{}
	for k, v := range data {
		data2[k] = v
	}
	data2["buffs"] = convertBuffTable(AsList(data["buffs"]))
	data2["effects"] = convertBuffTable(AsList(data["effects"]))
	tables := tablesFor(t, data2)
	charID := int(Num(bd["charId"]))
	settings, _ := bd["settings"].(map[string]any)
	state, err := BuildState(charID, settings, tables, BuildOptions{})
	if err != nil {
		t.Fatalf("装配失败: %v", err)
	}
	engine := NewEngine(state, tables)
	cases := expr["cases"].([]any)
	for _, c := range cases {
		cm := c.(map[string]any)
		name, _ := cm["expr"].(string)
		want := Num(cm["ts"])
		got := engine.Calculate(name)
		if !isClose(got, want) {
			t.Errorf("表达式 %s: mine=%v ref=%v", name, got, want)
		}
	}
}

func fullTablesVer(t *testing.T, version string) *GameDataTables {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	mock := filepath.Join(root, "mock", "data-pack", version+".zip")
	if _, err := os.Stat(mock); err != nil {
		t.Skipf("mock 包缺失: %v", err)
	}
	dir := t.TempDir()
	dest := filepath.Join(dir, version)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	// 硬链接复用只读包，避免大文件拷贝。
	if err := os.Link(mock, filepath.Join(dest, "package.zip")); err != nil {
		raw, err := os.ReadFile(mock)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dest, "package.zip"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pack := NewDataPackStore(dir, "", "-")
	if _, err := pack.Activate(version); err != nil {
		t.Fatalf("激活失败: %v", err)
	}
	tables, err := pack.LoadTables()
	if err != nil {
		t.Fatal(err)
	}
	return tables
}

// TestNewPackBuffShape 新格式包（1.7.4.1）：行形态断言 + 落子引擎。
func TestNewPackBuffShape(t *testing.T) {
	tables := fullTablesVer(t, "1.7.4.1")
	meta := map[string]bool{
		"id": true, "名称": true, "描述": true, "限定": true, "品质": true, "版本": true,
		"a": true, "b": true, "dx": true, "lx": true, "mx": true, "code": true,
		"技能": true, "tag": true, "attr": true, "display": true,
	}
	checkRows := func(rows []any, table string) {
		t.Helper()
		for _, e := range rows {
			m, ok := e.(map[string]any)
			if !ok || m == nil {
				continue
			}
			for k := range m {
				if meta[k] {
					continue
				}
				t.Errorf("%s %v 顶层遗留数值键 %s", table, m["名称"], k)
			}
			if attr, ok := m["attr"].(map[string]any); ok && attr != nil {
				for k, v := range attr {
					if IsNum(v) {
						continue
					}
					if l, ok := v.([]any); ok {
						allNum, allStr := true, true
						for _, e2 := range l {
							if !IsNum(e2) {
								allNum = false
							}
							if _, ok := e2.(string); !ok {
								allStr = false
							}
						}
						if allNum || (len(l) == 0 || allStr) {
							continue
						}
					}
					if _, ok := v.(string); ok {
						continue
					}
					t.Errorf("%s %v attr 非法值 %s: %T", table, m["名称"], k, v)
				}
			}
		}
	}
	checkRows(tables.Buffs, "buff")
	checkRows(tables.Effects, "effect")
	lirico := tables.BuffByName["莉莉蔻4溯"]
	if lirico == nil {
		t.Fatalf("新包缺 莉莉蔻4溯")
	}
	if tags, ok := lirico["tag"].([]any); !ok || len(tags) != 1 || tags[0] != "充盈" {
		t.Errorf("莉莉蔻4溯 tag: %v", lirico["tag"])
	}
	attr, _ := lirico["attr"].(map[string]any)
	if Num(attr["独立增伤"]) != 0.2 {
		t.Errorf("莉莉蔻4溯 attr: %v", attr)
	}
	inst := LevelBuff(lirico, 1, 1, 1)
	if tags, ok := inst["tag"].([]any); !ok || len(tags) != 1 {
		t.Errorf("实例 tag 未复制: %v", inst["tag"])
	}
	if got := Num(inst["独立增伤"]); got != 0.2 {
		t.Errorf("实例 独立增伤=%v want 0.2", got)
	}
	// 落子引擎：tag 条件进 ConditionalList。
	state, err := BuildState(1501, map[string]any{
		"charLevel": 80, "charSkillLevel": []any{10, 10, 10},
		"meleeWeapon": 0, "rangedWeapon": 0, "auraMod": 0,
		"buffs": []any{[]any{"莉莉蔻4溯", 1}},
	}, tables, BuildOptions{})
	if err != nil {
		t.Fatalf("装配失败: %v", err)
	}
	found := false
	for _, r := range NewEngine(state, tables).ConditionalList() {
		if len(r.Tag) == 1 && r.Tag[0] == "充盈" && isClose(Num(r.Props["独立增伤"]), 0.2) {
			found = true
		}
	}
	if !found {
		t.Errorf("ConditionalList 缺 tag[充盈] 独立增伤条目")
	}
}

func TestMatchCondRule(t *testing.T) {
	nameRe := regexp.MustCompile("暗影奔袭")
	cases := []struct {
		name  string
		rule  CondRule
		field string
		tags  []string
		want  bool
	}{
		{"无条件永假", CondRule{}, "暗影奔袭", []string{"充盈"}, false},
		{"名命中", CondRule{Re: nameRe}, "暗影奔袭", nil, true},
		{"名未命中", CondRule{Re: nameRe}, "暗影一刺", nil, false},
		{"tag命中", CondRule{Tags: []string{"充盈"}}, "附加伤害", []string{"技能", "充盈"}, true},
		{"tag未命中", CondRule{Tags: []string{"充盈"}}, "伤害", []string{"技能"}, false},
		{"双条件都满足", CondRule{Re: nameRe, Tags: []string{"充盈"}}, "暗影奔袭", []string{"充盈"}, true},
		{"双条件缺tag", CondRule{Re: nameRe, Tags: []string{"充盈"}}, "暗影奔袭", []string{"技能"}, false},
		{"双条件缺名", CondRule{Re: nameRe, Tags: []string{"充盈"}}, "附加伤害", []string{"充盈"}, false},
	}
	for _, c := range cases {
		if got := MatchCondRule(c.rule, c.field, c.tags); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
	if got := FieldTags(map[string]any{"tag": []any{"技能", 1}}); len(got) != 1 || got[0] != "技能" {
		t.Errorf("FieldTags 非字符串过滤失败: %v", got)
	}
}
