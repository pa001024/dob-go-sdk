// 技能伤害结算链：CharBuild.evaluateSkill/getDamage/getDef 的纯数据复刻。
package dob

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// WeaponDamageFieldBase [近战]/[远程]/[同律] → 面板基。
var WeaponDamageFieldBase = map[string]string{"[近战]": "近战", "[远程]": "远程", "[同律]": "同律"}

// PhysicalConverts 物理转属键。
var PhysicalConverts = []string{"转切割", "转贯穿", "转震荡", "转灾厄"}

// WeaponAttrBases 面板属性 → 武器基础属性。
var WeaponAttrBases = map[string]string{
	"攻击": "基础攻击", "暴击": "基础暴击", "暴伤": "基础暴伤", "触发": "基础触发",
	"攻速": "射速", "装填": "基础装填", "弹匣": "基础弹匣", "弹药": "基础弹药",
}

// FlatMap 平值映射。
var FlatMap = map[string]string{"固定攻击": "攻击", "固定生命": "生命"}

var aliasIdx = map[string]int{"E": 0, "e": 0, "Q": 1, "q": 1, "P": 2, "p": 2}

// IsMultAttr 对齐 leveled/minusAttr.isMultiplicativeAttr。
func IsMultAttr(attr string) bool {
	if attr == "无视防御" || attr == "技能无视防御" || attr == "技能倍率乘数" {
		return true
	}
	return strings.HasSuffix(attr, "独立增伤")
}

// CondRuleInput 条件规则输入（pattern 源码 + props）。
type CondRuleInput struct {
	Skill string
	Flags string
	Props map[string]any
}

// DamageContext 是一次真机快照的全部确定表 + 结算公式。
type DamageContext struct {
	Attrs       map[string]any
	AttackBonus map[string]any
	Panels      map[string]any
	Tables      map[string][]map[string]any
	SkillNames  []map[string]any
	Aliases     map[string]string
	Weapons     map[string]any
	WeaponBases map[string]any
	Combat      map[string]any
	BaseName    string
	BonusFn     func(key string, fieldName V, attr string, weaponType string, tags []any, ctxSafe string) float64

	Rules     []CondRule
	condCache map[string]map[string]any
	dmgCache  map[string]map[string]any
}

// NewDamageContext 构造（含规则编译）。
func NewDamageContext(attrs, panels map[string]any, tables map[string][]map[string]any, skillNames []map[string]any, aliases map[string]string, rules []CondRuleInput, attackBonus, weapons, weaponBases, combat map[string]any, baseName string, bonusFn func(string, V, string, string, []any, string) float64) *DamageContext {
	c := &DamageContext{
		Attrs: attrs, Panels: panels, Tables: tables, SkillNames: skillNames,
		Aliases: aliases, AttackBonus: attackBonus, Weapons: weapons,
		WeaponBases: weaponBases, Combat: combat, BaseName: baseName, BonusFn: bonusFn,
		condCache: map[string]map[string]any{}, dmgCache: map[string]map[string]any{},
	}
	for _, r := range rules {
		pattern := r.Skill
		prefix := ""
		for _, ch := range r.Flags {
			switch ch {
			case 'i':
				prefix += "(?i)"
			case 'm':
				prefix += "(?m)"
			case 's':
				prefix += "(?s)"
			}
		}
		var re *regexp.Regexp
		var err error
		re, err = regexp.Compile(prefix + pattern)
		if err != nil {
			re = regexp.MustCompile(regexp.QuoteMeta(r.Skill))
		}
		c.Rules = append(c.Rules, CondRule{Re: re, Props: r.Props})
	}
	return c
}

// ResolveSkillSafe 对齐 resolveSkillContext。
func (c *DamageContext) ResolveSkillSafe(namespace string) string {
	if namespace == "" {
		return ""
	}
	if idx, ok := aliasIdx[namespace]; ok {
		ordered := []string{c.Aliases["E"], c.Aliases["Q"], c.Aliases["P"]}
		if idx < 0 || idx > 2 {
			idx = 0
		}
		return ordered[idx]
	}
	for _, e := range c.SkillNames {
		if S(e, "名称") == namespace || S(e, "safeName") == namespace {
			return S(e, "safeName")
		}
	}
	return ""
}

func (c *DamageContext) table(safe string) []map[string]any {
	if safe == "" {
		return nil
	}
	return c.Tables[safe]
}

