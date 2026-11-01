// BD 装配（wasm 后端）：纯 BD JSON + 原始表 → 轻量 state。
//
// 计算（装配/属性/面板/技能结算）全部走内嵌 wasm（见 wasm.go），
// 本文件只做 wasm 求值之外的轻量数据 plumbing：技能等级 trio 归一化、
// 溯源加成解析、最终技能等级装配。实体等级化已随原生引擎移除。
package dob

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
)

var traceLevelRe = regexp.MustCompile(`\[([^\]]+)\]\s*等级\s*\+\s*(\d+)`)

// BuildOptions 装配选项。
type BuildOptions struct {
	SkillLevels []int
	HasLevels   bool
	Traces      int
	HasTraces   bool
}

func jsRoundHalfUp(x float64) int { return int(math.Floor(x + 0.5)) }

// NormalizeSkillLevels 复刻 normalizeCharSkillLevels：数字→三元组；数组缺项沿首项补齐；各项取整钳制 1-12。
func NormalizeSkillLevels(value V) []int {
	clamp := func(level V, fallback int) int {
		f, ok := toFiniteFloat(level)
		if !ok {
			return fallback
		}
		v := jsRoundHalfUp(f)
		if v < 1 {
			v = 1
		}
		if v > 12 {
			v = 12
		}
		return v
	}
	if value == nil {
		return []int{10, 10, 10}
	}
	switch value.(type) {
	case bool, string:
		// bool 与字符串整体按缺省处理（对齐上游 typeof number 判定）
		return []int{10, 10, 10}
	case float64, float32, int, int64, json.Number:
		level := clamp(value, 10)
		return []int{level, level, level}
	case []any:
		items := value.([]any)
		first := 10
		if len(items) > 0 {
			first = clamp(items[0], 10)
		}
		levels := make([]int, 3)
		for i := 0; i < 3; i++ {
			if i < len(items) {
				if _, ok := toFiniteFloat(items[i]); !ok {
					levels[i] = first
					if i > 0 {
						levels[i] = levels[0]
					}
					continue
				}
				levels[i] = clamp(items[i], first)
			} else {
				levels[i] = first
				if i > 0 {
					levels[i] = levels[0]
				}
			}
		}
		return levels
	}
	return []int{10, 10, 10}
}

func toFiniteFloat(v V) (float64, bool) {
	switch v.(type) {
	case bool, nil:
		return 0, false
	case float64, float32, int, int64, json.Number:
		f := Num(v)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

// ResolveSkillLevel 复刻 resolveCharSkillLevel：0→E，1→Q，2 及之后→被动（非整数/≤0 按 E）。
func ResolveSkillLevel(levels []int, index int) int {
	normalized := NormalizeSkillLevels(anySlice(levels))
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
	bonus := map[string]int{}
	n := len(traceTexts)
	if level < 0 {
		level = 0
	}
	if level > n {
		level = n
	}
	for i := 0; i < level; i++ {
		text, _ := traceTexts[i].(string)
		for _, m := range traceLevelRe.FindAllStringSubmatch(text, -1) {
			var amount int
			for _, c := range m[2] {
				amount = amount*10 + int(c-'0')
			}
			bonus[m[1]] += amount
		}
	}
	return bonus
}

// BuildState 装配一次构筑的轻量 state（wasm 求值 + 最终技能等级用）。
func BuildState(charID int, settings map[string]any, tables *GameDataTables, opts BuildOptions) (map[string]any, error) {
	if settings == nil {
		settings = map[string]any{}
	}
	rawLevels := V(settings["charSkillLevel"])
	if opts.HasLevels {
		rawLevels = anySlice(opts.SkillLevels)
	}
	trio := NormalizeSkillLevels(rawLevels)
	charRaw, ok := tables.CharByID[IDKey(charID)]
	if !ok {
		return nil, fmt.Errorf("角色 %d 未在静态表中找到", charID)
	}
	var skillNames []string
	for _, s := range AsList(charRaw["技能"]) {
		if m, ok := s.(map[string]any); ok {
			skillNames = append(skillNames, S(m, "名称"))
		}
	}
	traceTexts := AsList(charRaw["溯源"])
	traceLevel := 0
	if opts.HasTraces {
		traceLevel = opts.Traces
	} else if v, ok := settings["traces"]; ok {
		traceLevel = int(Num(v))
	}
	traceBonus := ParseTraceLevels(traceTexts, traceLevel)
	finals := make([]int, len(skillNames))
	for i, name := range skillNames {
		lv := ResolveSkillLevel(trio, i) + traceBonus[name]
		if lv < 1 {
			lv = 1
		}
		if lv > 12 {
			lv = 12
		}
		finals[i] = lv
	}
	return map[string]any{
		"charId":           charID,
		"settings":         settings,
		"skillLevel":       trio,
		"skillNames":       skillNames,
		"skillLevelsFinal": finals,
		"traceBonus":       traceBonus,
	}, nil
}
