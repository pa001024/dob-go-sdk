// 手造条件：mod 生效未命中 / 额外精通未命中（golden 全是命中态）。
// 数据源走 mock 全量包（无网络），最小装配精确断言分界。
package dob

import (
	"testing"
)

const effectCharID = 1501 // 莉兹贝尔（精通 重剑/霰弹枪）

func fullTables(t *testing.T) *GameDataTables {
	t.Helper()
	pack := NewDataPackStore(pinPackDir(t), "", "-")
	if _, err := pack.Activate(pinVersion); err != nil {
		t.Skipf("mock 包缺失: %v", err)
	}
	tables, err := pack.LoadTables()
	if err != nil {
		t.Fatal(err)
	}
	return tables
}

func effectAttrs(t *testing.T, tables *GameDataTables, settings map[string]any) map[string]any {
	t.Helper()
	base := map[string]any{
		"charLevel": 80, "charSkillLevel": []any{10, 10, 10},
		"meleeWeapon": 0, "rangedWeapon": 0, "auraMod": 0,
	}
	for k, v := range settings {
		base[k] = v
	}
	state, err := BuildState(effectCharID, base, tables, BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return NewEngine(state, tables).CalculateAttributes(false, false, 0)
}

func TestEffectPolarity(t *testing.T) {
	tables := fullTables(t)
	met := effectAttrs(t, tables, map[string]any{
		"charMods": []any{[]any{41735, 10}, []any{31522, 10}, []any{41002, 10}, []any{41715, 10}},
	})
	if got := Num(met["背水"]); got < 0.11 || got > 0.13 {
		t.Errorf("D趋向>=4 命中：背水=%v want≈0.12", got)
	}
	unmet := effectAttrs(t, tables, map[string]any{
		"charMods": []any{[]any{41735, 10}, []any{31522, 10}, []any{41002, 10}, []any{41411, 10}},
	})
	if got := Num(unmet["背水"]); got != 0 {
		t.Errorf("D趋向>=4 未命中：背水=%v want=0", got)
	}
}

func TestEffectStarID(t *testing.T) {
	tables := fullTables(t)
	met := effectAttrs(t, tables, map[string]any{"charMods": []any{[]any{41716, 10}}})
	if got := Num(met["昂扬"]); got < 0.35 || got > 0.37 {
		t.Errorf("*id<=1 命中：昂扬=%v want≈0.36", got)
	}
	unmet := effectAttrs(t, tables, map[string]any{"charMods": []any{[]any{41716, 10}, []any{41716, 10}}})
	if got := Num(unmet["昂扬"]); got != 0 {
		t.Errorf("*id<=1 未命中：昂扬=%v want=0", got)
	}
}

func TestEffectThreshold(t *testing.T) {
	tables := fullTables(t)
	unmet := effectAttrs(t, tables, map[string]any{"charMods": []any{[]any{56162, 10}}})
	if got := Num(unmet["昂扬"]); got != 0 {
		t.Errorf("范围>=1.6 未命中：昂扬=%v want=0", got)
	}
	met := effectAttrs(t, tables, map[string]any{
		"charMods": []any{[]any{56162, 10}, []any{41214, 10}, []any{41713, 10}},
	})
	if got := Num(met["昂扬"]); got < 0.65 || got > 0.67 {
		t.Errorf("范围>=1.6 命中：昂扬=%v want≈0.66", got)
	}
	if got := Num(met["技能范围"]); got < 1.6 {
		t.Errorf("技能范围=%v want>=1.6", got)
	}
}

func TestEffectAuraCounts(t *testing.T) {
	tables := fullTables(t)
	met := effectAttrs(t, tables, map[string]any{
		"charMods": []any{[]any{31502, 10}, []any{31513, 10}, []any{31526, 10}},
		"auraMod":  51765,
	})
	if got := Num(met["昂扬"]); got < 0.65 || got > 0.67 {
		t.Errorf("aura 计入 A趋向：昂扬=%v want≈0.66", got)
	}
	unmet := effectAttrs(t, tables, map[string]any{
		"charMods": []any{[]any{31502, 10}, []any{31513, 10}, []any{31522, 10}},
		"auraMod":  51765,
	})
	if got := Num(unmet["昂扬"]); got != 0 {
		t.Errorf("A趋向不足：昂扬=%v want=0", got)
	}
}

func TestForgeExtraMastery(t *testing.T) {
	tables := fullTables(t)
	miss := effectAttrs(t, tables, map[string]any{
		"meleeWeapon": 20298, "meleeWeaponRefine": 5, "meleeWeaponLevel": 80,
	})
	if got := Num(miss["背水"]); got != 0 {
		t.Errorf("无额外精通：背水=%v want=0", got)
	}
	hit := effectAttrs(t, tables, map[string]any{
		"meleeWeapon": 20298, "meleeWeaponRefine": 5, "meleeWeaponLevel": 80,
		"extraMastery": "双枪",
	})
	if got := Num(hit["背水"]); got < 0.19 || got > 0.21 {
		t.Errorf("extraMastery=双枪：背水=%v want≈0.2", got)
	}
}