// FindField 对齐 getSkillAttr：safeName.includes(query)。
func (c *DamageContext) FindField(fields []map[string]any, query string) map[string]any {
	if query == "" {
		return nil
	}
	for _, f := range fields {
		if strings.Contains(S(f, "safeName"), query) {
			return f
		}
	}
	return nil
}

// FindFieldWide safeName 或 名称 includes(query)。
func (c *DamageContext) FindFieldWide(fields []map[string]any, query string) map[string]any {
	if query == "" {
		return nil
	}
	for _, f := range fields {
		if strings.Contains(S(f, "safeName"), query) || strings.Contains(S(f, "名称"), query) {
			return f
		}
	}
	return nil
}

// GetSkillAttr 取技能字段。
func (c *DamageContext) GetSkillAttr(fieldName, base, ctxSafe string) map[string]any {
	if ctxSafe != "" {
		return c.FindField(c.table(ctxSafe), fieldName)
	}
	var table []map[string]any
	if base != "" {
		table = c.Tables[base]
	} else {
		table = c.Tables[c.BaseName]
	}
	return c.FindField(table, fieldName)
}

// CondProps 对齐 getConditionalBuffProps。
func (c *DamageContext) CondProps(fieldName string) map[string]any {
	if fieldName == "" {
		return nil
	}
	if r, ok := c.condCache[fieldName]; ok {
		return r
	}
	var result map[string]any
	for _, rule := range c.Rules {
		if !rule.Re.MatchString(fieldName) {
			continue
		}
		for prop, value := range rule.Props {
			if result == nil {
				result = map[string]any{}
			}
			if IsMultAttr(prop) {
				result[prop] = (1+Num(result[prop]))*(1+Num(value)) - 1
			} else {
				result[prop] = Num(result[prop]) + Num(value)
			}
		}
	}
	c.condCache[fieldName] = result
	return result
}

// WeaponFieldBase 武器字段基。
func (c *DamageContext) WeaponFieldBase(fieldName string) string {
	return WeaponDamageFieldBase[fieldName]
}

// WeaponDamageBase 武器伤害基。
func (c *DamageContext) WeaponDamageBase(base, fieldName, ctxSafe string) string {
	if keyword, ok := WeaponDamageFieldBase[fieldName]; ok {
		return keyword
	}
	key := base
	if key == "" {
		key = c.BaseName
	}
	if _, ok := c.Panels[key]; !ok || fieldName == "" {
		return ""
	}
	var table []map[string]any
	if ctxSafe != "" {
		table = c.table(ctxSafe)
	} else {
		table = c.Tables[key]
	}
	if field := c.FindField(table, fieldName); field != nil {
		nm := S(field, "名称")
		if strings.HasSuffix(nm, "伤害") || strings.HasSuffix(nm, "伤害倍率") {
			return key
		}
	}
	return ""
}

func (c *DamageContext) panelFor(base string) map[string]any {
	key := base
	if key == "" {
		key = c.BaseName
	}
	if p, ok := c.Panels[key].(map[string]any); ok {
		return p
	}
	return nil
}

// TempWeaponAttr 对齐 getTemporaryWeaponAttr。
func (c *DamageContext) TempWeaponAttr(base, fieldName string, temp map[string]any, ctxSafe string) map[string]any {
	weaponBase := c.WeaponDamageBase(base, fieldName, ctxSafe)
	var panel map[string]any
	if weaponBase != "" {
		panel = c.panelFor(weaponBase)
	} else {
		panel = c.panelFor(base)
	}
	if weaponBase == "" || panel == nil || len(temp) == 0 {
		return panel
	}
	eff, _ := c.WeaponBases[weaponBase].(map[string]any)
	if eff == nil {
		eff = map[string]any{}
	}
	out := CloneMap(panel)
	for attr, value := range temp {
		pv, ok := panel[attr]
		if !ok || !IsNum(pv) {
			continue
		}
		baseAttr := WeaponAttrBases[attr]
		baseValue := 1.0
		if baseAttr != "" {
			baseValue = Num(eff[baseAttr])
		}
		out[attr] = Num(pv) + baseValue*Num(value)
	}
	return out
}

