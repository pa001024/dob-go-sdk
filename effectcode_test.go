// 特效 code 执行 + 求索折算回归（对齐 TS src/data/tests/effect-code.test.ts）。
// 合成最小 state/上下文，不依赖 mock 数据包版本（旧包无 10199 数据）。
package dob

import (
	"testing"
)

func codeTestChar() map[string]any {
	return map[string]any{
		"id": 1, "名称": "测", "属性": "雷", "精通": []any{"单手剑"},
		"基础攻击": 100.0, "基础生命": 1000.0, "基础护盾": 500.0, "基础防御": 50.0, "基础神智": 100.0,
	}
}

func codeTestState(charMods []any, weapons map[string]map[string]any) map[string]any {
	melee, ok := weapons["melee"]
	if !ok {
		melee = map[string]any{}
	}
	ranged, ok := weapons["ranged"]
	if !ok {
		ranged = map[string]any{}
	}
	return map[string]any{
		"char":     codeTestChar(),
		"charMods": charMods, "meleeMods": []any{}, "rangedMods": []any{}, "skillMods": []any{},
		"meleeWeapon": melee, "rangedWeapon": ranged,
		"buffs": []any{}, "dynamicBuffs": []any{},
		"resonanceGain": 0.0, "hpPercent": 1.0,
		"enemyId": 130, "enemyLevel": 80, "enemyResistance": 0.0,
	}
}

func TestEffectCodeExecutes(t *testing.T) {
	mk := func(lv float64) map[string]any {
		return codeTestState([]any{
			map[string]any{
				"id": 999001.0, "名称": "码效测试", "系列": "测试", "品质": "金",
				"耐受": 0.0, "类型": "角色", "_等级": 10.0,
				"buff": map[string]any{"名称": "码效特效", "mx": 1.0, "_等级": lv, "code": "增伤+=0.5"},
			},
		}, nil)
	}
	on := NewEngine(mk(1), &GameDataTables{}).CalculateAttributes(false, false, 0)
	off := NewEngine(mk(0), &GameDataTables{}).CalculateAttributes(false, false, 0)
	if got := Num(on["增伤"]) - Num(off["增伤"]); got < 0.499999 || got > 0.500001 {
		t.Errorf("MOD 特效 code 增量=%v want=0.5", got)
	}
}

func TestEffectCodeWeaponForgeGate(t *testing.T) {
	mkWeapon := func(forge any) map[string]any {
		w := map[string]any{
			"id": 10199.0, "名称": "无解谜题", "类型": "近战", "forgeEffective": forge,
			"buff": map[string]any{"名称": "无解谜题", "mx": 6.0, "_等级": 6.0, "code": "增伤+=0.5"},
		}
		return codeTestState(nil, map[string]map[string]any{"melee": w})
	}
	on := NewEngine(mkWeapon(true), &GameDataTables{}).CalculateAttributes(false, false, 0)
	off := NewEngine(mkWeapon(false), &GameDataTables{}).CalculateAttributes(false, false, 0)
	if got := Num(on["增伤"]) - Num(off["增伤"]); got < 0.499999 || got > 0.500001 {
		t.Errorf("武器特效 code 熔炼门控增量=%v want=0.5", got)
	}
}

func qiusuoCtx(skillTrigger, weaponTrigger float64) *DamageContext {
	field := map[string]any{
		"名称": "[求索]伤害", "影响": "技能威力", "伤害类型": "灾厄", "值": 35.0,
		"tag": []any{"技能", "近战", "武器"},
	}
	weapon := map[string]any{"名称": "无解谜题", "类型": "近战", "伤害类型": "灾厄"}
	panel := map[string]any{
		"攻击": 100.0, "暴击": 0.3, "暴伤": 2.0, "触发": weaponTrigger, "攻速": 1.0, "多重": 1.0,
		"增伤": 0.0, "独立增伤": 0.0, "追加伤害": 0.0, "装填": 1.0, "弹匣": 20.0, "弹药": 100.0,
		"武器倍率": 0.0, "充盈转化": 1.0, "召唤物攻击速度转化": 0.0, "召唤物范围转化": 0.0,
	}
	attrs := map[string]any{
		"攻击": 1000.0, "增伤": 0.0, "技能伤害": 0.0, "元素增伤": 0.0, "物理增伤": 0.0,
		"独立增伤": 0.0, "技能追伤": 0.0, "技能无视防御": 0.0, "属性穿透": 0.0,
		"技能威力": 1.0, "技能触发": skillTrigger, "充盈威力": 0.0, "转充盈": 0.0,
	}
	return NewDamageContext(
		attrs,
		map[string]any{"求索": panel, "近战": panel},
		map[string][]map[string]any{"求索": {field}},
		nil, nil, nil, map[string]any{},
		map[string]any{"求索": weapon}, map[string]any{},
		map[string]any{"enemyResistance": 0.5}, "普通攻击", nil,
	)
}

