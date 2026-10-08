// BD 装配：纯 BD JSON（CharSettings + charId）→ 实体装配态。
package dob

import (
	"math"
	"regexp"
)

// TraceLevelRe 溯源文本的技能等级加成。
var TraceLevelRe = regexp.MustCompile(`\[([^\]]+)\]\s*等级\s*\+\s*(\d+)`)

// LegacySlotKeys 同律槽位在 settings 中的键。
var LegacySlotKeys = map[string]string{"角色": "charMods", "近战": "meleeMods", "远程": "rangedMods", "同律": "skillWeaponMods"}

// EmptyWeaponRaw 空武器占位。
var EmptyWeaponRaw = map[string]any{
	"id": 0, "名称": "空武器", "类型": []any{"近战", "长柄"}, "伤害类型": "切割",
	"攻击": 0, "暴击": 0, "暴伤": 0, "触发": 0, "描述": "", "加成": map[string]any{}, "熔炼": "", "技能": []any{},
}

func variantSlots(settings map[string]any, slot string) []any {
	index := int(Num(settings["modVariantIndex"]))
	if index <= 0 {
		return AsList(settings[LegacySlotKeys[slot]])
	}
	variants := AsList(settings["modVariants"])
	if index >= 1 && index <= len(variants) {
		if vm, ok := variants[index-1].(map[string]any); ok && vm != nil {
			if v := AsList(vm[slot]); v != nil {
				return v
			}
		}
		return AsList(settings[LegacySlotKeys[slot]])
	}
	return AsList(settings[LegacySlotKeys[slot]])
}

func variantAura(settings map[string]any) int {
	index := int(Num(settings["modVariantIndex"]))
	if index <= 0 {
		return int(Num(settings["auraMod"]))
	}
	variants := AsList(settings["modVariants"])
	if index >= 1 && index <= len(variants) {
		if vm, ok := variants[index-1].(map[string]any); ok && vm != nil {
			if aura := vm["中枢"]; IsFiniteNum(aura) && Num(aura) > 0 {
				return int(Num(aura))
			}
		}
	}
	return int(Num(settings["auraMod"]))
}

func modBuffLv(effectConfig map[string]any, tables *GameDataTables, modID int) int {
	if effectConfig != nil {
		if v, ok := effectConfig["m:"+itoa(modID)]; ok && IsNum(v) {
			return int(Num(v))
		}
	}
	if effect, ok := tables.ModEffectByID[IDKey(modID)]; ok && effect != nil {
		if mx := effect["mx"]; IsNum(mx) {
			return int(Num(mx))
		}
	}
	return 1
}

func weaponBuffLv(effectConfig map[string]any, tables *GameDataTables, weaponID int, elm string) int {
	if effect, ok := tables.WeaponEffectBy[IDKey(weaponID)]; ok && effect != nil {
		if lim, ok := effect["限定"].(string); ok && lim != elm && elm != "any" {
			return 0
		}
	}
	if effectConfig != nil {
		if v, ok := effectConfig["w:"+itoa(weaponID)]; ok && IsNum(v) {
			return int(Num(v))
		}
	}
	if effect, ok := tables.WeaponEffectBy[IDKey(weaponID)]; ok && effect != nil {
		if mx := effect["mx"]; IsNum(mx) {
			return int(Num(mx))
		}
	}
	return 1
}

func customBuff(buffsCfg []any, level int, coverage float64) map[string]any {
	raw := map[string]any{"名称": "自定义BUFF", "描述": "自行填写"}
	for _, entry := range buffsCfg {
		if pair, ok := entry.([]any); ok && len(pair) >= 2 {
			if k, ok := pair[0].(string); ok {
				raw[k] = pair[1]
			}
		}
	}
	buff := LevelBuff(raw, level, coverage, 1)
	if coverage != 1 {
		buff["_coverage"] = coverage
		BuffApplyLevel(buff, raw)
	}
	return buff
}

func buffFromSettings(tables *GameDataTables, name string, level int, customCfg []any, coverage float64) (map[string]any, bool) {
	if name == "自定义BUFF" {
		return customBuff(customCfg, level, coverage), true
	}
	raw, ok := tables.BuffByName[name]
	if !ok || raw == nil {
		return nil, false
	}
	buff := LevelBuff(raw, level, coverage, 1)
	if coverage != 1 {
		buff["_coverage"] = coverage
		BuffApplyLevel(buff, raw)
	}
	return buff, true
}

func traitBuffs(tables *GameDataTables, slots []any) []map[string]any {
	var out []map[string]any
	for _, slot := range slots {
		pair, ok := slot.([]any)
		if !ok || len(pair) < 2 || !IsNum(pair[0]) || !IsNum(pair[1]) {
			continue
		}
		entry := tables.TraitByLevel(pair[0], int(Num(pair[1])))
		if entry == nil {
			continue
		}
		name := ""
		if n, ok := entry["name"].(string); ok {
			name = "魔灵潜质:" + n
		}
		raw, ok := tables.BuffByName[name]
		if !ok || raw == nil {
			continue
		}
		out = append(out, LevelBuff(raw, entry["level"], 1, 1))
	}
	return out
}