// SummonAttrs 对齐 getSummonAttrs。
func (c *DamageContext) SummonAttrs(base string, temp map[string]any, fieldName, ctxSafe string) map[string]any {
	key := base
	if key == "" {
		key = c.BaseName
	}
	attrs := c.Attrs
	tableSafe := ctxSafe
	if tableSafe == "" {
		for _, e := range c.SkillNames {
			if S(e, "名称") == key {
				tableSafe = S(e, "safeName")
				break
			}
		}
	}
	var summonField map[string]any
	if fieldName != "" {
		summonField = c.FindFieldWide(c.table(tableSafe), fieldName)
	}
	ratio := 1.0
	if summonField != nil {
		if tags, ok := summonField["tag"].([]any); ok {
			for _, t := range tags {
				if s, ok := t.(string); ok && s == "召唤物" {
					ratio = NumD(attrs["召唤物属性继承比例"], 1)
					break
				}
			}
		}
	}
	current := attrs
	if ratio != 1 {
		current = CloneMap(attrs)
		current["攻击"] = Num(attrs["攻击"]) * ratio
		current["昂扬"] = Num(attrs["昂扬"]) * ratio
		current["背水"] = Num(attrs["背水"]) * ratio
	}
	var conditional map[string]any
	if fieldName != "" {
		conditional = c.CondProps(fieldName)
	}
	scoped := current
	if len(conditional) > 0 {
		scoped = CloneMap(current)
		for attr, value := range conditional {
			if IsMultAttr(attr) {
				if !IsNum(scoped[attr]) {
					continue
				}
				scoped[attr] = (1+Num(scoped[attr]))*(1+Num(value)) - 1
				continue
			}
			target := FlatMap[attr]
			if target == "" {
				target = attr
			}
			if !IsNum(scoped[target]) {
				continue
			}
			scoped[target] = Num(scoped[target]) + Num(value)
		}
	}
	if len(temp) == 0 {
		return scoped
	}
	fieldAttrs := CloneMap(scoped)
	tempPanel := c.TempWeaponAttr(base, fieldName, temp, ctxSafe)
	hasWeaponBase := c.WeaponDamageBase(base, fieldName, ctxSafe) != ""
	for attr, value := range temp {
		if hasWeaponBase {
			if tv, ok := tempPanel[attr]; ok && IsNum(tv) {
				continue
			}
		}
		target := FlatMap[attr]
		if target == "" {
			target = attr
		}
		av, ok := attrs[target]
		if !ok || !IsNum(av) {
			panic(&AstError{Msg: "找不到临时属性: \"" + attr + "\""})
		}
		fieldAttrs[target] = Num(fieldAttrs[target]) + Num(value)
	}
	return fieldAttrs
}

// LevelReduceRate 等级减伤。
func (c *DamageContext) LevelReduceRate(enemyLevel float64) float64 {
	if enemyLevel < 200 {
		return 1
	}
	return 1 / (1 + (enemyLevel-190)*0.05)
}

// DefenseMultiplier 对齐 calculateDefenseMultiplier。
func (c *DamageContext) DefenseMultiplier(merged map[string]any, isSkill bool) float64 {
	enemyLevel := Num(c.Combat["enemyLevel"])
	if enemyLevel == 0 {
		enemyLevel = 80
	}
	if Num(c.Combat["enemyShield"]) > 0 {
		return c.LevelReduceRate(enemyLevel)
	}
	charLevel := Num(c.Combat["charLevel"])
	if charLevel == 0 {
		charLevel = 80
	}
	levelDiff := math.Min(80, enemyLevel) - charLevel
	if levelDiff < 0 {
		levelDiff = 0
	}
	if levelDiff > 20 {
		levelDiff = 20
	}
	var ignore float64
	if isSkill {
		ignore = Num(merged["技能无视防御"]) + Num(merged["无视防御"])
	} else {
		ignore = Num(merged["无视防御"])
	}
	defense := Num(c.Combat["enemyDef"]) * (1 - ignore)
	reduceRate := defense / (300 + defense - levelDiff*10)
	v := (1 - reduceRate) * c.LevelReduceRate(enemyLevel)
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return v
}

// GetDef 对齐 getDef。
func (c *DamageContext) GetDef(base, fieldName string, temp map[string]any, ctxSafe string) float64 {
	key := base
	if key == "" {
		key = c.BaseName
	}
	_, isWeapon := c.Panels[key]
	if pv, ok := c.Panels[key]; !ok || pv == nil {
		isWeapon = false
	} else {
		isWeapon = true
	}
	merged := c.Attrs
	if len(temp) > 0 {
		merged = c.SummonAttrs(key, temp, fieldName, ctxSafe)
	}
	return c.DefenseMultiplier(merged, !isWeapon)
}

var formatTokenRe = regexp.MustCompile(`\{%\}|\{\}`)

