// BUFF × MOD 互斥（数据 `禁用`）：装备命中的 MOD 后 BUFF 不参与计算。
//
// 对齐 TS `CharBuild.isBuffDisabled` / Rust `refresh_buff_activation`：
// online_data.json 是旧快照（无 `禁用`），测试内注入字段。
package dob

import "testing"

// buffNames 收集 state 中生效的 BUFF 名称。
func buffNames(state map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, b := range AsList(state["buffs"]) {
		if m, ok := b.(map[string]any); ok {
			out[S(m, "名称")] = true
		}
	}
	return out
}

func TestBuffDisabledByMod(t *testing.T) {
	tables := onlineTables(t)
	raw := tables.BuffByName["激扬寒波"]
	if raw == nil {
		t.Fatal("夹具缺少 BUFF 激扬寒波")
	}
	// 巨鲸「激扬寒波」与 BUFF 同增益（固定攻击 +275）
	raw["禁用"] = []any{51922}

	// 角色槽固定 8 格（空槽为 nil），与真实配置同形
	slots := func(mods ...any) []any {
		out := make([]any, 8)
		copy(out, mods)
		return out
	}
	baseSettings := func(buffs []any, mods []any) map[string]any {
		return map[string]any{
			"charLevel":      80,
			"charSkillLevel": []any{10, 10, 10},
			"meleeWeapon":    0,
			"rangedWeapon":   0,
			"auraMod":        0,
			"buffs":          buffs,
			"charMods":       mods,
			"targetFunction": "伤害",
		}
	}
	build := func(buffs []any, mods []any) (map[string]any, *Engine) {
		state, err := BuildState(2201, baseSettings(buffs, mods), tables, BuildOptions{})
		if err != nil {
			t.Fatalf("装配失败: %v", err)
		}
		return state, NewEngine(state, tables)
	}

	// 未装备互斥 MOD：BUFF 生效
	onlyBuff, engineOnlyBuff := build([]any{[]any{"激扬寒波", 10}}, slots())
	if !buffNames(onlyBuff)["激扬寒波"] {
		t.Fatal("未装备互斥 MOD 时 BUFF 应生效")
	}
	withBuffAttack := Num(engineOnlyBuff.CalculateWeaponAttributes(nil, false, false)["攻击"])

	// 装备互斥 MOD：BUFF 失效，属性应与仅 MOD 一致
	bothBuff, engineBoth := build([]any{[]any{"激扬寒波", 10}}, slots([]any{51922, 10}))
	if buffNames(bothBuff)["激扬寒波"] {
		t.Fatal("装备互斥 MOD 后 BUFF 应被过滤")
	}
	modOnly, engineModOnly := build([]any{}, slots([]any{51922, 10}))
	if buffNames(modOnly)["激扬寒波"] {
		t.Fatal("仅 MOD 时不应出现该 BUFF")
	}
	bothAttack := Num(engineBoth.CalculateWeaponAttributes(nil, false, false)["攻击"])
	modOnlyAttack := Num(engineModOnly.CalculateWeaponAttributes(nil, false, false)["攻击"])
	if !isClose(bothAttack, modOnlyAttack) {
		t.Fatalf("BUFF+MOD 应与仅 MOD 一致：%v vs %v", bothAttack, modOnlyAttack)
	}
	if withBuffAttack <= modOnlyAttack {
		t.Fatalf("BUFF 未生效时攻击应更高：%v vs %v", withBuffAttack, modOnlyAttack)
	}
}
