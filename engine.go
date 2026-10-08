// 构筑聚合引擎：纯 BD JSON + 原始表 → 属性/面板/技能表/战斗状态 → 目标值。
package dob

import (
	"math"
	"regexp"
	"strings"
)

// CharacterBonus 40 维向量属性名。
var CharacterBonus = []string{
	"攻击", "固定攻击", "固定生命", "生命", "护盾", "防御", "神智", "属性攻击",
	"技能威力", "技能耐久", "技能效益", "技能范围", "昂扬", "背水", "增伤",
	"元素增伤", "物理增伤", "武器伤害", "技能伤害", "技能速度", "属性穿透",
	"失衡易伤", "技能倍率加数", "召唤物属性继承比例", "召唤物攻击速度", "召唤物范围",
	"召唤物伤害", "召唤物独立增伤", "技能倍率赋值", "转切割", "转贯穿", "转震荡",
	"转灾厄", "转充盈", "转属克", "转属逆", "充盈威力", "技能触发", "异常数量", "魔灵CD缩减",
}

// AttackTypeTags 攻击类型标签。
var AttackTypeTags = [][2]any{
	{"普攻", []string{"普攻", "普通攻击"}},
	{"蓄力", []string{"蓄力攻击"}},
	{"下落", []string{"下落攻击"}},
	{"滑行", []string{"滑行攻击"}},
}

// HpTypeCoefficients / HpTypeDMG 血条类型系数与伤害类型。
var HpTypeCoefficients = map[string]float64{"生命": 0.5, "护盾": 1, "战姿": 1}
var HpTypeDMG = map[string]string{"生命": "贯穿", "护盾": "切割", "战姿": "震荡"}

// ElmSeries 元素系列。
var ElmSeries = []string{"狮鹫", "百首", "契约者", "换生灵"}

// ZeroMap code 沙箱槽位记录：缺键读 0。
type ZeroMap map[string]any

// BonusTable 加成汇总表。
type BonusTable struct {
	Char           map[string]float64
	Melee          map[string]float64
	Ranged         map[string]float64
	ModsChar       map[string]float64
	ModsByScope    map[string]map[string]float64
	ModsAll        map[string]float64
	ModsBuffProps  map[string]float64
	Buffs          map[string]float64
	WeaponBuffProp map[string]float64
}

// Engine 是一次构筑的聚合状态机。
type Engine struct {
	S map[string]any
	T *GameDataTables

	table    *BonusTable
	modsMul  map[string]float64
	buffsMul map[string]float64
	skillTbl map[string][]map[string]any
}

// NewEngine 构造。
func NewEngine(state map[string]any, tables *GameDataTables) *Engine {
	return &Engine{S: state, T: tables, modsMul: map[string]float64{}, buffsMul: map[string]float64{}}
}