func petBuffs(tables *GameDataTables, settings map[string]any) ([]map[string]any, float64) {
	petID := Num(settings["petId"])
	if petID == 0 {
		return nil, 0
	}
	pet, ok := tables.PetByID[IDKey(petID)]
	if !ok || pet == nil {
		return nil, 0
	}
	traits := AsList(settings["traits"])
	petLevel := settings["petLevel"]
	base := 4.0
	if IsFiniteNum(petLevel) {
		base = Num(petLevel)
	}
	bonus := 0
	for _, slot := range traits {
		pair, ok := slot.([]any)
		if !ok || len(pair) < 2 {
			continue
		}
		var entry map[string]any
		if IsNum(pair[0]) {
			entry = tables.TraitByLevel(pair[0], int(Num(pair[1])))
		}
		if entry != nil && Num(entry["bid"]) == 1030 {
			bonus++
		}
	}
	level := int(base) + bonus
	if level < 0 {
		level = 0
	}
	if level > 4 {
		level = 4
	}
	var out []map[string]any
	if passive, ok := tables.BuffByName[S(pet, "名称")]; ok && passive != nil {
		out = append(out, LevelBuff(passive, level, 1, 1))
	}
	if active, ok := tables.BuffByName[S(pet, "名称")+"(主动)"]; ok && active != nil {
		inst := LevelBuff(active, level, 1, 1)
		inst["_coverage"] = resolveCoverage(tables, settings, pet, level)
		BuffApplyLevel(inst, active)
		out = append(out, inst)
	}
	baseCd := 0.0
	if m, ok := pet["主动"].(map[string]any); ok && m != nil {
		baseCd = Num(m["cd"])
	}
	return out, baseCd
}

func traitCdReduce(tables *GameDataTables, slots []any) float64 {
	reduce := 0.0
	for _, slot := range slots {
		pair, ok := slot.([]any)
		if !ok || len(pair) < 2 || !IsNum(pair[0]) {
			continue
		}
		entry := tables.TraitByLevel(pair[0], int(Num(pair[1])))
		if entry == nil {
			continue
		}
		name := ""
		if n, ok := entry["name"].(string); ok {
			name = "魔灵潜质:" + n
		}
		raw, ok := tables.BuffByName[name]
		if !ok || raw == nil {
			continue
		}
		inst := LevelBuff(raw, entry["level"], 1, 1)
		if v := inst["魔灵CD缩减"]; IsNum(v) {
			reduce += Num(v)
		}
	}
	return pyClamp(reduce, 0, 1)
}

var bracePairRe = regexp.MustCompile(`\{%?\}`)

func resolveCoverage(tables *GameDataTables, settings map[string]any, pet map[string]any, level int) float64 {
	clamp := func(v V) float64 {
		f := Num(v)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			f = 1
		}
		return pyClamp(f, 0, 1)
	}
	if auto, ok := settings["petAutoCoverage"].(bool); ok && !auto {
		return clamp(settings["petCoverage"])
	}
	active, _ := pet["主动"].(map[string]any)
	if active == nil {
		return clamp(settings["petCoverage"])
	}
	desc := S(active, "描述")
	matches := bracePairRe.FindAllStringSubmatchIndex(desc, -1)
	full := bracePairRe.FindAllString(desc, -1)
	durationIdx := -1
	for i, mm := range matches {
		if full[i] == "{}" && desc[mm[1]:] != "" && hasPrefixStr(desc[mm[1]:], "秒") {
			durationIdx = i
			break
		}
	}
	rows := AsList(active["值"])
	duration := 0.0
	if durationIdx >= 0 && durationIdx < len(rows) {
		if row, ok := rows[durationIdx].([]any); ok && len(row) > 0 {
			idx := level
			if idx < 0 {
				idx = 0
			}
			if idx >= len(row) {
				idx = len(row) - 1
			}
			duration = Num(row[idx])
		}
	}
	baseCd := Num(active["cd"])
	cdReduce := traitCdReduce(tables, AsList(settings["traits"]))
	cooldown := baseCd * (1 - pyClamp(cdReduce, 0, 1))
	if duration == 0 || cooldown <= 0 {
		return clamp(settings["petCoverage"])
	}
	return clamp(duration / cooldown)
}

func replaceMap(mods []map[string]any) map[int]map[string]any {
	out := map[int]map[string]any{}
	for _, mod := range mods {
		if mod == nil {
			continue
		}
		if rep, ok := mod["技能替换"].(map[string]any); ok {
			for skillID, skill := range rep {
				id, ok := parseIntStr(skillID)
				if !ok {
					continue
				}
				if sm, ok := skill.(map[string]any); ok {
					out[id] = sm
				}
			}
		}
	}
	return out
}

