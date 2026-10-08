// 等级化实体：Leveled* 构造器的纯函数复刻（实例 = map[string]any）。
package dob

import (
	"fmt"
	"math"
	"regexp"
	"sort"
)

// ModQualityMaxLevel 品质 → MOD 满级。
var ModQualityMaxLevel = map[string]int{"金": 10, "紫": 5, "蓝": 5, "绿": 3, "白": 3}

// ModExclude 非属性键。
var ModExclude = map[string]bool{
	"id": true, "系列": true, "品质": true, "耐受": true, "类型": true, "名称": true,
	"描述": true, "限定": true, "极性": true, "属性": true, "消耗": true, "技能替换": true,
	"_等级": true, "_originalModData": true, "buff": true, "buffLv": true, "maxLevel": true,
	"生效": true, "效果": true, "code": true, "count": true, "icon": true, "版本": true,
	"buffProps": true, "_effectAppliedKeys": true,
}

// BuffExclude 非属性键。
var BuffExclude = map[string]bool{
	"id": true, "名称": true, "描述": true, "限定": true, "品质": true, "_等级": true,
	"_originalBuffData": true, "a": true, "b": true, "lx": true, "bx": true, "mx": true,
	"dx": true, "pid": true, "pt": true, "code": true, "attr": true, "技能": true,
	"_ratio": true, "_coverage": true,
}

// JsRound 复刻 Math.round（half-up）。
func JsRound(x float64) int { return int(math.Floor(x + 0.5)) }

// JsRoundN 保留 n 位小数的 half-up。
func JsRoundN(x float64, n int) float64 {
	factor := math.Pow(10, float64(n))
	return math.Floor(x*factor+0.5) / factor
}

func buffPropName(prop string) string {
	if len(prop) > 0 && prop[0] == '@' {
		return prop[1:]
	}
	return prop
}

// LevelBuff 复刻 LeveledBuff 构造 + updatePropertiesByLevel。
func LevelBuff(raw map[string]any, level V, coverage, ratio float64) map[string]any {
	inst := map[string]any{
		"名称": raw["名称"], "描述": S(raw, "描述"),
		"_original": raw, "_coverage": coverage, "_ratio": ratio,
	}
	for _, key := range []string{"a", "b", "lx", "mx", "dx", "技能", "code", "attr"} {
		v := raw[key]
		if v == nil {
			continue
		}
		if (key == "a" || key == "b") && Num(v) == 1 {
			continue
		}
		inst[key] = v
	}
	if raw["mx"] != nil {
		inst["mx"] = raw["mx"]
		if dx, ok := raw["dx"]; ok && dx != nil {
			inst["dx"] = dx
		} else {
			inst["dx"] = raw["mx"]
		}
	}
	lv := -1
	if IsNum(level) {
		lv = int(Num(level))
	}
	if lv < 0 {
		if dx := inst["dx"]; IsNum(dx) {
			lv = int(Num(dx))
		} else if mx := inst["mx"]; IsNum(mx) {
			lv = int(Num(mx))
		} else {
			lv = 1
		}
	}
	inst["_等级"] = lv
	BuffApplyLevel(inst, raw)
	return inst
}

// BuffApplyLevel 复刻 _buff_apply_level。
func BuffApplyLevel(inst, raw map[string]any) {
	level := int(Num(inst["_等级"]))
	if lx := raw["lx"]; IsNum(lx) {
		if int(Num(lx)) > level {
			level = int(Num(lx))
		}
	}
	mx := 1
	if IsNum(raw["mx"]) {
		mx = int(Num(raw["mx"]))
	}
	if level > mx {
		level = mx
	}
	inst["_等级"] = level
	a := NumOrZero(inst["a"])
	if a == 0 {
		a = 1
	}
	b := NumOrZero(inst["b"])
	if b == 0 {
		b = 1
	}
	lx := 1
	if IsNum(raw["lx"]) {
		lx = int(Num(raw["lx"]))
	}
	for prop, maxValue := range raw {
		if BuffExclude[prop] {
			continue
		}
		if maxValue == nil {
			continue
		}
		name := buffPropName(prop)
		base := lx
		if IsNum(raw["lx"]) {
			base = int(Num(raw["lx"]))
		}
		if list, ok := maxValue.([]any); ok {
			idx := level - base
			if idx < 0 {
				idx = 0
			}
			if idx >= len(list) {
				idx = len(list) - 1
			}
			var v float64
			if idx >= 0 && idx < len(list) {
				v = Num(list[idx])
			}
			inst[name] = v * Num(inst["_ratio"]) * Num(inst["_coverage"])
		} else if IsNum(maxValue) {
			value := (Num(maxValue) / a) * (1 + float64(level-lx)/b) * Num(inst["_ratio"]) * Num(inst["_coverage"])
			if name == "神智回复" {
				value = float64(JsRound(value))
			}
			inst[name] = value
		}
	}
}