func charOf(s map[string]any) map[string]any {
	if m, ok := s["char"].(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func listOf(s map[string]any, key string) []any {
	return AsList(s[key])
}

func instOf(s map[string]any, key string) map[string]any {
	if m, ok := s[key].(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

// Mods 全部 MOD 实例。
func (e *Engine) Mods() []map[string]any {
	var out []map[string]any
	for _, slot := range []string{"charMods", "meleeMods", "rangedMods", "skillMods"} {
		for _, m := range listOf(e.S, slot) {
			if mm, ok := m.(map[string]any); ok && mm != nil {
				out = append(out, mm)
			}
		}
	}
	if aura, ok := e.S["auraMod"].(map[string]any); ok && aura != nil {
		out = append(out, aura)
	}
	return out
}

// ScopedMods 指定类型 MOD。
func (e *Engine) ScopedMods(scope string) []map[string]any {
	var out []map[string]any
	for _, m := range e.Mods() {
		if scope == "" || S(m, "类型") == scope {
			out = append(out, m)
		}
	}
	return out
}

// ScopeOf 作用域判定。
func ScopeOf(prefix string) string {
	if strings.HasPrefix(prefix, "同律近战") {
		return "同律近战"
	}
	if strings.HasPrefix(prefix, "同律远程") {
		return "同律远程"
	}
	if strings.HasPrefix(prefix, "近战") {
		return "近战"
	}
	if strings.HasPrefix(prefix, "远程") {
		return "远程"
	}
	return prefix
}

// BuffInScope BUFF 作用域判定。
func BuffInScope(attribute, scope string) bool {
	attrScope := ScopeOf(attribute)
	if attrScope == "近战" || attrScope == "远程" || strings.HasPrefix(attrScope, "同律") {
		return scope == attrScope
	}
	return true
}

// BonusTable 建表。
func (e *Engine) BonusTable() *BonusTable {
	if e.table != nil {
		return e.table
	}
	t := &BonusTable{
		Char: map[string]float64{}, Melee: map[string]float64{}, Ranged: map[string]float64{},
		ModsChar: map[string]float64{}, ModsByScope: map[string]map[string]float64{},
		ModsAll: map[string]float64{}, ModsBuffProps: map[string]float64{},
		Buffs: map[string]float64{}, WeaponBuffProp: map[string]float64{},
	}
	acc := func(target map[string]float64, source map[string]any, keys []string) {
		if source == nil {
			return
		}
		if keys == nil {
			for k, v := range source {
				if IsNum(v) {
					target[k] += Num(v)
				}
			}
			return
		}
		for _, k := range keys {
			if v, ok := source[k]; ok && IsNum(v) {
				target[k] += Num(v)
			}
		}
	}
	char := charOf(e.S)
	acc(t.Char, AsMap(char["加成"]), nil)
	melee := instOf(e.S, "meleeWeapon")
	ranged := instOf(e.S, "rangedWeapon")
	acc(t.Melee, melee, nil)
	acc(t.Ranged, ranged, nil)
	acc(t.WeaponBuffProp, AsMap(melee["buffProps"]), nil)
	acc(t.WeaponBuffProp, AsMap(ranged["buffProps"]), nil)
	for _, mod := range e.Mods() {
		props := ModProperties(mod)
		acc(t.ModsAll, mod, props)
		bucket, ok := t.ModsByScope[S(mod, "类型")]
		if !ok {
			bucket = map[string]float64{}
			t.ModsByScope[S(mod, "类型")] = bucket
		}
		acc(bucket, mod, props)
		if S(mod, "类型") == "角色" {
			acc(t.ModsChar, mod, props)
		}
		acc(t.ModsBuffProps, AsMap(mod["buffProps"]), nil)
	}
	for _, buff := range AsList(e.S["buffs"]) {
		if bm, ok := buff.(map[string]any); ok && bm != nil {
			if _, ok := bm["技能"].(string); ok {
				continue
			}
			acc(t.Buffs, bm, nil)
		}
	}
	e.table = t
	return t
}

// SumMods MOD 求和。
func (e *Engine) SumMods(t *BonusTable, attribute, scope string, includeMods bool) float64 {
	if !includeMods {
		return 0
	}
	switch attribute {
	case "暴击", "暴伤", "触发", "攻速", "充盈转化", "召唤物攻击速度转化", "召唤物范围转化":
		bonus := t.ModsChar[attribute]
		if scope != "角色" {
			bonus += t.ModsByScope[scope][attribute]
		}
		return bonus
	}
	if scope == "" {
		return t.ModsAll[attribute]
	}
	return t.ModsByScope[scope][attribute]
}

// ScopedModsMul MOD 乘区。
func (e *Engine) ScopedModsMul(t *BonusTable, attribute, scope string) float64 {
	key := scope + "\x00" + attribute
	if v, ok := e.modsMul[key]; ok {
		return v
	}
	product := 1.0
	for _, mod := range e.ScopedMods(scope) {
		if v, ok := mod[attribute]; ok && IsNum(v) {
			product *= 1 + Num(v)
		}
	}
	e.modsMul[key] = product
	return product
}

// BuffsMul BUFF 乘区。
func (e *Engine) BuffsMul(t *BonusTable, attribute string) float64 {
	if v, ok := e.buffsMul[attribute]; ok {
		return v
	}
	product := 1.0
	for _, b := range AsList(e.S["buffs"]) {
		bm, ok := b.(map[string]any)
		if !ok || bm == nil {
			continue
		}
		if _, ok := bm["技能"].(string); ok {
			continue
		}
		if v, ok := bm[attribute]; ok && IsNum(v) {
			product *= 1 + Num(v)
		}
	}
	e.buffsMul[attribute] = product
	return product
}

// Mastered 精通判定。
func (e *Engine) Mastered(category string) bool {
	char := charOf(e.S)
	for _, p := range AsList(char["精通"]) {
		if s, ok := p.(string); ok && (s == category || s == "全部类型") {
			return true
		}
	}
	if extra, ok := e.S["extraMastery"].(string); ok && extra != "" && category == extra {
		return true
	}
	return false
}

// ForgeEffective 锻造生效判定。
func (e *Engine) ForgeEffective(weapon map[string]any) bool {
	if has, ok := weapon["_hasForge"].(bool); !ok || !has {
		return true
	}
	return e.Mastered(S(weapon, "类别"))
}

// GetTotal 总加成。
func (e *Engine) GetTotal(attribute, prefix string, includeMods bool) float64 {
	t := e.BonusTable()
	scope := ScopeOf(prefix)
	bonus := 0.0
	if prefix == "角色" || attribute != "攻击" {
		bonus += t.Char[attribute]
	}
	melee := instOf(e.S, "meleeWeapon")
	ranged := instOf(e.S, "rangedWeapon")
	if scope == "角色" || (scope == "近战" && attribute != "攻击") {
		if e.ForgeEffective(melee) {
			bonus += t.Melee[attribute]
		}
	}
	if scope == "角色" || (scope == "远程" && attribute != "攻击") {
		if e.ForgeEffective(ranged) {
			bonus += t.Ranged[attribute]
		}
	}
	bonus += e.SumMods(t, attribute, scope, includeMods)
	inScope := BuffInScope(attribute, scope)
	shared := scope == "角色" || (attribute != "攻击" && attribute != "增伤")
	if inScope && shared {
		bonus += t.Buffs[attribute]
	}
	if inScope && shared {
		bonus += t.ModsBuffProps[attribute]
	}
	if shared {
		bonus += t.WeaponBuffProp[attribute]
	}
	return bonus
}

// GetTotalMul 总乘区。
func (e *Engine) GetTotalMul(attribute, prefix string, includeMods bool) float64 {
	t := e.BonusTable()
	scope := ScopeOf(prefix)
	bonus := 1.0
	if v, ok := AsMap(charOf(e.S)["加成"])[attribute]; ok && IsNum(v) {
		bonus *= 1 + Num(v)
	}
	if includeMods {
		bonus *= e.ScopedModsMul(t, attribute, scope)
	}
	inScope := BuffInScope(attribute, scope)
	shared := scope == "角色" || attribute != "独立增伤"
	if inScope && shared {
		bonus *= e.BuffsMul(t, attribute)
	}
	return bonus - 1
}

// GetTotalReduce 总减伤。
func (e *Engine) GetTotalReduce(attribute, prefix string) float64 {
	bonus := 0.0
	for _, mod := range e.Mods() {
		if prefix != "" && S(mod, "类型") != prefix {
			continue
		}
		if v, ok := mod[attribute]; ok && IsNum(v) {
			bonus = 1 - (1-bonus)*(1-Num(v))
		}
	}
	for _, b := range AsList(e.S["buffs"]) {
		bm, ok := b.(map[string]any)
		if !ok || bm == nil {
			continue
		}
		if _, ok := bm["技能"].(string); ok {
			continue
		}
		if v, ok := bm[attribute]; ok && IsNum(v) {
			bonus = 1 - (1-bonus)*(1-Num(v))
		}
	}
	return bonus
}

// GetModsBonus 指定 MOD 列表加成。
func (e *Engine) GetModsBonus(mods []any, attribute, prefix string) float64 {
	scope := ScopeOf(prefix)
	bonus := 0.0
	if prefix == "角色" || !hasPrefixStr(attribute, prefix) {
		for _, m := range mods {
			mm, ok := m.(map[string]any)
			if !ok || mm == nil {
				continue
			}
			if scope != "" && S(mm, "类型") != scope {
				continue
			}
			if v, ok := mm[attribute]; ok && IsNum(v) {
				bonus += Num(v)
			}
		}
	}
	return bonus
}

func hasPrefixStr(s, prefix string) bool { return strings.HasPrefix(s, prefix) }

// BonusVector 40 维向量。
func (e *Engine) BonusVector() []float64 {
	out := make([]float64, 0, len(CharacterBonus))
	for _, a := range CharacterBonus {
		out = append(out, e.GetTotal(a, "角色", true))
	}
	return out
}

// ConditionValues 条件计数。
func (e *Engine) ConditionValues() map[string]any {
	values := map[string]any{}
	var weapons []map[string]any
	if melee := instOf(e.S, "meleeWeapon"); !isEmptyWeapon(melee) {
		weapons = append(weapons, melee)
	}
	if ranged := instOf(e.S, "rangedWeapon"); !isEmptyWeapon(ranged) {
		weapons = append(weapons, ranged)
	}
	if sw, ok := e.S["skillWeapon"].(map[string]any); ok && sw != nil && S(sw, "inherit") == "" {
		weapons = append(weapons, sw)
	}
	for _, w := range weapons {
		if cat := S(w, "类别"); cat != "" {
			values[cat] = Num(values[cat]) + 1
		}
	}
	for _, c := range AsList(e.S["teamWeaponCategories"]) {
		if s, ok := c.(string); ok {
			values[s] = Num(values[s]) + 1
		}
	}
	return values
}

func isEmptyWeapon(w map[string]any) bool {
	if w == nil || len(w) == 0 {
		return true
	}
	if b, ok := w["_isEmpty"].(bool); ok {
		return b
	}
	return false
}

// ApplyCondition 条件改写 MOD 实例。
func (e *Engine) ApplyCondition(attrs map[string]any, mods []any) bool {
	condValues := e.ConditionValues()
	var charMods []any
	charMods = append(charMods, listOf(e.S, "charMods")...)
	if aura, ok := e.S["auraMod"].(map[string]any); ok && aura != nil {
		charMods = append(charMods, aura)
	}
	changed := false
	for _, m := range mods {
		if mm, ok := m.(map[string]any); ok && mm != nil {
			if ModApplyCondition(mm, attrs, charMods, condValues) {
				changed = true
			}
		}
	}
	if changed {
		e.table = nil
		e.modsMul = map[string]float64{}
		e.buffsMul = map[string]float64{}
	}
	return changed
}

// ApplyBuffAttr BUFF 动态 attr。
func (e *Engine) ApplyBuffAttr(attrs map[string]any) bool {
	changed := false
	for _, b := range AsList(e.S["buffs"]) {
		buff, ok := b.(map[string]any)
		if !ok || buff == nil {
			continue
		}
		exprs, _ := buff["attr"].(map[string]any)
		if len(exprs) == 0 {
			continue
		}
		ctx := e.DamageContext(attrs)
		for key, expression := range exprs {
			exprStr, _ := expression.(string)
			value, ok := evalBuffAttrExpr(ctx, attrs, exprStr)
			if !ok {
				continue
			}
			if !IsFiniteNum(value) {
				value = 0
			}
			value = value * NumD(buff["_coverage"], 1)
			if !valuesEqual(buff[key], value) {
				buff[key] = value
				changed = true
			}
		}
	}
	return changed
}

// evalBuffAttrExpr 求 attr-BUFF 表达式（抛错即跳过该条，对齐 except Exception: continue）。
func evalBuffAttrExpr(ctx *DamageContext, attrs map[string]any, exprStr string) (value float64, ok bool) {
	defer func() {
		if recover() != nil {
			value, ok = 0, false
		}
	}()
	value = NewEvaluator(CloneMap(attrs), nil, nil, nil, ctx.Panels, ctx).Eval(exprStr, nil, nil, nil, nil)
	return value, true
}

// CalculateAttributes 角色属性（含 MOD 条件 / attr-BUFF / code-BUFF）。
func (e *Engine) CalculateAttributes(nocode, attrApplied bool, depth int) map[string]any {
	if depth > 6 {
		panic("属性不动点迭代过深")
	}
	bonuses := e.BonusVector()
	idx := map[string]int{}
	for i, name := range CharacterBonus {
		idx[name] = i
	}
	attackBonus := bonuses[idx["攻击"]]
	attackAdd := bonuses[idx["固定攻击"]]
	healthAdd := bonuses[idx["固定生命"]]
	healthBonus := bonuses[idx["生命"]]
	shieldBonus := bonuses[idx["护盾"]]
	defenseBonus := bonuses[idx["防御"]]
	sanityBonus := bonuses[idx["神智"]]
	elemBonus := bonuses[idx["属性攻击"]]
	power := 1 + bonuses[idx["技能威力"]]
	durability := 1 + bonuses[idx["技能耐久"]]
	efficiency := 1 + bonuses[idx["技能效益"]]
	scopeRange := 1 + bonuses[idx["技能范围"]]
	boost := bonuses[idx["昂扬"]]
	desperate := bonuses[idx["背水"]]
	damageInc := bonuses[idx["增伤"]]
	elemInc := bonuses[idx["元素增伤"]]
	physInc := bonuses[idx["物理增伤"]]
	weaponDmg := bonuses[idx["武器伤害"]]
	skillDmg := bonuses[idx["技能伤害"]]
	skillSpeed := bonuses[idx["技能速度"]]
	penetration := bonuses[idx["属性穿透"]]
	imbalanceBonus := bonuses[idx["失衡易伤"]]
	skillAdd := bonuses[idx["技能倍率加数"]]
	inheritRatio := 1 + bonuses[idx["召唤物属性继承比例"]]
	summonAs := bonuses[idx["召唤物攻击速度"]]
	summonRange := bonuses[idx["召唤物范围"]]
	summonDmg := bonuses[idx["召唤物伤害"]]
	summonInd := bonuses[idx["召唤物独立增伤"]]
	ignoreDef := e.GetTotal("无视防御", "角色", true)
	skillIgnoreDef := e.GetTotal("技能无视防御", "角色", true)
	indInc := e.GetTotalMul("独立增伤", "角色", true)
	damageReduce := e.GetTotalReduce("减伤", "")
	skillSet := bonuses[idx["技能倍率赋值"]]
	skillMul := e.GetTotalMul("技能倍率乘数", "角色", true)
	convert := map[string]float64{}
	for _, k := range []string{"转切割", "转贯穿", "转震荡", "转灾厄", "转充盈", "转属克", "转属逆"} {
		convert[k] = bonuses[idx[k]]
	}
	fullnessBonus := bonuses[idx["充盈威力"]]
	skillTrigger := bonuses[idx["技能触发"]]
	anomaly := bonuses[idx["异常数量"]]
	petCdReduce := bonuses[idx["魔灵CD缩减"]]
	char := charOf(e.S)
	modAttrBonus := e.GetTotal(S(char, "属性")+"MOD属性", "角色", true)
	if modAttrBonus > 0 {
		var elm []any
		for _, m := range listOf(e.S, "charMods") {
			if mm, ok := m.(map[string]any); ok && mm != nil && hasStr(ElmSeries, S(mm, "系列")) {
				elm = append(elm, mm)
			}
		}
		attackBonus += modAttrBonus * e.GetModsBonus(elm, "攻击", "角色")
		healthBonus += modAttrBonus * e.GetModsBonus(elm, "生命", "角色")
		shieldBonus += modAttrBonus * e.GetModsBonus(elm, "护盾", "角色")
		defenseBonus += modAttrBonus * e.GetModsBonus(elm, "防御", "角色")
		sanityBonus += modAttrBonus * e.GetModsBonus(elm, "神智", "角色")
		elemBonus += modAttrBonus * e.GetModsBonus(elm, "属性攻击", "角色")
		power += modAttrBonus * e.GetModsBonus(elm, "技能威力", "角色")
		durability += modAttrBonus * e.GetModsBonus(elm, "技能耐久", "角色")
		efficiency += modAttrBonus * e.GetModsBonus(elm, "技能效益", "角色")
		scopeRange += modAttrBonus * e.GetModsBonus(elm, "技能范围", "角色")
		boost += modAttrBonus * e.GetModsBonus(elm, "昂扬", "角色")
		desperate += modAttrBonus * e.GetModsBonus(elm, "背水", "角色")
		damageInc += modAttrBonus * e.GetModsBonus(elm, "增伤", "角色")
		elemInc += modAttrBonus * e.GetModsBonus(elm, "元素增伤", "角色")
		physInc += modAttrBonus * e.GetModsBonus(elm, "物理增伤", "角色")
		weaponDmg += modAttrBonus * e.GetModsBonus(elm, "武器伤害", "角色")
		skillDmg += modAttrBonus * e.GetModsBonus(elm, "技能伤害", "角色")
		skillSpeed += modAttrBonus * e.GetModsBonus(elm, "技能速度", "角色")
		penetration += modAttrBonus * e.GetModsBonus(elm, "属性穿透", "角色")
		imbalanceBonus += modAttrBonus * e.GetModsBonus(elm, "失衡易伤", "角色")
		skillAdd += modAttrBonus * e.GetModsBonus(elm, "技能倍率加数", "角色")
		inheritRatio += modAttrBonus * e.GetModsBonus(elm, "召唤物属性继承比例", "角色")
		summonAs += modAttrBonus * e.GetModsBonus(elm, "召唤物攻击速度", "角色")
		summonRange += modAttrBonus * e.GetModsBonus(elm, "召唤物范围", "角色")
		summonDmg += modAttrBonus * e.GetModsBonus(elm, "召唤物伤害", "角色")
		summonInd += modAttrBonus * e.GetModsBonus(elm, "召唤物独立增伤", "角色")
	}
	resonance := Num(e.S["resonanceGain"])
	attack := Num(char["基础攻击"]) * (1 + attackBonus + resonance)
	health := Num(char["基础生命"]) * (1 + healthBonus + resonance)
	shield := Num(char["基础护盾"]) * (1 + shieldBonus + resonance)
	defense := Num(char["基础防御"]) * (1 + defenseBonus + resonance)
	sanity := Num(char["基础神智"]) * (1 + sanityBonus)
	attack = attack*(1+elemBonus) + attackAdd
	health = float64(JsRound(health + healthAdd))
	shield = float64(JsRound(shield))
	defense = float64(JsRound(defense))
	sanity = float64(JsRound(sanity))
	attack = float64(JsRound(attack*100)) / 100
	if efficiency > 1.75 {
		efficiency = 1.75
	}
	if scopeRange > 2.8 {
		scopeRange = 2.8
	}
	if durability > 4 {
		durability = 4
	}
	petCd := math.Max(0, Num(e.S["petBaseCd"])*(1-pyClamp(petCdReduce, 0, 1)))
	anomalyVal := anomaly
	if S(char, "属性") == "光" || S(char, "属性") == "暗" {
		anomalyVal = 1 + anomaly
	} else if anomaly < 1 {
		anomalyVal = 1
	}
	attrs := map[string]any{
		"攻击": attack, "生命": health, "护盾": shield, "防御": defense, "神智": sanity,
		"技能威力": power, "技能耐久": durability, "技能效益": efficiency, "技能范围": scopeRange,
		"昂扬": boost, "背水": desperate, "增伤": damageInc, "元素增伤": elemInc,
		"物理增伤": physInc, "属性攻击": elemBonus, "武器伤害": weaponDmg, "技能伤害": skillDmg,
		"独立增伤": indInc, "属性穿透": penetration, "无视防御": ignoreDef,
		"技能无视防御": skillIgnoreDef, "技能速度": skillSpeed, "失衡易伤": imbalanceBonus,
		"技能倍率加数": skillAdd, "技能倍率乘数": skillMul,
		"召唤物属性继承比例": inheritRatio, "召唤物攻击速度": summonAs,
		"召唤物范围": summonRange, "召唤物伤害": summonDmg, "召唤物独立增伤": summonInd,
		"减伤": damageReduce, "技能倍率赋值": skillSet,
		"有效生命": (health/(1-defense/(300+defense)) + shield) / (1 - damageReduce),
		"转切割":  convert["转切割"], "转贯穿": convert["转贯穿"], "转震荡": convert["转震荡"],
		"转灾厄": convert["转灾厄"], "转充盈": convert["转充盈"], "转属克": convert["转属克"],
		"转属逆": convert["转属逆"], "充盈威力": fullnessBonus, "技能触发": skillTrigger,
		"异常数量": anomalyVal,
		"魔灵CD": petCd, "魔灵CD缩减": pyClamp(petCdReduce, 0, 1),
	}
	var condMods []any
	for _, m := range listOf(e.S, "charMods") {
		if mm, ok := m.(map[string]any); ok && mm != nil {
			if eff, ok := mm["生效"].(map[string]any); ok && eff["条件"] != nil {
				condMods = append(condMods, mm)
			}
		}
	}
	if aura, ok := e.S["auraMod"].(map[string]any); ok && aura != nil {
		if eff, ok := aura["生效"].(map[string]any); ok && eff["条件"] != nil {
			condMods = append(condMods, aura)
		}
	}
	if e.ApplyCondition(attrs, condMods) {
		return e.CalculateAttributes(nocode, attrApplied, depth+1)
	}
	if !attrApplied && e.ApplyBuffAttr(attrs) {
		return e.CalculateAttributes(nocode, true, depth+1)
	}
	if nocode {
		return attrs
	}
	if dyn := AsList(e.S["dynamicBuffs"]); len(dyn) > 0 {
		allPanels := e.AllWeaponPanels(nil, nil)
		modAttrs := e.ModAttrSums()
		for _, b := range dyn {
			if buff, ok := b.(map[string]any); ok && buff != nil {
				if _, ok := buff["技能"].(string); ok {
					continue
				}
				attrs = e.ApplyCodeBuff(buff, attrs, allPanels, modAttrs)
			}
		}
	}
	return attrs
}

func pyClamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ModAttrSums 各槽位 MOD 属性求和（code 沙箱用）。
func (e *Engine) ModAttrSums() map[string]map[string]float64 {
	out := map[string]map[string]float64{}
	for _, slot := range []string{"charMods", "meleeMods", "rangedMods", "skillMods"} {
		sums := map[string]float64{}
		for _, m := range listOf(e.S, slot) {
			if mm, ok := m.(map[string]any); ok && mm != nil {
				for _, prop := range ModProperties(mm) {
					if v, ok := mm[prop]; ok && IsNum(v) {
						sums[prop] += Num(v)
					}
				}
			}
		}
		out[slot] = sums
	}
	return out
}

// AllWeaponPanels 顺序面板 [选中, 近战, 远程, 同律]。
func (e *Engine) AllWeaponPanels(weapon, weaponAttrs map[string]any) []map[string]any {
	ordered := []map[string]any{e.SelectedWeapon(), instOf(e.S, "meleeWeapon"), instOf(e.S, "rangedWeapon"), AsMap(e.S["skillWeapon"])}
	arg := weapon
	if arg == nil {
		arg = e.SelectedWeapon()
	}
	out := make([]map[string]any, 0, 4)
	for _, w := range ordered {
		if w == nil || len(w) == 0 {
			out = append(out, nil)
			continue
		}
		if arg != nil && S(w, "名称") == S(arg, "名称") {
			out = append(out, weaponAttrs)
		} else {
			out = append(out, AsMap(e.CalculateWeaponAttributes(w, true, true)["weapon"]))
		}
	}
	return out
}

// ApplyCodeBuff 复刻 applyDynamicAttr。
func (e *Engine) ApplyCodeBuff(buff, attrs map[string]any, allPanels []map[string]any, modAttrs map[string]map[string]float64) map[string]any {
	code, _ := buff["code"].(string)
	if code == "" {
		return attrs
	}
	char := charOf(e.S)
	weapons := []map[string]any{e.SelectedWeapon(), instOf(e.S, "meleeWeapon"), instOf(e.S, "rangedWeapon"), AsMap(e.S["skillWeapon"])}
	panels := append([]map[string]any{}, allPanels...)
	for len(panels) < 4 {
		panels = append(panels, nil)
	}
	sandbox := CloneMap(attrs)
	info := func(w map[string]any) map[string]any {
		if w == nil || len(w) == 0 {
			return nil
		}
		return map[string]any{
			"基础攻击": w["基础攻击"], "基础暴击": w["基础暴击"], "基础暴伤": w["基础暴伤"], "基础触发": w["基础触发"],
		}
	}
	sandbox["char"] = map[string]any{
		"基础攻击": char["基础攻击"], "基础生命": char["基础生命"], "基础护盾": char["基础护盾"],
		"基础防御": char["基础防御"], "基础神智": char["基础神智"],
	}
	names := []string{"weapon", "meleeWeapon", "rangedWeapon", "skillWeapon"}
	for i, key := range names {
		sandbox[key] = info(weapons[i])
	}
	pkeys := []string{"weaponAttr", "meleeWeaponAttr", "rangedWeaponAttr", "skillWeaponAttr"}
	for i, key := range pkeys {
		sandbox[key] = panels[i]
	}
	sandbox["enemy"] = e.EnemySandbox()
	for slot, sums := range modAttrs {
		zm := map[string]any{}
		for k, v := range sums {
			zm[k] = v
		}
		sandbox[slot] = ZeroMapWrap(zm)
	}
	func() {
		defer func() { _ = recover() }()
		RunJsCode(code, sandbox)
	}()
	for _, key := range []string{"weapon", "meleeWeapon", "rangedWeapon", "skillWeapon", "weaponAttr", "meleeWeaponAttr", "rangedWeaponAttr", "skillWeaponAttr", "enemy", "char", "charMods", "meleeMods", "rangedMods", "skillMods"} {
		delete(sandbox, key)
	}
	result := CloneMap(sandbox)
	result["weapon"] = panels[0]
	return result
}

// ZeroMapWrap 将求和包装为缺键读 0 的记录。
func ZeroMapWrap(m map[string]any) ZeroMap {
	if m == nil {
		return ZeroMap{}
	}
	return ZeroMap(m)
}

// EnemySandbox 敌人沙箱。
func (e *Engine) EnemySandbox() map[string]any {
	enemy := AsMap(e.S["enemy"])
	return map[string]any{"等级": NumD(enemy["_等级"], 80), "def": Num(enemy["def"])}
}

// SelectedWeapon 选中武器。
func (e *Engine) SelectedWeapon() map[string]any {
	base := S(e.S, "baseName")
	meleeNames := map[string]bool{}
	for _, s := range AsList(e.S["meleeWeaponSkills"]) {
		if m, ok := s.(map[string]any); ok {
			meleeNames[S(m, "名称")] = true
		}
	}
	rangedNames := map[string]bool{}
	for _, s := range AsList(e.S["rangedWeaponSkills"]) {
		if m, ok := s.(map[string]any); ok {
			rangedNames[S(m, "名称")] = true
		}
	}
	skillNames := map[string]bool{}
	for _, s := range AsList(e.S["skillWeaponSkills"]) {
		if m, ok := s.(map[string]any); ok {
			skillNames[S(m, "名称")] = true
		}
	}
	if meleeNames[base] {
		return instOf(e.S, "meleeWeapon")
	}
	if rangedNames[base] {
		return instOf(e.S, "rangedWeapon")
	}
	if sw := AsMap(e.S["skillWeapon"]); sw != nil && (skillNames[base] || S(sw, "名称") == base) {
		return sw
	}
	return nil
}

// CalculateWeaponAttributes 武器属性（含充盈/召唤物转化）。
func (e *Engine) CalculateWeaponAttributes(weapon map[string]any, nocode, nochar bool) map[string]any {
	if weapon == nil {
		// 由调用方传入 nil 表示“默认选中”；显式空 map 表示无武器
		weapon = e.SelectedWeapon()
		if weapon == nil {
			for _, s := range AsList(e.S["allSkills"]) {
				if m, ok := s.(map[string]any); ok && S(m, "名称") == S(e.S, "baseName") {
					if _, ok := m["召唤物"].(map[string]any); ok {
						weapon = instOf(e.S, "meleeWeapon")
					}
					break
				}
			}
		}
	}
	if weapon != nil && S(weapon, "inherit") != "" {
		if S(weapon, "inherit") == "melee" {
			weapon = instOf(e.S, "meleeWeapon")
		} else {
			weapon = instOf(e.S, "rangedWeapon")
		}
	}
	var attrs map[string]any
	if nochar {
		attrs = map[string]any{}
	} else {
		attrs = e.CalculateAttributes(true, false, 0)
	}
	if weapon != nil && len(weapon) > 0 {
		prefix := S(weapon, "类型")
		attackBonus := e.GetTotal(prefix+"攻击", prefix, true) + e.GetTotal("攻击", prefix, true)
		physicalBonus := e.GetTotal("物理", prefix, true)
		critBonus := e.GetTotal(prefix+"暴击", prefix, true) + e.GetTotal("暴击", prefix, true)
		critDmgBonus := e.GetTotal(prefix+"暴伤", prefix, true) + e.GetTotal("暴伤", prefix, true)
		trigBonus := e.GetTotal(prefix+"触发", prefix, true) + e.GetTotal("触发", prefix, true)
		asBonus := e.GetTotal(prefix+"攻速", prefix, true) + e.GetTotal("攻速", prefix, true)
		multiBonus := e.GetTotal(prefix+"多重", prefix, true) + e.GetTotal("多重", prefix, true)
		dmgInc := e.GetTotal(prefix+"增伤", prefix, true) + e.GetTotal("增伤", prefix, true)
		reloadBonus := e.GetTotal(prefix+"装填", prefix, true) + e.GetTotal("装填", prefix, true)
		magBonus := e.GetTotal(prefix+"弹匣", prefix, true) + e.GetTotal("弹匣", prefix, true)
		ammoBonus := e.GetTotal(prefix+"弹药", prefix, true) + e.GetTotal("弹药", prefix, true)
		additional := e.GetTotal("追加伤害", "角色", true)
		weaponMul := e.GetTotal(prefix+"武器倍率", prefix, true) + e.GetTotal("武器倍率", prefix, true)
		indInc := (1+e.GetTotalMul(prefix+"独立增伤", prefix, true))*(1+e.GetTotalMul("独立增伤", prefix, true)) - 1
		char := charOf(e.S)
		modAttrBonus := e.GetTotal(S(char, "属性")+"MOD属性", "角色", true)
		if modAttrBonus > 0 {
			var elm []any
			for _, m := range listOf(e.S, "charMods") {
				if mm, ok := m.(map[string]any); ok && mm != nil && hasStr(ElmSeries, S(mm, "系列")) {
					elm = append(elm, mm)
				}
			}
			additional += modAttrBonus * e.GetModsBonus(elm, "追加伤害", "角色")
		}
		if hasPrefixStr(prefix, "同律") {
			lower := string([]rune(prefix)[2:])
			attackBonus += e.GetTotal(lower+"攻击", lower, false)
			critBonus += e.GetTotal(lower+"暴击", lower, false)
			critDmgBonus += e.GetTotal(lower+"暴伤", lower, false)
			trigBonus += e.GetTotal(lower+"触发", lower, false)
			asBonus += e.GetTotal(lower+"攻速", lower, false)
			dmgInc += e.GetTotal(lower+"增伤", lower, false)
			multiBonus += e.GetTotal(lower+"多重", lower, false)
			reloadBonus += e.GetTotal(lower+"装填", lower, false)
			magBonus += e.GetTotal(lower+"弹匣", lower, false)
			ammoBonus += e.GetTotal(lower+"弹药", lower, false)
			weaponMul += e.GetTotal(lower+"武器倍率", lower, false)
			indInc = (1+indInc)*(1+e.GetTotalMul(lower+"独立增伤", lower, false)) - 1
		}
		if asBonus > 2 {
			asBonus = 2
		}
		mastered := e.Mastered(S(weapon, "类别"))
		atkRatio := 1.0
		if mastered {
			if hasPrefixStr(prefix, "同律") {
				atkRatio = 1.4
			} else {
				atkRatio = 1.2
			}
		}
		attack := Num(weapon["基础攻击"]) * (1 + attackBonus) * atkRatio
		crit := Num(weapon["基础暴击"]) * (1 + critBonus)
		critDmg := Num(weapon["基础暴伤"]) * (1 + critDmgBonus)
		trig := Num(weapon["基础触发"]) * (1 + trigBonus)
		attackSpeed := NumD(weapon["射速"], 1) * (1 + asBonus)
		reload := Num(weapon["基础装填"]) / (1 + reloadBonus)
		magazine := Num(weapon["基础弹匣"]) * (1 + magBonus)
		ammo := Num(weapon["基础弹药"]) * (1 + ammoBonus)
		multi := 1 + multiBonus
		attack *= 1 + physicalBonus
		attack = float64(JsRound(attack*100)) / 100
		crit = float64(JsRound(crit*100)) / 100
		critDmg = float64(JsRound(critDmg*100)) / 100
		trig = float64(JsRound(trig*100)) / 100
		attackSpeed = float64(JsRound(attackSpeed*100)) / 100
		multi = float64(JsRound(multi*100)) / 100
		indInc = float64(JsRound(indInc*1000)) / 1000
		attrs["weapon"] = map[string]any{
			"攻击": attack, "暴击": crit, "暴伤": critDmg, "触发": trig,
			"攻速": attackSpeed, "多重": multi, "增伤": dmgInc, "独立增伤": indInc,
			"追加伤害": additional, "装填": reload, "弹匣": magazine, "弹药": ammo,
			"武器倍率":      weaponMul,
			"充盈转化":      1 + e.GetTotal("充盈转化", prefix, true),
			"召唤物攻击速度转化": e.GetTotal("召唤物攻击速度转化", prefix, true),
			"召唤物范围转化":   e.GetTotal("召唤物范围转化", prefix, true),
		}
	}
	if nochar {
		return attrs
	}
	fullness := 0.0
	for _, w := range e.FullnessWeapons() {
		trigRate, conv := e.WeaponFullness(w)
		fullness += math.Max(0, trigRate-1) * conv
	}
	attrs["充盈威力"] = Num(attrs["充盈威力"]) + fullness
	if melee := instOf(e.S, "meleeWeapon"); len(melee) > 0 && !isEmptyWeapon(melee) {
		attackSpeed, conv := e.WeaponSummonSpeed(melee)
		attrs["召唤物攻击速度"] = Num(attrs["召唤物攻击速度"]) + math.Max(0, attackSpeed)*conv
		attrs["召唤物范围"] = Num(attrs["召唤物范围"]) + e.GetTotal("召唤物范围转化", "近战", true)
	}
	if nocode {
		return attrs
	}
	if dyn := AsList(e.S["dynamicBuffs"]); len(dyn) > 0 {
		allPanels := e.AllWeaponPanels(weapon, AsMap(attrs["weapon"]))
		modAttrs := e.ModAttrSums()
		for _, b := range dyn {
			if buff, ok := b.(map[string]any); ok && buff != nil {
				if _, ok := buff["技能"].(string); ok {
					continue
				}
				attrs = e.ApplyCodeBuff(buff, attrs, allPanels, modAttrs)
			}
		}
	}
	return attrs
}

// FullnessWeapons 充盈武器列表。
func (e *Engine) FullnessWeapons() []map[string]any {
	var out []map[string]any
	if melee := instOf(e.S, "meleeWeapon"); len(melee) > 0 && !isEmptyWeapon(melee) {
		out = append(out, melee)
	}
	if ranged := instOf(e.S, "rangedWeapon"); len(ranged) > 0 && !isEmptyWeapon(ranged) {
		out = append(out, ranged)
	}
	if sw, ok := e.S["skillWeapon"].(map[string]any); ok && sw != nil && S(sw, "inherit") == "" {
		out = append(out, sw)
	}
	return out
}

// WeaponFullness 武器充盈（触发率，转化）。
func (e *Engine) WeaponFullness(weapon map[string]any) (float64, float64) {
	prefix := S(weapon, "类型")
	bonus := e.GetTotal(prefix+"触发", prefix, true) + e.GetTotal("触发", prefix, true)
	if hasPrefixStr(prefix, "同律") {
		lower := string([]rune(prefix)[2:])
		bonus += e.GetTotal(lower+"触发", lower, false)
	}
	rate := float64(JsRound(Num(weapon["基础触发"])*(1+bonus)*100)) / 100
	return rate, 1 + e.GetTotal("充盈转化", prefix, true)
}

// WeaponSummonSpeed 武器召唤攻速。
func (e *Engine) WeaponSummonSpeed(weapon map[string]any) (float64, float64) {
	prefix := S(weapon, "类型")
	bonus := e.GetTotal(prefix+"攻速", prefix, true) + e.GetTotal("攻速", prefix, true)
	if hasPrefixStr(prefix, "同律") {
		lower := string([]rune(prefix)[2:])
		bonus += e.GetTotal(lower+"攻速", lower, false)
	}
	asb := bonus
	if asb > 2 {
		asb = 2
	}
	return NumD(weapon["射速"], 1) * (1 + asb), e.GetTotal("召唤物攻击速度转化", prefix, true)
}

// WeaponPanels 面板表。
func (e *Engine) WeaponPanels() map[string]any {
	panels := map[string]any{}
	melee := instOf(e.S, "meleeWeapon")
	ranged := instOf(e.S, "rangedWeapon")
	panels["远程"] = e.CalculateWeaponAttributes(ranged, true, true)["weapon"]
	panels["近战"] = e.CalculateWeaponAttributes(melee, true, true)["weapon"]
	if sw, ok := e.S["skillWeapon"].(map[string]any); ok && sw != nil {
		switch S(sw, "inherit") {
		case "melee":
			panels["同律"] = panels["近战"]
		case "ranged":
			panels["同律"] = panels["远程"]
		default:
			panels["同律"] = e.CalculateWeaponAttributes(sw, true, true)["weapon"]
		}
	}
	panels["melee"] = panels["近战"]
	panels["ranged"] = panels["远程"]
	if v, ok := panels["同律"]; ok {
		panels["skill"] = v
	}
	for _, sk := range AsList(e.S["weaponSkills"]) {
		if skill, ok := sk.(map[string]any); ok && skill != nil {
			slot := slot2(S(skill, "武器"))
			if slot != "" {
				if p, ok := panels[slot]; ok {
					panels[S(skill, "名称")] = p
				}
			}
		}
	}
	return panels
}

func slot2(s string) string {
	r := []rune(s)
	if len(r) > 2 {
		r = r[:2]
	}
	return string(r)
}

// WeaponsKeys 面板键序列。
func (e *Engine) WeaponsKeys() []string {
	keys := []string{"近战", "远程"}
	if _, ok := e.S["skillWeapon"].(map[string]any); ok {
		keys = append(keys, "同律")
	}
	keys = append(keys, "melee", "ranged")
	if _, ok := e.S["skillWeapon"].(map[string]any); ok {
		keys = append(keys, "skill")
	}
	for _, sk := range AsList(e.S["weaponSkills"]) {
		if skill, ok := sk.(map[string]any); ok && skill != nil {
			slot := slot2(S(skill, "武器"))
			if slot == "近战" || slot == "远程" || slot == "同律" {
				keys = append(keys, S(skill, "名称"))
			}
		}
	}
	return keys
}

// SkillTables 技能表（含别名）。
func (e *Engine) SkillTables(attrs map[string]any) map[string][]map[string]any {
	var rules []CondRule
	for _, r := range e.ConditionalList() {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			re = regexp.MustCompile(regexp.QuoteMeta(r.Pattern))
		}
		rules = append(rules, CondRule{Re: re, Props: r.Props})
	}
	tables := map[string][]map[string]any{}
	for _, s := range AsList(e.S["allSkills"]) {
		if skill, ok := s.(map[string]any); ok && skill != nil {
			tables[S(skill, "safeName")] = LevelSkillFieldsWithAttr(skill, attrs, rules, IsMultAttr)
		}
	}
	if aliases, ok := e.S["skill_aliases"].(map[string]any); ok {
		for _, alias := range []string{"E", "e", "Q", "q", "P", "p"} {
			upper := strings.ToUpper(alias)
			if safe, ok := aliases[upper].(string); ok && safe != "" {
				if t, ok := tables[safe]; ok {
					tables[alias] = t
				}
			}
		}
	}
	return tables
}

// CondRuleOut 条件规则输出。
type CondRuleOut struct {
	Skill   string
	Pattern string
	Flags   string
	Props   map[string]any
}

// ConditionalList 复刻 getConditionalBuffPropsList。
func (e *Engine) ConditionalList() []CondRuleOut {
	grouped := map[string]map[string]any{}
	var order []string
	for _, b := range AsList(e.S["buffs"]) {
		buff, ok := b.(map[string]any)
		if !ok || buff == nil {
			continue
		}
		pattern, ok := buff["技能"].(string)
		if !ok {
			continue
		}
		if _, ok := grouped[pattern]; !ok {
			grouped[pattern] = map[string]any{}
			order = append(order, pattern)
		}
		for _, prop := range BuffProperties(buff) {
			value, ok := buff[prop]
			if !ok || !IsNum(value) || Num(value) == 0 {
				continue
			}
			if IsMultAttr(prop) {
				grouped[pattern][prop] = (1+Num(grouped[pattern][prop]))*(1+Num(value)) - 1
			} else {
				grouped[pattern][prop] = Num(grouped[pattern][prop]) + Num(value)
			}
		}
	}
	var rules []CondRuleOut
	for _, p := range order {
		rules = append(rules, CondRuleOut{Skill: p, Pattern: p, Props: grouped[p]})
	}
	rules = append(rules, e.codeConditionals()...)
	return rules
}

func (e *Engine) codeConditionals() []CondRuleOut {
	var scoped []map[string]any
	for _, b := range AsList(e.S["dynamicBuffs"]) {
		if buff, ok := b.(map[string]any); ok && buff != nil {
			if _, ok := buff["技能"].(string); ok {
				scoped = append(scoped, buff)
			}
		}
	}
	if len(scoped) == 0 {
		return nil
	}
	base := e.CalculateWeaponAttributes(nil, false, false)
	var out []CondRuleOut
	for _, buff := range scoped {
		snapshot := CloneMap(base)
		result := e.ApplyCodeBuff(buff, snapshot, e.AllWeaponPanels(nil, nil), e.ModAttrSums())
		props := map[string]any{}
		for key, value := range result {
			if !IsNum(value) {
				continue
			}
			before, ok := snapshot[key]
			if !ok || !IsNum(before) || Num(value) == Num(before) {
				continue
			}
			if IsMultAttr(key) {
				if abs(1+Num(before)) < 1e-12 {
					continue
				}
				props[key] = (1+Num(value))/(1+Num(before)) - 1
			} else {
				props[key] = Num(value) - Num(before)
			}
		}
		out = append(out, CondRuleOut{Skill: S(buff, "技能"), Pattern: S(buff, "技能"), Props: props})
	}
	return out
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// CombatInfo 战斗状态。
func (e *Engine) CombatInfo() map[string]any {
	enemy := AsMap(e.S["enemy"])
	shield := Num(enemy["currentShield"])
	hpType := "生命"
	if shield > 0 {
		hpType = "护盾"
	}
	coef := map[string]any{}
	for k, v := range HpTypeCoefficients {
		coef[k] = v
	}
	dmg := map[string]any{}
	for k, v := range HpTypeDMG {
		dmg[k] = v
	}
	return map[string]any{
		"enemyDef": Num(enemy["def"]), "enemyLevel": NumD(enemy["_等级"], 80),
		"enemyShield": shield, "currentHPType": hpType,
		"resistance": Num(e.S["enemyResistance"]), "triggerBonus": e.GetTotal("触发倍率", "角色", true),
		"imbalance": isTrue(e.S["imbalance"]), "charLevel": NumD(charOf(e.S)["_等级"], 80),
		"hpPercent":          NumD(e.S["hpPercent"], 1),
		"hpTypeCoefficients": coef, "hpTypeDMG": dmg,
		"hasArrowRainMod": hasModID(listOf(e.S, "rangedMods"), 43604),
	}
}

func hasModID(mods []any, id int) bool {
	for _, m := range mods {
		if mm, ok := m.(map[string]any); ok && mm != nil && int(Num(mm["id"])) == id {
			return true
		}
	}
	return false
}

// WeaponsInfo 武器信息。
func (e *Engine) WeaponsInfo() map[string]any {
	info := map[string]any{}
	panels := e.WeaponPanels()
	_ = panels
	for _, key := range e.WeaponsKeys() {
		inst := e.instanceForKey(key)
		if inst == nil || len(inst) == 0 {
			info[key] = nil
			continue
		}
		info[key] = map[string]any{
			"伤害类型": inst["伤害类型"], "类型": inst["类型"], "inherit": inst["inherit"],
			"atk": inst["atk"], "视为": inst["视为"], "isSkillWeapon": isTrue(inst["_isSkillWeapon"]),
		}
	}
	return info
}

// WeaponBases 武器基础值。
func (e *Engine) WeaponBases() map[string]any {
	out := map[string]any{}
	picked := []string{"基础攻击", "基础暴击", "基础暴伤", "基础触发", "射速", "基础装填", "基础弹匣", "基础弹药"}
	for _, key := range e.WeaponsKeys() {
		var inst map[string]any
		switch key {
		case "近战", "melee":
			inst = instOf(e.S, "meleeWeapon")
		case "远程", "ranged":
			inst = instOf(e.S, "rangedWeapon")
		case "同律", "skill":
			sw := AsMap(e.S["skillWeapon"])
			if sw != nil && S(sw, "inherit") != "" {
				if S(sw, "inherit") == "melee" {
					inst = instOf(e.S, "meleeWeapon")
				} else {
					inst = instOf(e.S, "rangedWeapon")
				}
			} else {
				inst = sw
			}
		default:
			continue
		}
		if inst == nil || len(inst) == 0 {
			out[key] = nil
			continue
		}
		row := map[string]any{}
		for _, k := range picked {
			row[k] = inst[k]
		}
		out[key] = row
	}
	return out
}

func (e *Engine) instanceForKey(key string) map[string]any {
	switch key {
	case "近战", "melee":
		return instOf(e.S, "meleeWeapon")
	case "远程", "ranged":
		return instOf(e.S, "rangedWeapon")
	case "同律", "skill":
		return AsMap(e.S["skillWeapon"])
	}
	for _, sk := range AsList(e.S["weaponSkills"]) {
		if skill, ok := sk.(map[string]any); ok && skill != nil && S(skill, "名称") == key {
			slot := slot2(S(skill, "武器"))
			switch slot {
			case "近战":
				return instOf(e.S, "meleeWeapon")
			case "远程":
				return instOf(e.S, "rangedWeapon")
			case "同律":
				return AsMap(e.S["skillWeapon"])
			}
		}
	}
	return nil
}

// FieldTags 复刻 getFieldTags。
func (e *Engine) FieldTags(baseName, fieldName, ctxSafe string) []any {
	if fieldName == "" {
		return nil
	}
	tables := e.skillTbl
	if ctxSafe != "" && tables != nil {
		for _, f := range tables[ctxSafe] {
			if containsStr(S(f, "safeName"), fieldName) || containsStr(S(f, "名称"), fieldName) {
				if tags, ok := f["tag"].([]any); ok && len(tags) > 0 {
					return append([]any{}, tags...)
				}
			}
		}
	}
	if tables != nil {
		for _, entry := range AsList(e.S["skill_name_list"]) {
			if em, ok := entry.(map[string]any); ok && S(em, "名称") == baseName {
				for _, f := range tables[S(em, "safeName")] {
					if containsStr(S(f, "safeName"), fieldName) || containsStr(S(f, "名称"), fieldName) {
						if tags, ok := f["tag"].([]any); ok && len(tags) > 0 {
							return append([]any{}, tags...)
						}
					}
				}
			}
		}
	}
	return nil
}

// AttackTypeBonus 复刻 getWeaponAttackTypeBonus 的加成汇总段。
func (e *Engine) AttackTypeBonus(weaponType, prefix, attribute string) float64 {
	scope := ScopeOf(weaponType)
	multiplicative := hasSuffixStr(attribute, "独立增伤")
	var bonus func(attr, pre string, includeMods bool) float64
	bonus = func(attr, pre string, includeMods bool) float64 {
		if multiplicative {
			return e.GetTotalMul(attr, pre, includeMods)
		}
		return e.GetTotal(attr, pre, includeMods)
	}
	total := bonus(weaponType+prefix+attribute, scope, true)
	if containsStr(weaponType, "近战") {
		total += bonus(prefix+attribute, scope, true)
	}
	if hasPrefixStr(weaponType, "同律") {
		lower := string([]rune(weaponType)[2:])
		lowerScope := ScopeOf(lower)
		total += bonus(lower+prefix+attribute, lowerScope, false)
		if containsStr(lower, "近战") {
			total += bonus(prefix+attribute, lowerScope, false)
		}
		total += e.ModsScopeBonus(lower+prefix+attribute, scope, multiplicative)
	}
	return total
}

func hasSuffixStr(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// ModsScopeBonus MOD 作用域加成。
func (e *Engine) ModsScopeBonus(attribute, scope string, multiplicative bool) float64 {
	t := e.BonusTable()
	if !multiplicative {
		if scope != "" {
			return t.ModsByScope[scope][attribute]
		}
		return t.ModsAll[attribute]
	}
	product := 1.0
	for _, mod := range e.ScopedMods(scope) {
		if v, ok := mod[attribute]; ok && IsNum(v) {
			product *= 1 + Num(v)
		}
	}
	return product - 1
}

// ContextPanels 求值用面板。
func (e *Engine) ContextPanels(current map[string]any) map[string]any {
	panels := e.WeaponPanels()
	selected := e.SelectedWeapon()
	selectedPanel, _ := current["weapon"].(map[string]any)
	if selected != nil && len(selected) > 0 && selectedPanel != nil {
		for _, key := range e.WeaponsKeys() {
			inst := e.instanceForKey(key)
			if inst != nil && mapsEqual(inst, selected) {
				panels[key] = selectedPanel
			}
		}
	}
	return panels
}

func mapsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || !valuesEqual(v, bv) {
			return false
		}
	}
	return true
}

// DamageContext 装配求值上下文。
func (e *Engine) DamageContext(attrs map[string]any) *DamageContext {
	var current map[string]any
	if attrs != nil {
		current = attrs
	} else {
		current = e.CalculateWeaponAttributes(nil, false, false)
	}
	tables := e.SkillTables(current)
	e.skillTbl = tables
	panels := e.ContextPanels(current)
	eng := e
	bonusFn := func(key string, fieldName V, attribute, weaponType string, tags []any, ctxSafe string) float64 {
		realTags := tags
		if realTags == nil {
			if ft := eng.FieldTags(key, strOrEmpty(fieldName), ctxSafe); ft != nil {
				realTags = ft
			}
		}
		var prefix string
		for _, tt := range AttackTypeTags {
			name, _ := tt[0].(string)
			names, _ := tt[1].([]string)
			for _, n := range names {
				if hasStr(realTagsToStrings(realTags), n) {
					prefix = name
					break
				}
			}
			if prefix != "" {
				break
			}
		}
		if prefix == "" {
			var view string
			if key == "近战" || key == "melee" {
				view = S(instOf(eng.S, "meleeWeapon"), "视为")
			} else if key == "远程" || key == "ranged" {
				view = S(instOf(eng.S, "rangedWeapon"), "视为")
			} else {
				view = S(AsMap(eng.S["skillWeapon"]), "视为")
			}
			if view != "" {
				for _, tt := range AttackTypeTags {
					name, _ := tt[0].(string)
					names, _ := tt[1].([]string)
					if view == name || hasStr(names, view) {
						prefix = name
						break
					}
				}
			}
		}
		if prefix == "" {
			return 0
		}
		wt := weaponType
		if wt == "" {
			wt = key
		}
		return eng.AttackTypeBonus(wt, prefix, attribute)
	}
	var rules []CondRuleInput
	for _, r := range e.ConditionalList() {
		rules = append(rules, CondRuleInput{Skill: r.Pattern, Flags: r.Flags, Props: r.Props})
	}
	var skillNames []map[string]any
	for _, entry := range AsList(e.S["skill_name_list"]) {
		if em, ok := entry.(map[string]any); ok {
			skillNames = append(skillNames, em)
		}
	}
	aliases := map[string]string{}
	if am, ok := e.S["skill_aliases"].(map[string]any); ok {
		for k, v := range am {
			if s, ok := v.(string); ok {
				aliases[k] = s
			}
		}
	}
	tbls := map[string][]map[string]any{}
	for k, v := range tables {
		tbls[k] = v
	}
	coef := map[string]any{}
	for k, v := range HpTypeCoefficients {
		coef[k] = v
	}
	dmg := map[string]any{}
	for k, v := range HpTypeDMG {
		dmg[k] = v
	}
	return NewDamageContext(current, panels, tbls, skillNames, aliases, rules, map[string]any{},
		AsMap(e.WeaponsInfo()), AsMap(e.WeaponBases()), e.CombatInfo(), S(e.S, "baseName"), bonusFn)
}

func realTagsToStrings(tags []any) []string {
	var out []string
	for _, t := range tags {
		if s, ok := t.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// CharPanel 角色面板。
func (e *Engine) CharPanel() map[string]any {
	attrs := e.CalculateWeaponAttributes(nil, false, false)
	out := map[string]any{}
	for k, v := range attrs {
		if k != "weapon" {
			out[k] = v
		}
	}
	return out
}

// WeaponPanel 武器面板。
func (e *Engine) WeaponPanel(slot string) map[string]any {
	norm := slot
	switch slot {
	case "melee":
		norm = "近战"
	case "ranged":
		norm = "远程"
	case "skill":
		norm = "同律"
	}
	var weapon map[string]any
	switch norm {
	case "近战", "melee":
		weapon = instOf(e.S, "meleeWeapon")
	case "远程", "ranged":
		weapon = instOf(e.S, "rangedWeapon")
	case "同律", "skill":
		weapon = AsMap(e.S["skillWeapon"])
	default:
		for _, sk := range AsList(e.S["weaponSkills"]) {
			if skill, ok := sk.(map[string]any); ok && skill != nil && S(skill, "名称") == norm {
				weapon = e.instanceForKey(norm)
				break
			}
		}
		if weapon == nil {
			for _, w := range []map[string]any{instOf(e.S, "meleeWeapon"), instOf(e.S, "rangedWeapon"), AsMap(e.S["skillWeapon"])} {
				if w != nil && S(w, "名称") == norm {
					weapon = w
					break
				}
			}
		}
	}
	if weapon == nil || len(weapon) == 0 {
		return nil
	}
	return AsMap(e.CalculateWeaponAttributes(weapon, true, true)["weapon"])
}

// SkillLevelAt 复刻 getSkillLevel。
func (e *Engine) SkillLevelAt(index int) int {
	if trio, ok := e.S["skillLevel"].([]int); ok {
		return ResolveSkillLevel(trio, index)
	}
	return ResolveSkillLevel(nil, index)
}

// SelectedSkillLevel 复刻 selectedSkillLevel。
func (e *Engine) SelectedSkillLevel() int {
	index := -1
	for i, s := range AsList(e.S["skills"]) {
		if m, ok := s.(map[string]any); ok && S(m, "名称") == S(e.S, "baseName") {
			index = i
			break
		}
	}
	if index < 0 {
		index = 2
	}
	if trio, ok := e.S["skillLevel"].([]int); ok {
		return ResolveSkillLevel(trio, index)
	}
	return ResolveSkillLevel(nil, index)
}

// SkillLevelsFinal 最终技能等级。
func (e *Engine) SkillLevelsFinal() [][2]any {
	finals := AsList(e.S["skillLevelsFinal"])
	var out [][2]any
	for i, s := range AsList(e.S["skills"]) {
		var lv any
		if i < len(finals) {
			lv = finals[i]
		}
		if m, ok := s.(map[string]any); ok {
			out = append(out, [2]any{S(m, "名称"), lv})
		}
	}
	return out
}

// SkillFields 技能面板数值。
func (e *Engine) SkillFields(skillName string) []map[string]any {
	attrs := e.CalculateWeaponAttributes(nil, false, false)
	ctx := e.DamageContext(attrs)
	var safe string
	for _, entry := range AsList(e.S["skill_name_list"]) {
		if em, ok := entry.(map[string]any); ok && (S(em, "名称") == skillName || S(em, "safeName") == skillName) {
			safe = S(em, "safeName")
			break
		}
	}
	if safe == "" {
		return []map[string]any{}
	}
	var out []map[string]any
	for _, f := range ctx.Tables[safe] {
		row := map[string]any{}
		for _, k := range []string{"名称", "safeName", "值", "值2", "格式", "基础", "tag", "伤害类型"} {
			if v, ok := f[k]; ok {
				row[k] = v
			}
		}
		out = append(out, row)
	}
	return out
}

// CustomTables 自定义变量/函数表。
func (e *Engine) CustomTables() (map[string]string, map[string]CustomFunc) {
	variables := map[string]string{}
	functions := map[string]CustomFunc{}
	for _, entry := range AsList(e.S["customVariables"]) {
		pair, _ := entry.([]any)
		if len(pair) < 2 {
			continue
		}
		key, _ := pair[0].(string)
		value, _ := pair[1].(string)
		key = trimSpace(key)
		value = trimSpace(value)
		if key == "" || value == "" || !ValidVarKey(key) {
			continue
		}
		if name, params, ok := ParseFuncDef(key); ok {
			functions[name] = CustomFunc{Params: params, Body: value}
		} else {
			variables[key] = value
		}
	}
	return variables, functions
}

func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// Calculate 复刻 calculateOneTime。
func (e *Engine) Calculate(target string) float64 {
	if enemy, ok := e.S["enemy"].(map[string]any); ok && enemy != nil {
		enemy["currentHP"] = Num(enemy["hp"])
		if es := Num(enemy["es"]); es != 0 {
			enemy["currentShield"] = es
		} else {
			enemy["currentShield"] = 0
		}
	}
	attrs := e.CalculateWeaponAttributes(nil, false, false)
	ctx := e.DamageContext(attrs)
	variables, functions := e.CustomTables()
	t := target
	if t == "" {
		t = S(e.S, "targetFunction")
	}
	if t == "" {
		t = "伤害"
	}
	result := NewEvaluator(attrs, nil, variables, functions, ctx.Panels, ctx).Eval(t, nil, nil, nil, nil)
	if math.IsNaN(result) || math.IsInf(result, 0) {
		return 0
	}
	return result
}

var funcDefRe = regexp.MustCompile(`^([a-zA-Z_\x{4e00}-\x{9fa5}·\[][a-zA-Z0-9_\x{4e00}-\x{9fa5}·\]]*)\s*\(([^()]*)\)\s*$`)
var varNameRe = regexp.MustCompile(`^[a-zA-Z_\x{4e00}-\x{9fa5}·\[][a-zA-Z0-9_\x{4e00}-\x{9fa5}·\]]*$`)
var paramRe = regexp.MustCompile(`^[a-zA-Z_\x{4e00}-\x{9fa5}·][a-zA-Z0-9_\x{4e00}-\x{9fa5}·\]]*$`)

// ParseFuncDef 复刻 parseCustomFunctionDefinition。
func ParseFuncDef(key string) (string, []string, bool) {
	m := funcDefRe.FindStringSubmatch(trimSpace(key))
	if m == nil {
		return "", nil, false
	}
	var params []string
	for _, p := range splitComma(m[2]) {
		if t := trimSpace(p); t != "" {
			params = append(params, t)
		}
	}
	return m[1], params, true
}

// ValidVarKey 复刻 validateCustomVariableKey。
func ValidVarKey(key string) bool {
	text := trimSpace(key)
	if text == "" || containsStr(text, "::") || containsStr(text, ".") {
		return false
	}
	if name, params, ok := ParseFuncDef(text); ok {
		if !varNameRe.MatchString(name) {
			return false
		}
		seen := map[string]bool{}
		for _, param := range params {
			if !paramRe.MatchString(param) || seen[param] {
				return false
			}
			seen[param] = true
		}
		return true
	}
	if !varNameRe.MatchString(text) {
		return false
	}
	node, err := ParseAST(text, nil)
	if err != nil {
		return false
	}
	return node.Type == NodeProperty && node.Name == text && node.Namespace == ""
}
