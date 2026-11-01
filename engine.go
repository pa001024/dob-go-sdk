// 构筑聚合引擎（wasm 后端唯一实现）。
//
// 纯 BD JSON（CharSettings + charId）进，目标值出：装配、属性汇总、面板、
// 技能结算全部走内嵌 dob-calc-core wasm（见 wasm.go），与真机逐位一致。
// state 为 build.BuildState 装配的轻量 dict，不含任何实体等级化数据。
package dob

import (
	"math"
)

// Engine 是一次构筑的聚合状态机。
type Engine struct {
	S map[string]any
	T *GameDataTables

	wasm       *WasmBackend
	wasmAttrs  map[string]any
	wasmPanels map[string]any
}

// NewEngine 构造。
func NewEngine(state map[string]any, tables *GameDataTables) *Engine {
	return &Engine{S: state, T: tables}
}

func (e *Engine) wasmBackend() (*WasmBackend, error) {
	if e.wasm == nil {
		b, err := NewWasmBackend(e.T)
		if err != nil {
			return nil, err
		}
		e.wasm = b
	}
	return e.wasm, nil
}

func numOrZero(v any) float64 {
	if IsNum(v) {
		return Num(v)
	}
	return 0
}

// inlinePanels 取 evaluate 信封内联 `panels`（上游新增的面板计算 API）。
//
// 非空 map 时直接采用，免去一次 `dobcc_panels` 往返；缺失/空/非法时返回 nil，
// 调用方回退专用 panels 端点（行为与之前逐位一致）。
func inlinePanels(result map[string]any) map[string]any {
	if result == nil {
		return nil
	}
	panels, ok := result["panels"].(map[string]any)
	if !ok || len(panels) == 0 {
		return nil
	}
	out := make(map[string]any, len(panels))
	for k, v := range panels {
		if m, ok := v.(map[string]any); ok && m != nil {
			cp := make(map[string]any, len(m))
			for kk, vv := range m {
				cp[kk] = vv
			}
			out[k] = cp
		} else {
			out[k] = v
		}
	}
	return out
}

func (e *Engine) wasmFullAttrs() (map[string]any, error) {
	if e.wasmAttrs == nil {
		b, err := e.wasmBackend()
		if err != nil {
			return nil, err
		}
		charID := int(Num(e.S["charId"]))
		settings, _ := e.S["settings"].(map[string]any)
		if settings == nil {
			settings = map[string]any{}
		}
		result, err := b.Evaluate(charID, settings, "")
		if err != nil {
			return nil, err
		}
		attrs := map[string]any{}
		for k, v := range AsMap(result["attributes"]) {
			attrs[k] = numOrZero(v)
		}
		if w, ok := result["weapon"].(map[string]any); ok && w != nil {
			panel := map[string]any{}
			for k, v := range w {
				panel[k] = numOrZero(v)
			}
			attrs["weapon"] = panel
		}
		e.wasmAttrs = attrs
		if inlined := inlinePanels(result); inlined != nil {
			e.wasmPanels = inlined
		}
	}
	return e.wasmAttrs, nil
}

func (e *Engine) wasmAllPanels() (map[string]any, error) {
	if e.wasmPanels == nil {
		b, err := e.wasmBackend()
		if err != nil {
			return nil, err
		}
		charID := int(Num(e.S["charId"]))
		settings, _ := e.S["settings"].(map[string]any)
		if settings == nil {
			settings = map[string]any{}
		}
		panels, err := b.Panels(charID, settings)
		if err != nil {
			return nil, err
		}
		e.wasmPanels = panels
	}
	return e.wasmPanels, nil
}

// Calculate 目标函数值（缺省取 settings 的 targetFunction；非法表达式抛错）。
func (e *Engine) Calculate(target string) float64 {
	b, err := e.wasmBackend()
	if err != nil {
		panic(err)
	}
	charID := int(Num(e.S["charId"]))
	settings, _ := e.S["settings"].(map[string]any)
	if settings == nil {
		settings = map[string]any{}
	}
	result, err := b.Evaluate(charID, settings, target)
	if err != nil {
		panic(err)
	}
	if v, ok := result["targetValue"]; ok && IsNum(v) {
		if f := Num(v); !math.IsNaN(f) && !math.IsInf(f, 0) {
			return f
		}
	}
	return 0
}

// CalculateWeaponAttributes 全量属性（含 weapon 面板键；调用方不得原地改写返回值）。
func (e *Engine) CalculateWeaponAttributes() map[string]any {
	cached, err := e.wasmFullAttrs()
	if err != nil {
		panic(err)
	}
	out := map[string]any{}
	for k, v := range cached {
		out[k] = v
	}
	if w, ok := cached["weapon"].(map[string]any); ok && w != nil {
		panel := map[string]any{}
		for k, v := range w {
			panel[k] = v
		}
		out["weapon"] = panel
	}
	return out
}

// CharPanel 角色面板：全量角色属性（不含 weapon 键）。
func (e *Engine) CharPanel() map[string]any {
	attrs, err := e.wasmFullAttrs()
	if err != nil {
		panic(err)
	}
	out := map[string]any{}
	for k, v := range attrs {
		if k != "weapon" {
			out[k] = v
		}
	}
	return out
}

// WeaponPanel 武器面板：slot 取 melee（近战）/ ranged（远程）/ skill（同律），
// 或武器技能名；未知槽位返回 nil。
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
	panels, err := e.wasmAllPanels()
	if err != nil {
		panic(err)
	}
	if p, ok := panels[norm].(map[string]any); ok && p != nil {
		out := map[string]any{}
		for k, v := range p {
			out[k] = v
		}
		return out
	}
	return nil
}

// SkillLevelsFinal 最终技能等级：[(技能名, 等级)]（trio + 溯源加成并钳制 1-12）。
func (e *Engine) SkillLevelsFinal() [][2]any {
	names, _ := e.S["skillNames"].([]string)
	finals, _ := e.S["skillLevelsFinal"].([]int)
	out := make([][2]any, 0, len(names))
	for i, name := range names {
		var level any
		if i < len(finals) {
			level = finals[i]
		}
		out = append(out, [2]any{name, level})
	}
	return out
}

// SkillFields 技能字段面板数值（结算字段表；未知技能为空）。
func (e *Engine) SkillFields(skillName string) []map[string]any {
	b, err := e.wasmBackend()
	if err != nil {
		panic(err)
	}
	charID := int(Num(e.S["charId"]))
	settings, _ := e.S["settings"].(map[string]any)
	if settings == nil {
		settings = map[string]any{}
	}
	raw, err := b.SkillFields(charID, settings, skillName)
	if err != nil {
		panic(err)
	}
	var out []map[string]any
	for _, f := range raw {
		fm, ok := f.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		for _, kv := range [][2]string{
			{"name", "名称"}, {"safeName", "safeName"}, {"value", "值"},
			{"value2", "值2"}, {"format", "格式"}, {"base", "基础"},
			{"tags", "tag"}, {"damageType", "伤害类型"},
		} {
			if v, ok := fm[kv[0]]; ok && v != nil {
				row[kv[1]] = v
			}
		}
		out = append(out, row)
	}
	if out == nil {
		return []map[string]any{}
	}
	return out
}