func parseIntStr(s string) (int, bool) {
	n := 0
	neg := false
	for i, r := range s {
		if i == 0 && r == '-' {
			neg = true
			continue
		}
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	if neg {
		n = -n
	}
	return n, true
}

func normalizeWeaponSkill(skillData, fallback map[string]any) map[string]any {
	strOr := func(a, b V) V {
		if s, ok := a.(string); ok && s != "" {
			return s
		}
		return b
	}
	return map[string]any{
		"id": strOr(skillData["id"], fallback["id"]), "名称": strOr(skillData["名称"], fallback["名称"]),
		"类型": strOr(skillData["类型"], fallback["类型"]), "武器": fallback["武器"],
		"描述": strOr(skillData["描述"], fallback["描述"]), "字段": firstNonNil(skillData["字段"], []any{}),
	}
}

func firstNonNil(a V, d V) V {
	if a != nil {
		return a
	}
	return d
}

// NormalizeSkillLevels 复刻 normalizeCharSkillLevels（接受 []any / []int / 数字）。
func NormalizeSkillLevels(value V) []int {
	clamp := func(level V, fallback int) int {
		if !IsFiniteNum(level) {
			return fallback
		}
		return pyClampInt(JsRound(Num(level)), 1, 12)
	}
	if IsNum(value) {
		lv := clamp(value, 10)
		return []int{lv, lv, lv}
	}
	if list := AsList(value); list != nil {
		var first V
		if len(list) > 0 {
			first = list[0]
		}
		f := clamp(first, 10)
		levels := make([]int, 3)
		for i := 0; i < 3; i++ {
			var item V
			if i < len(list) {
				item = list[i]
			}
			levels[i] = clamp(item, f)
			if !IsFiniteNum(item) {
				if i == 0 {
					levels[i] = f
				} else {
					levels[i] = levels[0]
				}
			}
		}
		return levels
	}
	return []int{10, 10, 10}
}

func pyClampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ResolveSkillLevel 复刻 resolveCharSkillLevel。
func ResolveSkillLevel(levels []int, index int) int {
	normalized := []int{10, 10, 10}
	if len(levels) >= 3 {
		normalized = levels[:3]
	} else if len(levels) > 0 {
		normalized = NormalizeSkillLevels(anySlice(levels))
	}
	if index <= 0 {
		return normalized[0]
	}
	if index == 1 {
		return normalized[1]
	}
	return normalized[2]
}

func anySlice(ints []int) []any {
	out := make([]any, len(ints))
	for i, v := range ints {
		out[i] = v
	}
	return out
}

// ParseTraceLevels 解析已解锁溯源节点的技能等级加成。
func ParseTraceLevels(traceTexts []any, level int) map[string]int {
	count := level
	if count < 0 {
		count = 0
	}
	if count > len(traceTexts) {
		count = len(traceTexts)
	}
	bonus := map[string]int{}
	for i := 0; i < count; i++ {
		text, _ := traceTexts[i].(string)
		for _, m := range TraceLevelRe.FindAllStringSubmatch(text, -1) {
			amount := 0
			for _, r := range m[2] {
				amount = amount*10 + int(r-'0')
			}
			bonus[m[1]] += amount
		}
	}
	return bonus
}

// BuildOptions 装配参数。
type BuildOptions struct {
	SkillLevels []int
	HasLevels   bool
	Traces      int
	HasTraces   bool
}

// BuildState 装配一次构筑的全部实体。
func BuildState(charID int, settings map[string]any, tables *GameDataTables, opts BuildOptions) (map[string]any, error) {
	effectConfig := AsMap(settings["effectConfig"])
	useGlobal, _ := settings["useGlobal"].(bool)
	settings = NormalizeSettings(settings, tables)
	effectConfig = AsMap(settings["effectConfig"])
	charRaw, ok := tables.CharByID[IDKey(charID)]
	if !ok || charRaw == nil {
		return nil, &DobApiError{Message: "角色未在静态表中找到", Code: "missing_char"}
	}
	charLevel := 80
	if IsNum(settings["charLevel"]) {
		charLevel = int(Num(settings["charLevel"]))
	}
	char := LevelChar(charRaw, charLevel)
	var rawLevels V = settings["charSkillLevel"]
	if opts.HasLevels {
		rawLevels = anySlice(opts.SkillLevels)
	}
	trio := NormalizeSkillLevels(rawLevels)
	traceTexts := AsList(charRaw["溯源"])
	traceLevel := 0
	if opts.HasTraces {
		traceLevel = opts.Traces
	} else if IsNum(settings["traces"]) {
		traceLevel = int(Num(settings["traces"]))
	}
	traceBonus := ParseTraceLevels(traceTexts, traceLevel)
	charSkillRaws := AsList(charRaw["技能"])
	var finalLevels []any
	for i, s := range charSkillRaws {
		sm, _ := s.(map[string]any)
		skillName := ""
		if sm != nil {
			skillName = S(sm, "名称")
		}
		bonus := traceBonus[skillName]
		finalLevels = append(finalLevels, pyClampInt(ResolveSkillLevel(trio, i)+bonus, 1, 12))
	}
	wbuffLv := func(weaponID int) V {
		if useGlobal {
			return nil
		}
		return weaponBuffLv(effectConfig, tables, weaponID, S(char, "属性"))
	}
	auraID := variantAura(settings)
	var aura map[string]any
	if auraID != 0 {
		auraRaw, ok := tables.ModByID[IDKey(auraID)]
		if !ok || auraRaw == nil {
			return nil, &DobApiError{Message: "MOD 未在静态表中找到", Code: "missing_mod"}
		}
		aura = LevelMod(auraRaw, nil, nil, tables.ModEffectByID[IDKey(auraID)])
	}
	buildSlot := func(slot string) []any {
		var out []any
		for _, e := range variantSlots(settings, slot) {
			if e == nil {
				out = append(out, nil)
				continue
			}
			pair, ok := e.([]any)
			if !ok || len(pair) < 2 || !IsNum(pair[0]) {
				out = append(out, nil)
				continue
			}
			modID := int(Num(pair[0]))
			raw, ok := tables.ModByID[IDKey(modID)]
			if !ok || raw == nil {
				return nil
			}
			lv := 0
			if IsNum(pair[1]) {
				lv = int(Num(pair[1]))
			}
			var bl V
			if !useGlobal {
				bl = modBuffLv(effectConfig, tables, modID)
			}
			out = append(out, LevelMod(raw, lv, bl, tables.ModEffectByID[IDKey(modID)]))
		}
		return out
	}
	charMods := buildSlot("角色")
	meleeMods := buildSlot("近战")
	rangedMods := buildSlot("远程")
	skillMods := buildSlot("同律")
	if charMods == nil || meleeMods == nil || rangedMods == nil || skillMods == nil {
		return nil, &DobApiError{Message: "MOD 未在静态表中找到", Code: "missing_mod"}
	}
	customCfg := AsList(settings["customBuff"])
	var buffs []map[string]any
	for _, entry := range AsList(settings["buffs"]) {
		pair, _ := entry.([]any)
		if len(pair) < 2 {
			continue
		}
		name, _ := pair[0].(string)
		level := 0
		if IsNum(pair[1]) {
			level = int(Num(pair[1]))
		}
		coverage := 1.0
		if len(pair) > 2 && IsNum(pair[2]) {
			coverage = Num(pair[2])
		}
		if b, ok := buffFromSettings(tables, name, level, customCfg, coverage); ok {
			buffs = append(buffs, b)
		}
	}
	buffs = append(buffs, traitBuffs(tables, AsList(settings["traits"]))...)
	petBuffList, petBaseCd := petBuffs(tables, settings)
	buffs = append(buffs, petBuffList...)
	weaponFrom := func(key, refineKey, levelKey string) (map[string]any, error) {
		weaponID := Num(settings[key])
		if weaponID == 0 {
			raw := CloneMap(EmptyWeaponRaw)
			raw["类型"] = []any{"近战", "长柄"}
			return LevelWeapon(raw, settings[refineKey], settings[levelKey], nil, nil), nil
		}
		raw, ok := tables.WeaponByID[IDKey(weaponID)]
		if !ok || raw == nil {
			return nil, &DobApiError{Message: "武器未在静态表中找到", Code: "missing_weapon"}
		}
		var bl V
		if !useGlobal {
			bl = wbuffLv(int(weaponID))
		}
		return LevelWeapon(raw, settings[refineKey], settings[levelKey], bl, tables.WeaponEffectBy[IDKey(weaponID)]), nil
	}
	meleeWeapon, err := weaponFrom("meleeWeapon", "meleeWeaponRefine", "meleeWeaponLevel")
	if err != nil {
		return nil, err
	}
	rangedWeapon, err := weaponFrom("rangedWeapon", "rangedWeaponRefine", "rangedWeaponLevel")
	if err != nil {
		return nil, err
	}
	extraMastery := S(settings, "extraMastery")
	for _, weapon := range []map[string]any{meleeWeapon, rangedWeapon} {
		effective := true
		if has, ok := weapon["_hasForge"].(bool); ok && has {
			effective = masteredOf(char, S(weapon, "类别"), extraMastery)
		}
		weapon["forgeEffective"] = effective
		if effective {
			weapon["buffProps"] = weapon["_effectiveBuffProps"]
		} else {
			weapon["buffProps"] = map[string]any{}
		}
	}
	var skills []map[string]any
	for i, s := range charSkillRaws {
		lv := trio[2]
		if i < len(finalLevels) {
			lv = int(Num(finalLevels[i]))
		}
		if sm, ok := s.(map[string]any); ok {
			skills = append(skills, LevelSkill(sm, lv, nil))
		}
	}
	var skillWeapon map[string]any
	if syncList := AsList(char["同律武器"]); len(syncList) > 0 {
		uweapon := CloneMap(AsMap(syncList[0]))
		skillIDs := []int{1}
		if sk, ok := uweapon["skill"].([]any); ok {
			skillIDs = nil
			for _, v := range sk {
				if IsNum(v) {
					skillIDs = append(skillIDs, int(Num(v)))
				}
			}
		}
		var sources []map[string]any
		for i, s := range skills {
			for _, id := range skillIDs {
				if i == id {
					sources = append(sources, s)
				}
			}
		}
		var collected []any
		filt := S(uweapon, "filter")
		if filt == "" {
			filt = "伤害"
		}
		filtRe := regexp.MustCompile(filt)
		for _, source := range sources {
			for _, f := range AsList(source["字段"]) {
				if fm, ok := f.(map[string]any); ok {
					name := S(fm, "名称")
					if filtRe.MatchString(name) && hasSuffixStr(name, "伤害") {
						collected = append(collected, fm)
					}
				}
			}
		}
		uweapon["技能"] = []any{map[string]any{"名称": uweapon["名称"], "类型": "同律武器伤害", "字段": collected}}
		syncIndex := 2
		for _, id := range skillIDs {
			if id >= 0 {
				syncIndex = id
				break
			}
		}
		syncLevel := pyClampInt(ResolveSkillLevel(trio, syncIndex)+traceBonus[S(uweapon, "名称")], 1, 12)
		skillWeapon = LevelSkillWeapon(uweapon, syncLevel, Num(char["_等级"]), nil)
		if inherit := S(skillWeapon, "inherit"); inherit != "" {
			var target map[string]any
			if inherit == "melee" {
				target = meleeWeapon
			} else {
				target = rangedWeapon
			}
			if isEmptyWeapon(target) {
			} else {
				skillWeapon["伤害类型"] = target["伤害类型"]
			}
		}
	}
	state := map[string]any{
		"char": char, "skills": skills, "skillWeapon": skillWeapon,
		"charMods": charMods, "meleeMods": meleeMods, "rangedMods": rangedMods, "skillMods": skillMods,
		"auraMod": aura, "buffs_all": buffs,
		"meleeWeapon": meleeWeapon, "rangedWeapon": rangedWeapon,
		"baseName": settings["baseName"], "imbalance": isTrue(settings["imbalance"]),
		"hpPercent":     pyClamp(NumD(settings["hpPercent"], 1), 0, 1),
		"resonanceGain": Num(settings["resonanceGain"]),
		"enemyId":       NumD(settings["enemyId"], 130), "enemyLevel": enemyLevelOf(settings),
		"enemyResistance":      Num(settings["enemyResistance"]),
		"targetFunction":       S(settings, "targetFunction"),
		"extraMastery":         extraMastery,
		"skillLevelsFinal":     finalLevels,
		"traceBonus":           traceBonus,
		"teamWeaponCategories": teamCategories(tables, settings),
		"petBaseCd":            petBaseCd,
		"skillLevel":           trio,
		"settings":             settings,
	}
	if state["targetFunction"] == "" {
		state["targetFunction"] = "伤害"
	}
	var plainBuffs []any
	var dynamicBuffs []any
	for _, b := range buffs {
		hasCode := S(b, "code") != ""
		propCount := 0
		for k := range b {
			if !BuffExclude[k] && !(len(k) > 0 && k[0] == '_') {
				propCount++
			}
		}
		if !hasCode || propCount > 0 {
			plainBuffs = append(plainBuffs, b)
		}
		if hasCode {
			dynamicBuffs = append(dynamicBuffs, b)
		}
	}
	state["buffs"] = plainBuffs
	state["dynamicBuffs"] = dynamicBuffs
	var customVars []any
	for _, e := range AsList(settings["customVariables"]) {
		customVars = append(customVars, e)
	}
	state["customVariables"] = customVars
	state["meleeWeaponSkills"] = replaceSkills(AsList(meleeWeapon["技能"]), meleeMods)
	state["rangedWeaponSkills"] = replaceSkills(AsList(rangedWeapon["技能"]), rangedMods)
	var swSkills []any
	if skillWeapon != nil {
		swSkills = AsList(skillWeapon["技能"])
	}
	state["skillWeaponSkills"] = replaceSkills(swSkills, skillMods)
	var weaponSkills []any
	weaponSkills = append(weaponSkills, AsList(state["meleeWeaponSkills"])...)
	weaponSkills = append(weaponSkills, AsList(state["rangedWeaponSkills"])...)
	weaponSkills = append(weaponSkills, AsList(state["skillWeaponSkills"])...)
	state["weaponSkills"] = weaponSkills
	var allSkills []any
	for _, s := range skills {
		allSkills = append(allSkills, s)
	}
	allSkills = append(allSkills, weaponSkills...)
	state["allSkills"] = allSkills
	var nameList []any
	for _, s := range allSkills {
		if m, ok := s.(map[string]any); ok {
			nameList = append(nameList, map[string]any{"名称": S(m, "名称"), "safeName": S(m, "safeName")})
		}
	}
	state["skill_name_list"] = nameList
	var eSkill, qSkill, pSkill map[string]any
	if len(skills) > 0 {
		eSkill = skills[0]
	}
	if len(skills) > 1 {
		qSkill = skills[1]
	}
	if len(skills) > 2 {
		pSkill = skills[2]
	}
	state["skill_aliases"] = map[string]any{"E": S(eSkill, "safeName"), "Q": S(qSkill, "safeName"), "P": S(pSkill, "safeName")}
	enemyRaw, ok := tables.MonsterByID[IDKey(state["enemyId"])]
	if !ok || enemyRaw == nil {
		enemyRaw, ok = tables.MonsterByID[IDKey(130)]
	}
	if !ok || enemyRaw == nil {
		return nil, &DobApiError{Message: "怪物未在静态表中找到", Code: "missing_monster"}
	}
	state["enemy"] = LevelMonster(enemyRaw, int(Num(state["enemyLevel"])), false)
	return state, nil
}

func enemyLevelOf(settings map[string]any) float64 {
	if v := Num(settings["enemyLevel"]); v != 0 {
		return v
	}
	return 80
}

func masteredOf(char map[string]any, category, extra string) bool {
	for _, p := range AsList(char["精通"]) {
		if s, ok := p.(string); ok && s == category {
			return true
		}
	}
	for _, p := range AsList(char["精通"]) {
		if s, ok := p.(string); ok && s == "全部类型" {
			return true
		}
	}
	return category != "" && category == extra
}

func teamCategories(tables *GameDataTables, settings map[string]any) []any {
	var out []any
	for _, key := range []string{"team1Weapon", "team2Weapon"} {
		weaponID := settings[key]
		if _, ok := weaponID.(bool); ok {
			continue
		}
		if weaponID == nil {
			continue
		}
		if Num(weaponID) == 0 && IsNum(weaponID) {
			out = append(out, "长柄")
			continue
		}
		if IsNum(weaponID) {
			if raw, ok := tables.WeaponByID[IDKey(weaponID)]; ok && raw != nil {
				if t := AsList(raw["类型"]); len(t) > 1 {
					if s, ok := t[1].(string); ok {
						out = append(out, s)
					}
				}
			}
		}
	}
	return out
}

func replaceSkills(skills []any, mods []any) []any {
	var modList []map[string]any
	for _, m := range mods {
		if mm, ok := m.(map[string]any); ok && mm != nil {
			modList = append(modList, mm)
		}
	}
	replace := replaceMap(modList)
	if len(replace) == 0 {
		return skills
	}
	var out []any
	for _, s := range skills {
		sm, _ := s.(map[string]any)
		if sm == nil {
			out = append(out, s)
			continue
		}
		id := 0
		if IsNum(sm["id"]) {
			id = int(Num(sm["id"]))
		}
		target, ok := replace[id]
		if !ok {
			out = append(out, sm)
			continue
		}
		raw := normalizeWeaponSkill(target, sm)
		lv := V(nil)
		if sm["_level"] != nil {
			lv = sm["_level"]
		}
		out = append(out, LevelSkill(raw, lv, strPtr(S(sm, "武器名"))))
	}
	return out
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ModSlotCounts 槽位数量。
var ModSlotCounts = map[string]int{"角色": 8, "近战": 8, "远程": 8, "同律": 4}

// ModSlotTypes 槽位类型。
var ModSlotTypes = []string{"角色", "近战", "远程", "同律"}

// pyRound 复刻 Python round（banker's，half-even）。
func pyRound(f float64) float64 { return math.RoundToEven(f) }

func normalizeSlots(value V, length int) []any {
	slots := AsList(value)
	out := make([]any, 0, length)
	for i := 0; i < length; i++ {
		var slot V
		if i < len(slots) {
			slot = slots[i]
		}
		pair, ok := slot.([]any)
		if !ok || len(pair) < 2 || !IsFiniteNum(pair[0]) || !IsFiniteNum(pair[1]) {
			out = append(out, nil)
			continue
		}
		out = append(out, []any{pyRound(Num(pair[0])), pyRound(Num(pair[1]))})
	}
	return out
}

// NormalizeTraitSlots 复刻 normalizeTraitSlots。
func NormalizeTraitSlots(slots []any, tables *GameDataTables) []any {
	used := map[string]bool{}
	var picked []any
	for _, slot := range slots {
		pair, ok := slot.([]any)
		if !ok || len(pair) < 2 {
			continue
		}
		bid, level := pair[0], pair[1]
		if !IsNum(bid) || !IsNum(level) {
			continue
		}
		bi, lv := int(Num(bid)), int(Num(level))
		if used[itoa(bi)] || tables.TraitByLevel(bi, lv) == nil {
			continue
		}
		used[itoa(bi)] = true
		picked = append(picked, []any{bi, lv})
	}
	for len(picked) < 4 {
		picked = append(picked, nil)
	}
	return picked[:4]
}

func migrateLegacyPetBuffs(settings map[string]any, tables *GameDataTables) {
	buffs := AsList(settings["buffs"])
	if len(buffs) == 0 {
		return
	}
	traits := NormalizeTraitSlots(AsList(settings["traits"]), tables)
	used := map[string]bool{}
	for _, slot := range traits {
		if pair, ok := slot.([]any); ok && len(pair) > 0 && IsNum(pair[0]) {
			used[itoa(int(Num(pair[0])))] = true
		}
	}
	hadPet := Num(settings["petId"]) != 0
	petID := settings["petId"]
	petCoverage := settings["petCoverage"]
	petLevel := settings["petLevel"]
	petLevelTaken := hadPet
	var remaining []any
	migrated := false
	for _, buff := range buffs {
		pair, _ := buff.([]any)
		var name V
		var level V = 0
		var coverage V
		if len(pair) > 0 {
			name = pair[0]
		}
		if len(pair) > 1 {
			level = pair[1]
		}
		if len(pair) > 2 {
			coverage = pair[2]
		}
		origin := resolvePetBuff(name, tables)
		if origin == nil {
			remaining = append(remaining, buff)
			continue
		}
		migrated = true
		if origin.kind == "trait" {
			level0 := 1
			if IsNum(level) {
				level0 = pyClampInt(int(pyRound(Num(level))), 1, 3)
			}
			if !used[itoa(origin.bid)] {
				idx := -1
				for i, t := range traits {
					if t == nil {
						idx = i
						break
					}
				}
				if idx != -1 {
					traits[idx] = []any{origin.bid, level0}
					used[itoa(origin.bid)] = true
				}
			}
			continue
		}
		if petID == nil || Num(petID) == 0 {
			petID = origin.pid
		}
		if !hadPet && origin.active && IsNum(coverage) {
			petCoverage = coverage
		}
		if !petLevelTaken && IsFiniteNum(level) {
			petLevel = pyClampInt(int(pyRound(Num(level))), 0, 4)
			petLevelTaken = true
		}
	}
	if !migrated {
		return
	}
	settings["buffs"] = remaining
	settings["traits"] = traits
	settings["petId"] = petID
	settings["petLevel"] = petLevel
	settings["petCoverage"] = petCoverage
}

type petBuffOrigin struct {
	kind   string
	bid    int
	pid    int
	active bool
}

func resolvePetBuff(name V, tables *GameDataTables) *petBuffOrigin {
	s, ok := name.(string)
	if !ok {
		return nil
	}
	const prefix = "魔灵潜质:"
	if hasPrefixStr(s, prefix) {
		traitName := s[len(prefix):]
		for _, e := range tables.PetEntries {
			if m, ok := e.(map[string]any); ok && S(m, "name") == traitName {
				bid := 0
				if IsNum(m["bid"]) {
					bid = int(Num(m["bid"]))
				}
				return &petBuffOrigin{kind: "trait", bid: bid}
			}
		}
		return nil
	}
	for _, p := range tables.Pets {
		if m, ok := p.(map[string]any); ok {
			if S(m, "名称") == s {
				pid := 0
				if IsNum(m["id"]) {
					pid = int(Num(m["id"]))
				}
				return &petBuffOrigin{kind: "pet", pid: pid}
			}
			if S(m, "名称")+"(主动)" == s {
				pid := 0
				if IsNum(m["id"]) {
					pid = int(Num(m["id"]))
				}
				return &petBuffOrigin{kind: "pet", pid: pid, active: true}
			}
		}
	}
	return nil
}

// NormalizeSettings 复刻 normalizeCharSettings（读路径子集）。
func NormalizeSettings(settings map[string]any, tables *GameDataTables) map[string]any {
	defaults := map[string]any{
		"charLevel": 80, "baseName": "", "hpPercent": 1.0, "resonanceGain": 3,
		"enemyId": 130, "enemyLevel": 80, "enemyResistance": 0, "isRouge": false,
		"targetFunction": "", "customVariables": []any{}, "charSkillLevel": []any{10, 10, 10},
		"extraMastery": "", "meleeWeapon": 10206, "meleeWeaponLevel": 80, "meleeWeaponRefine": 5,
		"rangedWeapon": 20102, "rangedWeaponLevel": 80, "rangedWeaponRefine": 5,
		"auraMod": 31524, "imbalance": false,
		"charMods":        []any{nil, nil, nil, nil, nil, nil, nil, nil},
		"meleeMods":       []any{nil, nil, nil, nil, nil, nil, nil, nil},
		"rangedMods":      []any{nil, nil, nil, nil, nil, nil, nil, nil},
		"skillWeaponMods": []any{nil, nil, nil, nil},
		"modVariantIndex": 0, "modVariants": []any{},
		"buffs": []any{}, "customBuff": []any{}, "petId": 0, "petLevel": 3, "petCoverage": 1.0,
		"petAutoCoverage": true, "traits": []any{nil, nil, nil, nil},
		"team1Weapon": "-", "team2Weapon": "-",
		"useGlobal": false, "effectConfig": map[string]any{},
		"dotSettings": map[string]any{"skill": 0, "melee": 0, "ranged": 0, "skillweapon": 0, "forceOwnAdditionalDamage": false},
	}
	normalized := map[string]any{}
	for k, v := range defaults {
		normalized[k] = v
	}
	if settings != nil {
		for key, value := range settings {
			dflt, ok := normalized[key]
			if !ok || value == nil {
				continue
			}
			if _, ok := dflt.([]any); ok {
				if _, ok := value.([]any); ok {
					normalized[key] = value
				}
				continue
			}
			if _, ok := dflt.(map[string]any); ok {
				if vm, ok := value.(map[string]any); ok {
					merged := CloneMap(dflt.(map[string]any))
					for k, v := range vm {
						merged[k] = v
					}
					normalized[key] = merged
				}
				continue
			}
			if sameType(value, dflt) {
				normalized[key] = value
			}
		}
	}
	var buffs []any
	for _, b := range AsList(normalized["buffs"]) {
		buffs = append(buffs, b)
	}
	normalized["buffs"] = buffs
	var customBuff []any
	for _, p := range AsList(normalized["customBuff"]) {
		if pair, ok := p.([]any); ok && len(pair) >= 2 {
			customBuff = append(customBuff, []any{pair[0], round6(Num(pair[1]))})
		}
	}
	normalized["customBuff"] = customBuff
	normalized["traits"] = NormalizeTraitSlots(AsList(normalized["traits"]), tables)
	migrateLegacyPetBuffs(normalized, tables)
	petID := normalized["petId"]
	if !IsFiniteNum(petID) || tables.PetByID[IDKey(petID)] == nil {
		normalized["petId"] = 0
	} else {
		normalized["petId"] = int(Num(petID))
	}
	if pl := normalized["petLevel"]; IsFiniteNum(pl) {
		normalized["petLevel"] = pyClampInt(int(pyRound(Num(pl))), 0, 4)
	} else {
		normalized["petLevel"] = 4
	}
	if pc := normalized["petCoverage"]; IsFiniteNum(pc) {
		normalized["petCoverage"] = pyClamp(round6(Num(pc)), 0, 1)
	} else {
		normalized["petCoverage"] = 1.0
	}
	if auto, ok := normalized["petAutoCoverage"].(bool); ok {
		normalized["petAutoCoverage"] = auto
	} else {
		normalized["petAutoCoverage"] = normalized["petAutoCoverage"] != false
	}
	for _, key := range []string{"team1Weapon", "team2Weapon"} {
		if value, ok := normalized[key].(string); ok && value != "-" {
			if found, ok := tables.WeaponByName[value]; ok && found != nil {
				normalized[key] = found["id"]
			} else {
				normalized[key] = value
			}
		}
	}
	var rawLevels V
	if settings != nil {
		rawLevels = settings["charSkillLevel"]
	}
	normalized["charSkillLevel"] = NormalizeSkillLevels(rawLevels)
	variants, _ := normalized["modVariants"].([]any)
	var cleaned []any
	for _, raw := range variants {
		if len(cleaned) >= 2 {
			break
		}
		rm, _ := raw.(map[string]any)
		entry := map[string]any{"中枢": 0}
		for _, slot := range ModSlotTypes {
			var v V
			if rm != nil {
				v = rm[slot]
			}
			entry[slot] = normalizeSlots(v, ModSlotCounts[slot])
		}
		if rm != nil {
			if aura := rm["中枢"]; IsFiniteNum(aura) && Num(aura) > 0 {
				entry["中枢"] = int(pyRound(Num(aura)))
			}
		}
		cleaned = append(cleaned, entry)
	}
	if cleaned == nil {
		cleaned = []any{}
	}
	normalized["modVariants"] = cleaned
	if idx := normalized["modVariantIndex"]; IsFiniteNum(idx) {
		normalized["modVariantIndex"] = pyClampInt(int(pyRound(Num(idx))), 0, len(cleaned))
	} else {
		normalized["modVariantIndex"] = 0
	}
	return normalized
}

func round6(v float64) float64 {
	return math.Floor(v*1e6+0.5) / 1e6
}

func sameType(value, dflt V) bool {
	_, vb := value.(bool)
	_, db := dflt.(bool)
	if vb || db {
		_, vok := value.(bool)
		_, dok := dflt.(bool)
		return vok && dok
	}
	if IsNum(dflt) && IsNum(value) {
		return true
	}
	switch dflt.(type) {
	case string:
		_, ok := value.(string)
		return ok
	default:
		return false
	}
}
