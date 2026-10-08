// 碎片 BD parity：42/42 表达式 + attrs。
package dob

import (
	"testing"
)

func buildShardEngine(t *testing.T) (*Engine, map[string]any, map[string]any) {
	t.Helper()
	bd := loadGolden(t, "shardBuild1_bd.json").(map[string]any)
	data := loadGolden(t, "shardBuild1_data.json").(map[string]any)
	expr := loadGolden(t, "shardBuild1_expr.json").(map[string]any)
	tables := tablesFor(t, data)
	charID := int(Num(bd["charId"]))
	settings, _ := bd["settings"].(map[string]any)
	state, err := BuildState(charID, settings, tables, BuildOptions{})
	if err != nil {
		t.Fatalf("装配失败: %v", err)
	}
	return NewEngine(state, tables), expr, bd
}

func TestShardbuildParity(t *testing.T) {
	engine, expr, _ := buildShardEngine(t)
	cases := expr["cases"].([]any)
	matched := 0
	for _, c := range cases {
		cm := c.(map[string]any)
		name, _ := cm["expr"].(string)
		want := Num(cm["ts"])
		var got float64
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("表达式 %s 抛错: %v", name, r)
				}
			}()
			got = engine.Calculate(name)
		}()
		if t.Failed() {
			return
		}
		if !isClose(got, want) {
			t.Errorf("表达式 %s: mine=%v ref=%v", name, got, want)
		} else {
			matched++
		}
	}
	t.Logf("parity: matched=%d/%d", matched, len(cases))
}

func TestShardbuildAttrs(t *testing.T) {
	engine, expr, _ := buildShardEngine(t)
	want := expr["expectedAttrs"].(map[string]any)
	got := engine.CalculateWeaponAttributes(nil, false, false)
	for k, wv := range want {
		if k == "weapon" || !IsNum(wv) {
			continue
		}
		g, ok := got[k]
		if !ok || !IsNum(g) {
			t.Errorf("attrs.%s 缺失", k)
			continue
		}
		if !isClose(Num(g), Num(wv)) {
			t.Errorf("attrs.%s: mine=%v ref=%v", k, Num(g), Num(wv))
		}
	}
}