func TestQiusuoSkillTriggerCoverage(t *testing.T) {
	// 触发概率 = 武器面板触发(0.2) + 技能触发后封顶：0.74/0.29
	hi := qiusuoCtx(0.54, 0.2).GetDamage("求索", "[求索]伤害", map[string]any{}, "")
	lo := qiusuoCtx(0.09, 0.2).GetDamage("求索", "[求索]伤害", map[string]any{}, "")
	if lo["expectedDamage"] == nil || Num(lo["expectedDamage"]) == 0 {
		t.Fatalf("低触发期望为 0，无法验证比例")
	}
	if got := Num(hi["expectedDamage"]) / Num(lo["expectedDamage"]); got < 2.551 || got > 2.553 {
		t.Errorf("求索伤害触发比例=%v want=0.74/0.29", got)
	}
	floor := qiusuoCtx(0, 0.2).GetDamage("求索", "[求索]伤害", map[string]any{}, "")
	capped := qiusuoCtx(1, 0.2).GetDamage("求索", "[求索]伤害", map[string]any{}, "")
	if got := Num(floor["expectedDamage"]) / Num(capped["expectedDamage"]); got < 0.19999 || got > 0.20001 {
		t.Errorf("无技能触发时应按武器面板触发结算，比例=%v want=0.2", got)
	}
}

func TestQiusuoWeaponTriggerIgnored(t *testing.T) {
	// 封顶后武器触发不再影响求索伤害
	a := qiusuoCtx(1, 0.2).GetDamage("求索", "[求索]伤害", map[string]any{}, "")
	b := qiusuoCtx(1, 2.0).GetDamage("求索", "[求索]伤害", map[string]any{}, "")
	if got := Num(b["expectedDamage"]) / Num(a["expectedDamage"]); got < 0.999999 || got > 1.000001 {
		t.Errorf("封顶后武器触发应不影响求索伤害，比例=%v want=1", got)
	}
	// 未封顶时武器触发提高求索伤害（加法模型）
	c := qiusuoCtx(0, 0.2).GetDamage("求索", "[求索]伤害", map[string]any{}, "")
	d := qiusuoCtx(0, 0.5).GetDamage("求索", "[求索]伤害", map[string]any{}, "")
	if got := Num(d["expectedDamage"]) / Num(c["expectedDamage"]); got < 2.49999 || got > 2.50001 {
		t.Errorf("未封顶时武器触发应等比提高求索伤害，比例=%v want=2.5", got)
	}
}

func TestQiusuoDamagePools(t *testing.T) {
	// 求索伤害视为技能伤害：吃技能伤害，不吃武器伤害/武器追伤
	base := qiusuoCtx(1, 0.2).GetDamage("求索", "[求索]伤害", map[string]any{}, "")
	baseDmg := Num(base["expectedDamage"])
	if baseDmg == 0 {
		t.Fatalf("基准求索伤害为 0")
	}
	withSkill := qiusuoCtx(1, 0.2)
	withSkill.Attrs["技能伤害"] = 1.0
	if got := Num(withSkill.GetDamage("求索", "[求索]伤害", map[string]any{}, "")["expectedDamage"]) / baseDmg; got < 1.99999 || got > 2.00001 {
		t.Errorf("技能伤害+100%% 比例=%v want=2", got)
	}
	withWeapon := qiusuoCtx(1, 0.2)
	withWeapon.Attrs["武器伤害"] = 1.0
	if got := Num(withWeapon.GetDamage("求索", "[求索]伤害", map[string]any{}, "")["expectedDamage"]) / baseDmg; got < 0.999999 || got > 1.000001 {
		t.Errorf("武器伤害应不影响求索伤害，比例=%v want=1", got)
	}
	withChase := qiusuoCtx(1, 0.2)
	withChase.Panels["求索"].(map[string]any)["追加伤害"] = 1.0
	if got := Num(withChase.GetDamage("求索", "[求索]伤害", map[string]any{}, "")["expectedDamage"]) / baseDmg; got < 0.999999 || got > 1.000001 {
		t.Errorf("武器追伤应不影响求索伤害，比例=%v want=1", got)
	}
}