// BuffProperties 属性键列表（恒返回非 nil，与 Python [] 口径一致）。
func BuffProperties(inst map[string]any) []string {
	out := []string{}
	for k := range inst {
		if BuffExclude[k] || (len(k) > 0 && k[0] == '_') {
			continue
		}
		out = append(out, k)
	}
	return out
}

// BuffIsLayerProp 是否 @ 层属性。
func BuffIsLayerProp(raw map[string]any, prop string) bool {
	_, ok := raw["@"+prop]
	return ok
}

// LevelMod 复刻 LeveledMod 构造 + updateProperties。
func LevelMod(raw map[string]any, level, buffLv V, effectRaw map[string]any) map[string]any {
	maxLevel := ModQualityMaxLevel[S(raw, "品质")]
	if _, ok := ModQualityMaxLevel[S(raw, "品质")]; !ok {
		maxLevel = 1
	}
	inst := map[string]any{
		"id": raw["id"], "名称": raw["名称"], "系列": raw["系列"], "品质": raw["品质"],
		"耐受": raw["耐受"], "类型": raw["类型"], "_original": raw,
		"maxLevel": maxLevel, "buffProps": map[string]any{}, "_effectKeys": []any{},
	}
	for _, key := range []string{"极性", "属性", "限定", "效果", "消耗", "技能替换"} {
		if raw[key] != nil {
			inst[key] = raw[key]
		}
	}
	if effectRaw != nil && (S(effectRaw, "品质") == "" || S(effectRaw, "品质") == S(inst, "品质")) {
		inst["buff"] = LevelBuff(effectRaw, buffLv, 1, 1)
	}
	lv := maxLevel
	if IsNum(level) {
		lv = int(Num(level))
		if lv < 0 {
			lv = 0
		}
		if lv > maxLevel {
			lv = maxLevel
		}
	}
	inst["_等级"] = lv
	modApplyLevel(inst, raw)
	return inst
}

func modApplyLevel(inst, raw map[string]any) {
	maxLevel := int(Num(inst["maxLevel"]))
	level := int(Num(inst["_等级"]))
	var modID float64
	var hasID bool
	if IsNum(inst["id"]) {
		modID, hasID = Num(inst["id"]), true
	}
	if hasID && modID > 100000 {
		inst["耐受"] = Num(raw["耐受"]) + float64(maxLevel-level)
	} else {
		inst["耐受"] = Num(raw["耐受"]) - float64(maxLevel) + float64(level)
	}
	if rep, ok := raw["技能替换"]; ok && rep != nil {
		if hasID && modID > 200000 {
			ratio := 1 + (float64(level*10+100)-100)/200
			inst["技能替换"] = scaleReplace(AsMap(rep), ratio)
		} else {
			inst["技能替换"] = rep
		}
	}
	if eff, ok := raw["生效"].(map[string]any); ok && eff != nil {
		scaled := map[string]any{"条件": eff["条件"]}
		for key, mv := range eff {
			if key == "条件" {
				continue
			}
			delete(inst, key)
			if list, ok := mv.([]any); ok && len(list) == 2 {
				scaled[key] = []any{(Num(list[0]) / float64(maxLevel+1)) * float64(level+1), (Num(list[1]) / float64(maxLevel+1)) * float64(level+1)}
			} else {
				value := (Num(mv) / float64(maxLevel+1)) * float64(level+1)
				if key == "神智回复" || key == "最大耐受" {
					value = math.Ceil(value)
				}
				scaled[key] = value
			}
		}
		inst["生效"] = scaled
	}
	for prop, maxValue := range raw {
		if ModExclude[prop] {
			continue
		}
		if isFalsy(maxValue) {
			continue
		}
		lv := level
		if ser, ok := inst["系列"].(string); ok && (ser == "换生灵" || ser == "海妖") && prop == "减伤" {
			lv = maxLevel
		}
		if hasID && ((modID > 100000 && modID < 150000) || (modID > 200000 && modID < 250000)) {
			lv = maxLevel
		}
		if list, ok := maxValue.([]any); ok {
			idx := lv + 1
			if idx > len(list) {
				idx = len(list)
			}
			idx--
			if idx < 0 {
				idx = 0
			}
			inst[prop] = list[idx]
		} else {
			value := (Num(maxValue) / float64(maxLevel+1)) * float64(lv+1)
			if prop == "神智回复" || prop == "最大耐受" {
				value = math.Ceil(value)
			}
			inst[prop] = value
		}
	}
	buffProps := map[string]any{}
	if buff, ok := inst["buff"].(map[string]any); ok && buff != nil {
		for _, prop := range BuffProperties(buff) {
			maxValue := NumOrZero(buff[prop])
			value := (maxValue / float64(maxLevel+1)) * float64(level+1)
			if prop == "神智回复" || prop == "最大耐受" {
				value = math.Ceil(value)
			}
			if BuffIsLayerProp(AsMap(buff["_original"]), prop) {
				buffProps[prop] = NumOrZero(buffProps[prop]) + value
			} else {
				target := resolveEffectProp(inst, prop)
				if target == "" {
					continue
				}
				cur := 0.0
				if IsNum(inst[target]) {
					cur = Num(inst[target])
				}
				inst[target] = cur + value
				inst["_effectKeys"] = append(AsList(inst["_effectKeys"]), target)
			}
		}
	}
	inst["buffProps"] = buffProps
}