// EvaluateFormat 对齐 evaluateExpression。
func (c *DamageContext) EvaluateFormat(fmtStr string, value1, value2, baseValue float64, ev func(*Node, map[string]any, map[string]any) float64) (result float64) {
	count := 0
	expr := formatTokenRe.ReplaceAllStringFunc(fmtStr, func(m string) string {
		count++
		var value float64
		if count%2 == 1 {
			value = value1
		} else {
			value = value2
		}
		if m == "{%}" {
			return "(" + formatFloat(value) + " * " + formatFloat(baseValue) + ")"
		}
		return formatFloat(value)
	})
	expr = strings.ReplaceAll(expr, "×", "*")
	evalSafe := func(e string) (result float64, ok bool) {
		node, err := ParseAST(e, nil)
		if err != nil {
			return 0, false
		}
		defer func() {
			if r := recover(); r != nil {
				// 仅 AstError 走兜底；其余 panic 原样抛出（对齐 Python 只捕 AstError）
				if _, isAst := r.(*AstError); !isAst {
					panic(r)
				}
				result = value1 * baseValue
				ok = false
			}
		}()
		result = ev(node, map[string]any{}, map[string]any{})
		return result, true
	}
	if r, ok := evalSafe(expr); ok {
		if math.IsNaN(r) {
			return value1 * baseValue
		}
		return r
	}
	safe := nonArithRe.ReplaceAllString(expr, "")
	if strings.TrimSpace(safe) == "" {
		return value1 * baseValue
	}
	if r, ok := evalSafe(safe); ok {
		if math.IsNaN(r) {
			return value1 * baseValue
		}
		return r
	}
	return value1 * baseValue
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// EvaluateSkill 对齐 evaluateSkill。
func (c *DamageContext) EvaluateSkill(fieldName, ns string, temp map[string]any, ctxSafe string, ev func(*Node, map[string]any, map[string]any) float64) float64 {
	if wb, ok := WeaponDamageFieldBase[fieldName]; ok {
		if _, has := c.Panels[wb]; !has {
			return 0
		}
		panel := c.TempWeaponAttr(wb, fieldName, temp, ctxSafe)
		current := c.SummonAttrs(wb, temp, fieldName, ctxSafe)
		return (Num(current["攻击"]) + Num(panel["攻击"])) * c.GetDef(wb, fieldName, temp, ctxSafe)
	}
	fieldBase := ns
	if _, ok := WeaponDamageFieldBase[fieldName]; ok {
		fieldBase = WeaponDamageFieldBase[fieldName]
	}
	current := c.SummonAttrs(fieldBase, temp, fieldName, ctxSafe)
	switch fieldName {
	case "[攻击]":
		panel := c.TempWeaponAttr(ns, fieldName, temp, ctxSafe)
		return (Num(current["攻击"]) + Num(panel["攻击"])) * c.GetDef(ns, fieldName, temp, ctxSafe)
	case "[防御]":
		return Num(current["防御"]) * c.GetDef(ns, fieldName, temp, ctxSafe)
	case "[生命]":
		return Num(current["生命"]) * c.GetDef(ns, fieldName, temp, ctxSafe)
	}
	var table []map[string]any
	if ctxSafe != "" {
		table = c.table(ctxSafe)
	} else if ns != "" {
		table = c.Tables[ns]
	} else {
		table = c.Tables[c.BaseName]
	}
	field := c.FindField(table, fieldName)
	if field == nil {
		return 0
	}
	name := S(field, "名称")
	if strings.HasSuffix(name, "伤害") || strings.HasSuffix(name, "治疗") {
		value1 := Num(field["值"])
		value2 := Num(field["值2"])
		kind := S(field, "基础")
		var baseValue float64
		if kind == "" {
			patk := Num(c.TempWeaponAttr(ns, fieldName, temp, ctxSafe)["攻击"])
			baseValue = Num(current["攻击"]) + patk
		} else if kind == "生命" {
			baseValue = Num(current["生命"])
		} else if kind == "防御" {
			baseValue = Num(current["防御"])
		} else {
			baseValue = Num(current["攻击"])
		}
		var baseDamage float64
		if fmtStr, ok := field["格式"].(string); ok {
			baseDamage = c.EvaluateFormat(fmtStr, value1, value2, baseValue, ev)
		} else {
			baseDamage = value1*baseValue + value2
		}
		if strings.HasSuffix(name, "治疗") {
			return baseDamage
		}
		return baseDamage * c.GetDef(ns, fieldName, temp, ctxSafe)
	}
	return Num(field["值"])
}

// IsDamageSkillField 对齐 isDamageSkillField。
func (c *DamageContext) IsDamageSkillField(fieldName, base, ctxSafe string) bool {
	if fieldName == "[攻击]" || fieldName == "[防御]" || fieldName == "[生命]" {
		return true
	}
	if _, ok := WeaponDamageFieldBase[fieldName]; ok {
		return true
	}
	var table []map[string]any
	if ctxSafe != "" {
		table = c.table(ctxSafe)
	} else if base != "" {
		table = c.Tables[base]
	} else {
		table = c.Tables[c.BaseName]
	}
	field := c.FindField(table, fieldName)
	if field == nil {
		return false
	}
	name := S(field, "名称")
	return strings.HasSuffix(name, "伤害") || strings.HasSuffix(name, "伤害倍率")
}

func (c *DamageContext) boost(merged map[string]any) float64 {
	return BoostMultiplier(merged, NumD(c.Combat["hpPercent"], 1))
}

func (c *DamageContext) desperate(merged map[string]any) float64 {
	return DesperateMultiplier(merged, NumD(c.Combat["hpPercent"], 1))
}

func (c *DamageContext) triggerMult(dtype string) float64 {
	resistance := Num(c.Combat["resistance"])
	bonus := Num(c.Combat["triggerBonus"])
	if dtype == "灾厄" {
		if resistance != 0 {
			return 1 + bonus
		}
		return 0
	}
	hpTypeDMG, _ := c.Combat["hpTypeDMG"].(map[string]any)
	cur, _ := c.Combat["currentHPType"].(string)
	if dtype == S(hpTypeDMG, cur) && cur != "" {
		coef, _ := c.Combat["hpTypeCoefficients"].(map[string]any)
		return Num(coef[cur]) + bonus
	}
	return 0
}

// SkillDamage 对齐 calculateSkillDamage。
func (c *DamageContext) SkillDamage(merged map[string]any, base, fieldName, ctxSafe string) map[string]any {
	tableSafe := ctxSafe
	if tableSafe == "" {
		key := base
		if key == "" {
			key = c.BaseName
		}
		for _, e := range c.SkillNames {
			if S(e, "名称") == key {
				tableSafe = S(e, "safeName")
				break
			}
		}
	}
	var field map[string]any
	if fieldName != "" {
		field = c.FindFieldWide(c.table(tableSafe), fieldName)
	}
	isSummon := false
	if field != nil {
		if tags, ok := field["tag"].([]any); ok {
			for _, t := range tags {
				if s, ok := t.(string); ok && s == "召唤物" {
					isSummon = true
				}
			}
		}
	}
	diBase := 1 + Num(merged["增伤"]) + Num(merged["技能伤害"])
	if isSummon {
		diBase += Num(merged["召唤物伤害"])
	}
	elemInc := Num(merged["元素增伤"])
	physInc := Num(merged["物理增伤"])
	other := 1 + Num(merged["独立增伤"])
	if isSummon {
		other *= 1 + Num(merged["召唤物独立增伤"])
	}
	if b, ok := c.Combat["imbalance"].(bool); ok && b {
		other *= Num(merged["失衡易伤"]) + 1.5
	}
	other *= math.Max(0, 1+Num(merged["属性穿透"]))
	hpMore := c.boost(merged) * c.desperate(merged)
	resistance := Num(c.Combat["resistance"])
	factor := func(r float64) float64 { return math.Max(0, 1-r) }
	flipped := math.Max(0, 1-func() float64 {
		if resistance > 0 {
			return -4
		}
		return 0.5
	}())
	var active string
	if resistance > 0 {
		active = "转属克"
	} else if resistance < 0 {
		active = "转属逆"
	}
	rawElem := 0.0
	if active != "" {
		rawElem = math.Max(0, Num(merged[active]))
	}
	rawPhys := 0.0
	for _, k := range PhysicalConverts {
		rawPhys += math.Max(0, Num(merged[k]))
	}
	pool := rawElem + rawPhys
	scale := 1.0
	if pool > 1 {
		scale = 1 / pool
	}
	rest := math.Max(0, 1-math.Min(1, pool))
	type part struct {
		r  float64
		f  float64
		el bool
	}
	var parts []part
	if rest > 0 {
		parts = append(parts, part{rest, factor(resistance), true})
	}
	if active != "" {
		ratio := math.Max(0, Num(merged[active])) * scale
		if ratio > 0 {
			parts = append(parts, part{ratio, flipped, true})
		}
	}
	for _, k := range PhysicalConverts {
		ratio := math.Max(0, Num(merged[k])) * scale
		if ratio > 0 {
			parts = append(parts, part{ratio, 1, false})
		}
	}
	phys, elem := 0.0, 0.0
	for _, p := range parts {
		inc := diBase + elemInc
		if !p.el {
			inc = diBase + physInc
		}
		if p.el {
			elem += p.r * p.f * inc
		} else {
			phys += p.r * p.f * inc
		}
	}
	total := phys + elem
	return map[string]any{
		"expectedDamage": total * other * hpMore,
		"noHpDamage":     total * other,
		"physicalDamage": phys * other * hpMore,
		"elementDamage":  elem * other * hpMore,
	}
}

func (c *DamageContext) convTrigger(key string) float64 {
	m := map[string]string{"转切割": "切割", "转贯穿": "贯穿", "转震荡": "震荡", "转灾厄": "灾厄"}
	if v, ok := m[key]; ok {
		return c.triggerMult(v)
	}
	return c.triggerMult(key)
}

// WeaponDamage 对齐 calculateWeaponDamage。
func (c *DamageContext) WeaponDamage(merged map[string]any, key, fieldDamageType string) map[string]any {
	w, _ := c.Weapons[key].(map[string]any)
	if w == nil {
		w = map[string]any{}
	}
	weaponAttrs, _ := merged["weapon"].(map[string]any)
	if weaponAttrs == nil {
		weaponAttrs = map[string]any{}
	}
	total := Num(merged["攻击"]) + Num(weaponAttrs["攻击"])
	inheritAll := S(w, "atk") == "all" && isTrue(w["inherit"]) && isTrue(w["isSkillWeapon"])
	dtype := fieldDamageType
	if dtype == "" {
		dtype = S(w, "伤害类型")
	}
	convertPhys := dtype == "灾厄" && !inheritAll
	var physShare, elemShare float64
	if convertPhys {
		physShare, elemShare = 1, 0
	} else if inheritAll {
		physShare, elemShare = 0, 1
	} else {
		if total != 0 {
			physShare = Num(weaponAttrs["攻击"]) / total
			elemShare = Num(merged["攻击"]) / total
		}
	}
	tr := Num(weaponAttrs["触发"])
	if tr < 0 {
		tr = 0
	}
	if tr > 1 {
		tr = 1
	}
	triggerRate := tr
	critRate := Num(weaponAttrs["暴击"])
	critDmg := Num(weaponAttrs["暴伤"])
	lowerCd := (critDmg-1)*math.Floor(critRate) + 1
	higherCd := (critDmg-1)*math.Ceil(critRate) + 1
	critExp := 1 + critRate*(critDmg-1)
	resistance := Num(c.Combat["resistance"])
	flipped := math.Max(0, 1-func() float64 {
		if resistance > 0 {
			return -4
		}
		return 0.5
	}())
	pen := math.Max(0, 1+Num(merged["属性穿透"]))
	hpMore := c.boost(merged) * c.desperate(merged)
	diBase := 1 + Num(merged["增伤"]) + Num(weaponAttrs["增伤"]) + Num(merged["武器伤害"])
	elemInc := Num(merged["元素增伤"])
	physInc := Num(merged["物理增伤"])
	other := (1 + Num(merged["独立增伤"])) * (1 + Num(weaponAttrs["独立增伤"]))
	other *= 1 + Num(weaponAttrs["追加伤害"])
	if b, ok := c.Combat["imbalance"].(bool); ok && b {
		other *= Num(merged["失衡易伤"]) + 1.5
	}
	other *= pen
	common := hpMore * other
	var active string
	if resistance > 0 {
		active = "转属克"
	} else if resistance < 0 {
		active = "转属逆"
	}
	rawElem := 0.0
	if active != "" {
		rawElem = math.Max(0, Num(merged[active]))
	}
	rawPhys := 0.0
	for _, k := range PhysicalConverts {
		rawPhys += math.Max(0, Num(merged[k]))
	}
	pool := rawElem + rawPhys
	scale := 1.0
	if pool > 1 {
		scale = 1 / pool
	}
	rest := math.Max(0, 1-math.Min(1, pool))
	elemRatio := math.Min(1, rawElem) * scale
	f := func(r float64) float64 { return math.Max(0, 1-r) }
	var resFactor float64
	if resistance == 0 {
		resFactor = f(0)
	} else if resistance < 0 {
		resFactor = (1-elemRatio)*f(resistance) + elemRatio*f(0.5)
	} else {
		resFactor = (1-elemRatio)*f(resistance) + elemRatio*f(-4)
	}
	type entry struct {
		ratio float64
		kind  string
		key   string
	}
	var entries []entry
	if active != "" {
		entries = append(entries, entry{rawElem, "element", active})
	}
	for _, k := range PhysicalConverts {
		entries = append(entries, entry{math.Max(0, Num(merged[k])), "physical", k})
	}
	elemPart := elemShare * resFactor
	type dpart struct {
		r  float64
		t  float64
		el bool
	}
	var parts []dpart
	if inheritAll {
		if rest > 0 {
			parts = append(parts, dpart{elemPart * rest, 0, true})
		}
	} else {
		if physShare > 0 && rest > 0 {
			parts = append(parts, dpart{physShare * rest, c.triggerMult(dtype), false})
		}
		if rest > 0 {
			parts = append(parts, dpart{elemPart * rest, 0, true})
		}
	}
	for _, e := range entries {
		e.ratio *= scale
		if e.ratio <= 0 {
			continue
		}
		if e.kind == "element" {
			if physShare > 0 {
				parts = append(parts, dpart{physShare * e.ratio * flipped, 0, true})
			}
			parts = append(parts, dpart{elemPart * e.ratio, 0, true})
		} else {
			trig := c.convTrigger(e.key)
			if inheritAll {
				parts = append(parts, dpart{elemPart * e.ratio, trig, false})
			} else {
				if physShare > 0 {
					parts = append(parts, dpart{physShare * e.ratio, trig, false})
				}
				parts = append(parts, dpart{elemPart * e.ratio, trig, false})
			}
		}
	}
	inc := func(el bool) float64 {
		if el {
			return diBase + elemInc
		}
		return diBase + physInc
	}
	allPart, trigPart, expPart := 0.0, 0.0, 0.0
	physExp, elemExp := 0.0, 0.0
	for _, p := range parts {
		allPart += p.r * inc(p.el)
		trigPart += p.r * (1 + p.t) * inc(p.el)
		expPart += p.r * (1 + p.t*triggerRate) * inc(p.el)
		if p.el {
			elemExp += p.r * (1 + p.t*triggerRate) * inc(p.el)
		} else {
			physExp += p.r * (1 + p.t*triggerRate) * inc(p.el)
		}
	}
	expCritBase := critExp * common
	return map[string]any{
		"lowerCritNoTrigger":        allPart * (lowerCd * common),
		"higherCritNoTrigger":       allPart * (higherCd * common),
		"lowerCritTrigger":          trigPart * (lowerCd * common),
		"higherCritTrigger":         trigPart * (higherCd * common),
		"lowerCritExpectedTrigger":  expPart * (lowerCd * common),
		"higherCritExpectedTrigger": expPart * (higherCd * common),
		"expectedCritTrigger":       expPart * expCritBase,
		"expectedCritNoTrigger":     allPart * expCritBase,
		"expectedDamage":            expPart * expCritBase,
		"noHpDamage":                expPart * critExp * other,
		"physicalDamage":            physExp * expCritBase,
		"elementDamage":             elemExp * expCritBase,
	}
}

func isTrue(v V) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// GetDamage 对齐 getDamage。
func (c *DamageContext) GetDamage(base string, fieldName V, temp map[string]any, ctxSafe string) map[string]any {
	weaponBase := ""
	if s, ok := fieldName.(string); ok {
		weaponBase = WeaponDamageFieldBase[s]
	}
	key := weaponBase
	if key == "" {
		key = base
	}
	if key == "" {
		key = c.BaseName
	}
	cacheKey := key + "\x00" + KeyOf(fieldName) + "\x00" + ctxSafe
	if len(temp) == 0 {
		if d, ok := c.dmgCache[cacheKey]; ok {
			return d
		}
	}
	merged := CloneMap(c.SummonAttrs(key, temp, strOrEmpty(fieldName), ctxSafe))
	var table []map[string]any
	if ctxSafe != "" {
		table = c.table(ctxSafe)
	} else {
		table = c.Tables[key]
	}
	var field map[string]any
	if s, ok := fieldName.(string); ok && s != "" {
		field = c.FindFieldWide(table, s)
	}
	fieldDtype := S(field, "伤害类型")
	w, _ := c.Weapons[key].(map[string]any)
	arrow := false
	if w != nil && fieldDtype == "灾厄" && S(w, "类型") == "远程" {
		if b, ok := c.Combat["hasArrowRainMod"].(bool); ok && b {
			arrow = true
		}
	}
	weaponAttr := c.TempWeaponAttr(key, strOrEmpty(fieldName), temp, ctxSafe)
	var damage map[string]any
	if w != nil && weaponAttr != nil {
		var tags []any
		if field != nil {
			tags, _ = field["tag"].([]any)
		}
		var bonusInc, bonusInd float64
		if c.BonusFn != nil {
			bonusInc = c.BonusFn(key, fieldName, "增伤", S(w, "类型"), tags, ctxSafe)
			bonusInd = c.BonusFn(key, fieldName, "独立增伤", S(w, "类型"), tags, ctxSafe)
		} else {
			bonusInc = Num(c.AttackBonus[KeyOf(key, fieldName, "增伤")])
			bonusInd = Num(c.AttackBonus[KeyOf(key, fieldName, "独立增伤")])
		}
		denom := 1.0
		if arrow {
			denom = 0.4
		}
		indep := (1 + Num(weaponAttr["独立增伤"])) / denom
		wa := CloneMap(weaponAttr)
		wa["增伤"] = Num(weaponAttr["增伤"]) + bonusInc
		wa["独立增伤"] = indep*(1+bonusInd) - 1
		merged["weapon"] = wa
		damage = c.WeaponDamage(merged, key, fieldDtype)
	} else {
		var bonusInc, bonusInd float64
		if c.BonusFn != nil {
			var wtype string
			if ww, ok := c.Weapons[key].(map[string]any); ok && ww != nil {
				wtype = S(ww, "类型")
			}
			bonusInc = c.BonusFn(key, fieldName, "增伤", wtype, nil, ctxSafe)
			bonusInd = c.BonusFn(key, fieldName, "独立增伤", wtype, nil, ctxSafe)
		} else {
			bonusInc = Num(c.AttackBonus[KeyOf(key, fieldName, "增伤")])
			bonusInd = Num(c.AttackBonus[KeyOf(key, fieldName, "独立增伤")])
		}
		merged["增伤"] = Num(merged["增伤"]) + bonusInc
		merged["独立增伤"] = (1+Num(merged["独立增伤"]))*(1+bonusInd) - 1
		damage = c.SkillDamage(merged, base, strOrEmpty(fieldName), ctxSafe)
	}
	fullnessMult := 1.0
	if field != nil {
		if tags, ok := field["tag"].([]any); ok && hasTag(tags, "充盈") {
			fullnessMult = 1 + Num(merged["充盈威力"])
		} else {
			if conv := math.Max(0, Num(merged["转充盈"])); conv > 0 {
				fullnessMult = 1 + conv*Num(merged["充盈威力"])
			}
		}
	}
	if fullnessMult != 1 {
		scaled := map[string]any{}
		for k, v := range damage {
			if IsNum(v) {
				scaled[k] = Num(v) * fullnessMult
			} else {
				scaled[k] = v
			}
		}
		damage = scaled
	}
	if len(temp) == 0 {
		c.dmgCache[cacheKey] = damage
	}
	return damage
}

func strOrEmpty(v V) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func hasTag(tags []any, want string) bool {
	for _, t := range tags {
		if s, ok := t.(string); ok && s == want {
			return true
		}
	}
	return false
}

var nonArithRe = regexp.MustCompile(`[^0-9+\-*/.()\s]`)

// DamageMember 对齐 evaluateMember。
func (c *DamageContext) DamageMember(damage map[string]any, member string) float64 {
	if member == "" {
		return Num(damage["expectedDamage"])
	}
	name := strings.Map(func(r rune) rune {
		if r == '非' || r == '低' {
			return '未'
		}
		return r
	}, member)
	table := map[string]string{
		"N": "noHpDamage", "物理": "physicalDamage", "元素": "elementDamage",
		"暴击": "higherCritExpectedTrigger", "未暴击": "lowerCritExpectedTrigger",
		"触发": "expectedCritTrigger", "未触发": "expectedCritNoTrigger",
		"暴击触发": "higherCritTrigger", "触发暴击": "higherCritTrigger",
		"未触发暴击": "higherCritNoTrigger", "暴击未触发": "higherCritNoTrigger",
		"触发未暴击": "lowerCritTrigger", "未暴击触发": "lowerCritTrigger",
		"未暴击未触发": "lowerCritNoTrigger", "未触发未暴击": "lowerCritNoTrigger",
	}
	key, ok := table[name]
	if !ok {
		return 0
	}
	if v, ok := damage[key]; ok && v != nil {
		return Num(v)
	}
	return Num(damage["expectedDamage"])
}
