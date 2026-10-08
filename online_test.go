// 线上 BD 靶点 parity：真实分享构筑（DOT 分流跳过）。
package dob

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func onlineFiles(t *testing.T) []string {
	t.Helper()
	dir := filepath.Join(goldenDir(t), "online")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	if len(out) == 0 {
		t.Fatal("online 靶点缺失")
	}
	return out
}

func onlineTables(t *testing.T) *GameDataTables {
	t.Helper()
	data := loadGolden(t, "online_data.json").(map[string]any)
	raw := map[string]any{}
	for _, table := range []string{"chars", "mods", "buffs", "weapons", "pets", "monsters"} {
		var list []any
		for _, e := range AsList(data[table]) {
			if e == nil {
				t.Fatalf("原始表 %s 含 null", table)
			}
			list = append(list, e)
		}
		raw[table] = list
	}
	var effects []any
	for _, e := range AsList(data["effects"]) {
		if e != nil {
			effects = append(effects, e)
		}
	}
	raw["effects"] = effects
	var entries []any
	for _, e := range AsList(data["petEntries"]) {
		if e == nil {
			t.Fatalf("petEntries 含 null")
		}
		entries = append(entries, e)
	}
	raw["pet_entries"] = entries
	tables, err := TablesFromDict(raw)
	if err != nil {
		t.Fatal(err)
	}
	return tables
}

// referencedVars 表达式引用的自定义变量名（IDENT 精确匹配）。
func referencedVars(body string, names map[string]bool) map[string]bool {
	out := map[string]bool{}
	toks, err := TokenizeAST(body, -1)
	if err != nil {
		return out
	}
	for _, tok := range toks {
		if tok.Kind == TokIdent && names[tok.Value] {
			out[tok.Value] = true
		}
	}
	return out
}

// dotDependent 传递引用 DOT伤害 的变量集合。
func dotDependent(settings map[string]any) map[string]bool {
	var entries [][2]string
	names := map[string]bool{}
	for _, e := range AsList(settings["customVariables"]) {
		pair, _ := e.([]any)
		if len(pair) < 2 {
			continue
		}
		k, _ := pair[0].(string)
		v, _ := pair[1].(string)
		entries = append(entries, [2]string{k, v})
		names[k] = true
	}
	closure := map[string]bool{}
	for _, e := range entries {
		if strings.Contains(e[1], "DOT伤害") {
			closure[e[0]] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, e := range entries {
			if closure[e[0]] {
				continue
			}
			for r := range referencedVars(e[1], closure) {
				_ = r
				closure[e[0]] = true
				changed = true
				break
			}
		}
	}
	return closure
}

func TestOnlineTargets(t *testing.T) {
	tables := onlineTables(t)
	expected := loadGolden(t, "online_expected.json").(map[string]any)["expected"].(map[string]any)
	for _, file := range onlineFiles(t) {
		file := file
		t.Run(file, func(t *testing.T) {
			payload := loadGolden(t, "online/"+file).(map[string]any)
			key := strings.TrimSuffix(file, ".json")
			want := expected[key].(map[string]any)
			settings, _ := payload["settings"].(map[string]any)
			state, err := BuildState(int(Num(payload["charId"])), settings, tables, BuildOptions{})
			if err != nil {
				t.Fatal(err)
			}
			engine := NewEngine(state, tables)
			skipped := dotDependent(settings)
			if len(skipped) > 0 {
				t.Logf("%s: 跳过 %d 个 DOT 依赖变量", key, len(skipped))
			}
			matched := 0
			for name, wv := range AsMap(want["vars"]) {
				if skipped[name] || !IsNum(wv) {
					continue
				}
				var got float64
				func() {
					defer func() {
						if r := recover(); r != nil {
							t.Errorf("%s 变量 %s 抛错: %v", file, name, r)
						}
					}()
					got = engine.Calculate(name)
				}()
				if t.Failed() {
					return
				}
				if !isClose(got, Num(wv)) {
					t.Errorf("%s 变量 %s: mine=%v ref=%v", file, name, got, Num(wv))
				} else {
					matched++
				}
			}
			if matched == 0 {
				t.Errorf("%s 无有效变量断言", file)
			}
			if tf, ok := settings["targetFunction"].(string); ok && tf != "" && !skipped[tf] {
				if tw, ok := want["target"]; ok && IsNum(tw) {
					assertClose(t, file+" target", engine.Calculate(""), Num(tw))
				}
			}
			gotAttrs := engine.CalculateWeaponAttributes(nil, false, false)
			for attr, wv := range AsMap(want["attrs"]) {
				if attr == "weapon" || !IsNum(wv) {
					continue
				}
				g, ok := gotAttrs[attr]
				if !ok || !IsNum(g) || !isClose(Num(g), Num(wv)) {
					t.Errorf("%s attrs.%s: mine=%v ref=%v", file, attr, g, wv)
				}
			}
		})
	}
}

func TestConditionalBuffFiltering(t *testing.T) {
	tables := onlineTables(t)
	expected := loadGolden(t, "online_expected.json").(map[string]any)["expected"].(map[string]any)
	payload := loadGolden(t, "online/Svmqw3LGoY.json").(map[string]any)
	settings, _ := payload["settings"].(map[string]any)
	state, err := BuildState(int(Num(payload["charId"])), settings, tables, BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(state, tables)
	var globalNames, dynamicNames []string
	for _, b := range AsList(state["buffs"]) {
		if m, ok := b.(map[string]any); ok {
			globalNames = append(globalNames, S(m, "名称"))
		}
	}
	for _, b := range AsList(state["dynamicBuffs"]) {
		if m, ok := b.(map[string]any); ok {
			dynamicNames = append(dynamicNames, S(m, "名称"))
		}
	}
	if !hasStr(dynamicNames, "琳恩裂伤特性") {
		t.Errorf("条件 BUFF 应在 dynamicBuffs: %v", dynamicNames)
	}
	if hasStr(globalNames, "琳恩裂伤特性") {
		t.Errorf("条件 BUFF 不应进全局 buffs")
	}
	found := false
	for _, r := range engine.ConditionalList() {
		if r.Skill == "裂伤" {
			found = true
		}
	}
	if !found {
		t.Errorf("裂伤条件规则缺失")
	}
	want := Num(AsMap(AsMap(expected["Svmqw3LGoY"])["vars"])["[引爆]单次"])
	assertClose(t, "[引爆]单次", engine.Calculate("[引爆]单次"), want)
}