func scaleReplace(replace map[string]any, ratio float64) map[string]any {
	if ratio == 1 || replace == nil {
		return replace
	}
	out := map[string]any{}
	for skillID, skill := range replace {
		entry := CloneMap(AsMap(skill))
		var fields []any
		for _, f := range AsList(entry["字段"]) {
			item := CloneMap(AsMap(f))
			for _, key := range []string{"值", "值2"} {
				if IsNum(item[key]) {
					item[key] = Num(item[key]) * ratio
				}
			}
			fields = append(fields, item)
		}
		entry["字段"] = fields
		out[skillID] = entry
	}
	return out
}

func resolveEffectProp(inst map[string]any, prop string) string {
	modType := S(inst, "类型")
	if modType != "" && len(prop) >= len(modType) && prop[:len(modType)] == modType {
		return prop[len(modType):]
	}
	if prop == "近战" || prop == "远程" || (len(prop) >= 6 && prop[:6] == "同律") {
		return ""
	}
	return prop
}

// ModProperties 属性键列表（恒返回非 nil，与 Python [] 口径一致）。
func ModProperties(inst map[string]any) []string {
	out := []string{}
	for k := range inst {
		if ModExclude[k] || (len(k) > 0 && k[0] == '_') {
			continue
		}
		out = append(out, k)
	}
	return out
}

// ModAddAttr 数值属性子集。
func ModAddAttr(inst map[string]any) map[string]any {
	out := map[string]any{}
	for _, prop := range ModProperties(inst) {
		if IsNum(inst[prop]) {
			out[prop] = inst[prop]
		}
	}
	return out
}

func sortStrings(s []string) { sort.Strings(s) }

var jpLifeDefRe = regexp.MustCompile("生命|防御")

// LevelSkillFields 复刻 LeveledSkill.updateProperties。
func LevelSkillFields(rawFields any, level int) []map[string]any {
	var items []any
	if m, ok := rawFields.(map[string]any); ok {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sortStrings(keys)
		for _, k := range keys {
			items = append(items, m[k])
		}
	} else {
		items = AsList(rawFields)
	}
	var out []map[string]any
	for _, fo := range items {
		fm, ok := fo.(map[string]any)
		if !ok {
			continue
		}
		key := S(fm, "名称")
		obj := CloneMap(fm)
		obj["safeName"] = safeNameOf(key)
		value := fm["值"]
		if list, ok := value.([]any); ok {
			idx := level - 1
			if idx < 0 {
				idx = 0
			}
			if idx >= len(list) {
				idx = len(list) - 1
			}
			obj["值"] = list[idx]
		} else {
			obj["值"] = value
		}
		if fm["值2"] != nil {
			if list, ok := fm["值2"].([]any); ok {
				idx := level - 1
				if idx < 0 {
					idx = 0
				}
				if idx >= len(list) {
					idx = len(list) - 1
				}
				obj["值2"] = list[idx]
			} else {
				obj["值2"] = fm["值2"]
			}
		}
		if S(obj, "格式") != "" {
			if m := jpLifeDefRe.FindString(S(obj, "格式")); m != "" {
				obj["基础"] = m
			}
		}
		out = append(out, obj)
	}
	return out
}

func safeNameOf(key string) string {
	out := make([]rune, 0, len(key))
	for _, r := range key {
		if r == '/' {
			out = append(out, '_')
		} else {
			out = append(out, r)
		}
	}
	return string(out)
}

// LevelSkill 复刻 LeveledSkill。
func LevelSkill(raw map[string]any, level V, weaponName *string) map[string]any {
	lv := 10
	if IsNum(level) {
		lv = int(Num(level))
	}
	if lv < 1 {
		lv = 1
	}
	if lv > 12 {
		lv = 12
	}
	id := raw["id"]
	if id == nil {
		id = 0
	}
	name := S(raw, "名称")
	if name == "" {
		name = "SKILL" + IDKey(id)
	}
	inst := map[string]any{
		"id": id, "名称": name, "safeName": safeNameOf(name),
		"类型": raw["类型"], "描述": raw["描述"],
		"_level": lv, "_original": raw,
	}
	for _, key := range []string{"武器", "术语解释", "实体", "召唤物"} {
		if raw[key] != nil {
			inst[key] = raw[key]
		}
	}
	if weaponName != nil {
		inst["武器名"] = *weaponName
	}
	var subs []any
	for i, s := range AsList(raw["子技能"]) {
		m := AsMap(s)
		if m == nil {
			continue
		}
		nm := S(m, "名称")
		hasID := m["id"] != nil && !(IsNum(m["id"]) && Num(m["id"]) == 0)
		if nm == "" && !hasID {
			continue
		}
		if nm == "" {
			if hasID {
				nm = IDKey(m["id"])
			} else {
				nm = "子技能" + itoa(i+1)
			}
		}
		subs = append(subs, nm)
	}
	inst["子技能"] = subs
	inst["字段"] = LevelSkillFields(raw["字段"], lv)
	return inst
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf []byte
	for i > 0 {
		buf = append([]byte{byte('0' + i%10)}, buf...)
		i /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

// LevelChar 复刻 LeveledChar。
func LevelChar(raw map[string]any, level V) map[string]any {
	lv := 80
	if IsNum(level) {
		lv = int(Num(level))
	}
	if lv < 1 {
		lv = 1
	}
	if lv > 80 {
		lv = 80
	}
	inst := map[string]any{
		"id": raw["id"], "名称": raw["名称"], "属性": raw["属性"],
		"精通":   append([]any{}, AsList(raw["精通"])...),
		"基础攻击": raw["基础攻击"], "基础生命": raw["基础生命"],
		"基础护盾": raw["基础护盾"], "基础防御": raw["基础防御"],
		"基础神智": raw["基础神智"], "_original": raw, "_等级": lv,
	}
	for _, key := range []string{"额外精通", "溯源", "别名", "阵营", "加成", "同律武器", "icon"} {
		if raw[key] != nil {
			inst[key] = raw[key]
		}
	}
	mult := CommonLevelUp[lv-1]
	if IsNum(raw["基础攻击"]) {
		inst["基础攻击"] = JsRoundN(Num(raw["基础攻击"])*mult, 2)
	}
	if IsNum(raw["基础生命"]) {
		inst["基础生命"] = float64(JsRound(Num(raw["基础生命"]) * mult))
	} else {
		inst["基础生命"] = raw["基础生命"]
	}
	if IsNum(raw["基础护盾"]) {
		inst["基础护盾"] = float64(JsRound(Num(raw["基础护盾"]) * mult))
	} else {
		inst["基础护盾"] = raw["基础护盾"]
	}
	// 基础防御/基础神智：TS 侧 updatePropertiesByLevel 仅缩放攻击/生命/护盾（神智不受等级影响）
	return inst
}

// LevelWeapon 复刻 LeveledWeapon。
func LevelWeapon(raw map[string]any, refine, level, effectLv V, effectRaw map[string]any) map[string]any {
	hasForge := raw["熔炉"] != nil && jsTruthyWeapon(raw["熔炉"])
	inst := map[string]any{
		"id": raw["id"], "名称": raw["名称"], "描述": S(raw, "描述"),
		"_original": raw, "buffProps": map[string]any{}, "_effectiveBuffProps": map[string]any{},
		"forgeEffective": true, "倍率": 1, "弹片数": nil,
	}
	wtype := AsList(raw["类型"])
	if len(wtype) > 0 {
		if s, ok := wtype[0].(string); ok {
			inst["类型"] = s
		} else {
			inst["类型"] = ""
		}
	} else {
		inst["类型"] = ""
	}
	if len(wtype) > 1 {
		if s, ok := wtype[1].(string); ok {
			inst["类别"] = s
		} else {
			inst["类别"] = ""
		}
	} else {
		inst["类别"] = ""
	}
	inst["伤害类型"] = raw["伤害类型"]
	inst["基础攻击"] = raw["攻击"]
	inst["基础暴击"] = raw["暴击"]
	inst["基础暴伤"] = raw["暴伤"]
	inst["基础触发"] = raw["触发"]
	if skills := AsList(raw["技能"]); skills != nil {
		var out []any
		hasRay := false
		for _, v := range skills {
			m := AsMap(v)
			if m == nil {
				continue
			}
			skill := map[string]any{"id": m["id"], "名称": m["名称"], "武器": inst["类型"], "类型": m["类型"], "描述": m["描述"]}
			if m["字段"] != nil {
				skill["字段"] = m["字段"]
			}
			if m["实体"] != nil {
				skill["实体"] = m["实体"]
			}
			for _, f := range AsList(m["字段"]) {
				if fm, ok := f.(map[string]any); ok && S(fm, "名称") == "射线伤害" {
					hasRay = true
				}
			}
			wn := S(raw, "名称")
			out = append(out, LevelSkill(skill, nil, &wn))
		}
		inst["技能"] = out
		if hasRay {
			inst["弹道类型"] = "非弹道"
		} else {
			inst["弹道类型"] = "弹道"
		}
	}
	if effectRaw != nil {
		inst["buff"] = LevelBuff(effectRaw, effectLv, 1, 1)
	}
	if interval := Num(raw["射击间隔"]); interval != 0 {
		inst["射速"] = JsRoundN(1/interval, 4)
	} else {
		inst["射速"] = nil
	}
	if z, ok := raw["装填"]; ok && IsNum(z) {
		inst["基础装填"] = z
	} else {
		inst["基础装填"] = 0
	}
	inst["基础弹匣"] = raw["弹匣"]
	inst["基础弹药"] = raw["最大弹药"]
	rf := 5
	if IsNum(refine) {
		rf = int(Num(refine))
	}
	if hasForge {
		inst["_精炼"] = 0
	} else {
		if rf < 0 {
			rf = 0
		}
		if rf > 5 {
			rf = 5
		}
		inst["_精炼"] = rf
	}
	lv := 80
	if IsNum(level) {
		lv = int(Num(level))
	}
	if lv < 1 {
		lv = 1
	}
	if lv > 80 {
		lv = 80
	}
	inst["_等级"] = lv
	inst["_hasForge"] = hasForge
	inst["_isEmpty"] = Num(inst["id"]) == 0
	inst["_isSkillWeapon"] = false
	weaponApply(inst, raw, hasForge)
	return inst
}

func jsTruthyWeapon(v V) bool {
	if m, ok := v.(map[string]any); ok {
		return len(m) > 0
	}
	if l, ok := v.([]any); ok {
		return len(l) > 0
	}
	return jsTruthy(v)
}

func weaponApply(inst, raw map[string]any, hasForge bool) {
	lv := int(Num(inst["_等级"]))
	if lv < 1 {
		lv = 1
	}
	if lv > 80 {
		lv = 80
	}
	inst["基础攻击"] = JsRoundN(Num(raw["攻击"])*CommonLevelUp[lv-1], 2)
	refineLevel := int(Num(inst["_精炼"]))
	if hasForge {
		refineLevel = 5
	}
	scale := !hasForge
	for prop, original := range AsMap(raw["加成"]) {
		if original == nil {
			continue
		}
		if scale {
			inst[prop] = (Num(original) / 5) * float64(refineLevel+5)
		} else {
			inst[prop] = original
		}
	}
	inst["效果"] = raw["熔炼"]
	if buff, ok := inst["buff"].(map[string]any); ok && buff != nil {
		ratio := float64(refineLevel+5) / 10
		buff["_ratio"] = ratio
		BuffApplyLevel(buff, AsMap(buff["_original"]))
		props := map[string]any{}
		for _, p := range BuffProperties(buff) {
			if IsNum(buff[p]) {
				props[p] = buff[p]
			}
		}
		inst["_effectiveBuffProps"] = props
		if fe, ok := inst["forgeEffective"].(bool); !ok || fe {
			inst["buffProps"] = props
		} else {
			inst["buffProps"] = map[string]any{}
		}
	}
}

// LevelSkillWeapon 复刻 LeveledSkillWeapon。
func LevelSkillWeapon(raw map[string]any, skillLevel, level V, charSkills []any) map[string]any {
	wtype := AsList(raw["类型"])
	t0, t1, t2 := "", "", ""
	if len(wtype) > 0 {
		t0, _ = wtype[0].(string)
	}
	if len(wtype) > 1 {
		t1, _ = wtype[1].(string)
	}
	if len(wtype) > 2 {
		t2, _ = wtype[2].(string)
	}
	sl := 10
	if IsNum(skillLevel) {
		sl = int(Num(skillLevel))
	}
	if sl < 1 {
		sl = 1
	}
	if sl > 12 {
		sl = 12
	}
	lv := 80
	if IsNum(level) {
		lv = int(Num(level))
	}
	if lv < 1 {
		lv = 1
	}
	if lv > 80 {
		lv = 80
	}
	inst := map[string]any{
		"id": raw["id"], "名称": raw["名称"], "类型": t0 + t1, "类别": t2,
		"伤害类型": raw["伤害类型"], "基础攻击": Num(raw["攻击"]), "基础暴击": Num(raw["暴击"]),
		"基础暴伤": Num(raw["暴伤"]), "基础触发": Num(raw["触发"]),
		"倍率": 1, "弹片数": nil, "射速": 1, "基础装填": 0,
		"_original": raw, "_技能等级": sl, "_等级": lv,
		"_isSkillWeapon": true, "_isEmpty": false,
	}
	if inst["伤害类型"] == nil {
		inst["伤害类型"] = "切割"
	}
	if atk := raw["攻速"]; IsNum(atk) {
		inst["射速"] = atk
	}
	for _, key := range []string{"inherit", "atk", "视为"} {
		if raw[key] != nil {
			inst[key] = raw[key]
		}
	}
	if skills := AsList(raw["技能"]); skills != nil {
		var out []any
		wn := S(raw, "名称")
		for _, s := range skills {
			m := AsMap(s)
			if m == nil {
				continue
			}
			cp := CloneMap(m)
			cp["武器"] = inst["类型"]
			out = append(out, LevelSkill(cp, sl, &wn))
		}
		inst["技能"] = out
	}
	inst["基础攻击"] = Num(raw["攻击"]) * CommonLevelUp[lv-1]
	return inst
}

// LevelMonster 复刻 LeveledMonster。
func LevelMonster(raw map[string]any, level int, isRouge bool) map[string]any {
	if level < 1 {
		level = 1
	}
	if level > 240 {
		level = 240
	}
	mult := MobLevelUp[level-1]
	inst := map[string]any{
		"id": raw["id"], "n": raw["n"], "f": Num(raw["f"]),
		"atk": float64(JsRound(Num(raw["atk"]) * mult.Atk)), "def": Num(raw["def"]),
		"_等级": level, "_original": raw,
	}
	hpm := mult.HP
	if isRouge {
		hpm = mult.Rhp
	}
	inst["hp"] = float64(JsRound(Num(raw["hp"]) * hpm))
	for _, key := range []string{"t", "es", "tn", "icon", "tags"} {
		if raw[key] != nil {
			inst[key] = raw[key]
		}
	}
	if raw["es"] != nil {
		esm := mult.ES
		if isRouge {
			esm = mult.Res
		}
		inst["es"] = float64(JsRound(Num(raw["es"]) * esm))
	}
	inst["currentHP"] = inst["hp"]
	inst["currentShield"] = Num(inst["es"])
	inst["currentTN"] = Num(inst["tn"])
	return inst
}

func summonFields(summon, skill map[string]any, attrs map[string]any) []map[string]any {
	atkspd := Num(attrs["召唤物攻击速度"])
	var durationField map[string]any
	for _, f := range AsList(skill["字段"]) {
		fm, _ := f.(map[string]any)
		if fm == nil {
			continue
		}
		nm, _ := fm["名称"].(string)
		if containsStr(nm, "召唤物") && containsStr(nm, "持续时间") {
			durationField = fm
			break
		}
	}
	duration := 0.0
	if durationField != nil {
		raw := Num(durationField["值"])
		impact := S(durationField, "影响")
		if containsStr(impact, "技能耐久") {
			duration = raw * NumOr(attrs["技能耐久"], 1)
		} else {
			duration = raw
		}
	}
	interval := NumOr(summon["攻击间隔"], 1)
	if 1+atkspd != 0 {
		interval = interval / (1 + atkspd)
	} else {
		interval = 0
	}
	delay := Num(summon["攻击延迟"])
	attackTimes := 0.0
	if interval != 0 {
		attackTimes = math.Floor((duration - delay) / interval)
	}
	scopeRange := NumOr(attrs["技能范围"], 1) * (1 + Num(attrs["召唤物范围"]))
	if scopeRange > 2.8 {
		scopeRange = 2.8
	}
	rows := []map[string]any{
		{"名称": "召唤物名称", "格式": summon["名称"], "值": 0},
		{"名称": "召唤物攻击延迟", "值": delay, "格式": "{}秒"},
		{"名称": "召唤物攻击间隔", "值": interval, "格式": "{}秒"},
		{"名称": "召唤物攻速", "值": atkspd},
		{"名称": "召唤物攻击次数", "值": attackTimes, "格式": "{}"},
		{"名称": "召唤物范围", "值": scopeRange},
	}
	for _, row := range rows {
		row["safeName"] = safeNameOf(S(row, "名称"))
	}
	return rows
}

func containsStr(s, sub string) bool {
	if s == "" || sub == "" {
		return false
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// NumD 取数值（缺省 d）。
func NumD(v V, d float64) float64 {
	if v == nil {
		return d
	}
	return Num(v)
}

// NumOr 取数值（0/nil 一律回 d，对齐 Python `or` 缺省口径）。
func NumOr(v V, d float64) float64 {
	if v == nil {
		return d
	}
	if f := Num(v); f != 0 {
		return f
	}
	return d
}

// isFalsy 复刻 Python `not value`（None/0/""/False/空容器）。
func isFalsy(v V) bool {
	if v == nil {
		return true
	}
	if b, ok := v.(bool); ok {
		return !b
	}
	if IsNum(v) {
		return Num(v) == 0
	}
	if s, ok := v.(string); ok {
		return s == ""
	}
	if l, ok := v.([]any); ok {
		return len(l) == 0
	}
	if m, ok := v.(map[string]any); ok {
		return len(m) == 0
	}
	return false
}

// pyEqual 复刻 Python `==`（数值互认、bool 参与数值比较、字符串仅同类相等）。

// MergeConditional 复刻 LeveledSkill.mergeConditionalProps。
func MergeConditional(base map[string]any, props map[string]any, isMult func(string) bool) map[string]any {
	merged := CloneMap(base)
	for prop, value := range props {
		if isMult(prop) {
			merged[prop] = (1+Num(merged[prop]))*(1+Num(value)) - 1
		} else {
			merged[prop] = Num(merged[prop]) + Num(value)
		}
	}
	return merged
}

// CondRule 条件规则（编译后 pattern + props）。
type CondRule struct {
	Re    *regexp.Regexp
	Props map[string]any
}

// LevelSkillFieldsWithAttr 复刻 LeveledSkill.getFieldsWithAttr。
func LevelSkillFieldsWithAttr(skill map[string]any, attrs map[string]any, rules []CondRule, isMult func(string) bool) []map[string]any {
	power := NumD(attrs["技能威力"], 0)
	if power == 0 {
		power = 1
	}
	dur := NumD(attrs["技能耐久"], 0)
	if dur == 0 {
		dur = 1
	}
	eff := NumD(attrs["技能效益"], 0)
	if eff == 0 {
		eff = 1
	}
	rng := NumD(attrs["技能范围"], 0)
	if rng == 0 {
		rng = 1
	}
	var out []map[string]any
	for _, f := range AsList(skill["字段"]) {
		fm, ok := f.(map[string]any)
		if !ok {
			continue
		}
		fieldAttrs := attrs
		if attrs != nil && len(rules) > 0 {
			for _, rule := range rules {
				if rule.Re.MatchString(S(fm, "名称")) {
					fieldAttrs = MergeConditional(fieldAttrs, rule.Props, isMult)
				}
			}
		}
		if fieldAttrs != nil && Num(fieldAttrs["技能倍率赋值"]) != 0 && containsStr(S(fm, "名称"), "伤害") {
			cp := CloneMap(fm)
			cp["值"] = Num(fieldAttrs["技能倍率赋值"])*(1+Num(fieldAttrs["技能倍率乘数"])) + Num(fieldAttrs["技能倍率加数"])
			out = append(out, cp)
			continue
		}
		if S(fm, "影响") != "" {
			val := Num(fm["值"])
			val2 := Num(fm["值2"])
			if containsStr(S(fm, "名称"), "伤害") && fieldAttrs != nil {
				val = val*(1+Num(fieldAttrs["技能倍率乘数"])) + Num(fieldAttrs["技能倍率加数"])
			}
			props := splitComma(S(fm, "影响"))
			if hasStr(props, "技能范围") {
				val = val * rng
			}
			if hasStr(props, "技能威力") {
				val = val * power
				val2 = val2 * power
			}
			if hasStr(props, "技能耐久") {
				if containsStr(S(fm, "名称"), "每秒神智消耗") {
					val = val / dur
				} else {
					val = val * dur
				}
			}
			if hasStr(props, "技能效益") {
				if hasStr(props, "技能耐久") {
					m := (2 - eff) / dur
					if m < 0.25 {
						m = 0.25
					}
					val = Num(fm["值"]) * m
				} else {
					val = Num(fm["值"]) * (2 - eff)
				}
			}
			if containsStr(S(fm, "名称"), "神智消耗") {
				val = math.Ceil(val)
			}
			cp := CloneMap(fm)
			cp["值"] = val
			cp["值2"] = val2
			out = append(out, cp)
			continue
		}
		out = append(out, fm)
	}
	if summon, ok := skill["召唤物"].(map[string]any); ok && summon != nil && attrs != nil {
		for _, row := range summonFields(summon, skill, attrs) {
			out = append(out, row)
		}
	}
	return out
}

func splitComma(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return out
}

func hasStr(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// CheckModCondition 复刻 LeveledMod.checkCondition。
func CheckModCondition(mod, attrs map[string]any, charMods []any, condValues map[string]any) map[string]any {
	eff, _ := mod["生效"].(map[string]any)
	if eff == nil {
		return nil
	}
	conditions, _ := eff["条件"].([]any)
	if len(conditions) == 0 {
		return nil
	}
	polar := map[string]int{}
	idCounts := map[string]int{}
	for _, m := range charMods {
		mm, _ := m.(map[string]any)
		if mm == nil {
			continue
		}
		if p, ok := mm["极性"].(string); ok && p != "" {
			polar[p]++
		}
		idCounts[IDKey(mm["id"])]++
	}
	maxCount := 0
	for _, c := range idCounts {
		if c > maxCount {
			maxCount = c
		}
	}
	check := func(actual V, op string, value V) bool {
		switch op {
		case "*":
			return true
		case "=":
			return pyEqual(actual, value)
		case ">":
			return Num(actual) > Num(value)
		case ">=":
			return Num(actual) >= Num(value)
		case "<":
			return Num(actual) < Num(value)
		case "<=":
			return Num(actual) <= Num(value)
		}
		return false
	}
	for _, c := range conditions {
		triple, _ := c.([]any)
		if len(triple) < 3 {
			continue
		}
		attr, _ := triple[0].(string)
		op, _ := triple[1].(string)
		value := triple[2]
		var actual V
		if attr == "*id" {
			actual = maxCount
		} else if polarFirst(attr, polar) {
			actual = polar[string([]rune(attr)[:1])]
		} else if condValues != nil {
			if v, ok := condValues[attr]; ok {
				actual = v
			} else {
				actual = Num(attrs[attr])
			}
		} else {
			if IsNum(attrs[attr]) {
				actual = Num(attrs[attr])
			} else {
				actual = 0.0
			}
		}
		if !check(actual, op, value) {
			props := map[string]any{}
			for k, v := range eff {
				if k != "条件" {
					props[k] = v
				}
			}
			return map[string]any{"isEffective": false, "props": props}
		}
	}
	props := map[string]any{}
	for k, v := range eff {
		if k != "条件" {
			props[k] = v
		}
	}
	return map[string]any{"isEffective": true, "props": props}
}

// pyEqual 复刻 Python `==`（数值互认、bool 参与数值比较、字符串仅同类相等）。
func pyEqual(a, b V) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if sa, ok := a.(string); ok {
		sb, ok := b.(string)
		return ok && sa == sb
	}
	if _, ok := b.(string); ok {
		return false
	}
	if _, ok := a.(bool); ok {
		return Num(a) == Num(b)
	}
	if _, ok := b.(bool); ok {
		return Num(a) == Num(b)
	}
	if IsNum(a) && IsNum(b) {
		return Num(a) == Num(b)
	}
	return false
}

func polarFirst(attr string, polar map[string]int) bool {
	r := []rune(attr)
	if len(r) == 0 {
		return false
	}
	_, ok := polar[string(r[:1])]
	return ok
}

// ModApplyCondition 复刻 LeveledMod.applyCondition。
func ModApplyCondition(mod, attrs map[string]any, charMods []any, condValues map[string]any) bool {
	condition := CheckModCondition(mod, attrs, charMods, condValues)
	if condition == nil {
		return false
	}
	changed := false
	props, _ := condition["props"].(map[string]any)
	effective, _ := condition["isEffective"].(bool)
	for key, prop := range props {
		if effective {
			var final V
			if list, ok := prop.([]any); ok && len(list) == 2 {
				cond := ""
				if eff, ok := mod["生效"].(map[string]any); ok {
					if c, ok := eff["条件"].([]any); ok && len(c) > 0 {
						if t, ok := c[0].([]any); ok && len(t) > 0 {
							cond, _ = t[0].(string)
						}
					}
				}
				var base float64
				if condValues != nil {
					if v, ok := condValues[cond]; ok {
						base = Num(v)
					} else {
						base = Num(attrs[cond])
					}
				} else {
					base = Num(attrs[cond])
				}
				p0, p1 := Num(list[0]), Num(list[1])
				if base*p0 < p1 {
					final = base * p0
				} else {
					final = p1
				}
			} else {
				final = prop
			}
			if !valuesEqual(mod[key], final) {
				mod[key] = final
				changed = true
			}
		} else if mod[key] != nil && jsTruthy(mod[key]) {
			delete(mod, key)
			changed = true
		}
	}
	return changed
}

func valuesEqual(a, b V) bool {
	if IsNum(a) && IsNum(b) {
		return Num(a) == Num(b)
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}
