// 在线 BD 快照：输入 bdid 拉线上 JSON，输出完整可计算快照。
package dob

import (
	"bytes"
	"encoding/json"
)

// BuildSnapshot 是一次在线 BD 的完整快照。
type BuildSnapshot struct {
	Bdid     string
	Title    string
	CharID   int
	Settings map[string]any
	Engine   *Engine
	attrs    map[string]any
}

// Attrs 全量角色属性（含 weapon 面板；首次计算后缓存）。
func (s *BuildSnapshot) Attrs() map[string]any {
	if s.attrs == nil {
		s.attrs = s.Engine.CalculateWeaponAttributes(nil, false, false)
	}
	return s.attrs
}

// Calculate 目标函数值（缺省取 BD 自带 targetFunction）。
func (s *BuildSnapshot) Calculate(target string) float64 {
	return s.Engine.Calculate(target)
}

// Evaluate 任意表达式求值。
func (s *BuildSnapshot) Evaluate(expr string) float64 {
	return s.Calculate(expr)
}

// CharPanel 角色面板。
func (s *BuildSnapshot) CharPanel() map[string]any {
	return s.Engine.CharPanel()
}

// WeaponPanel 武器面板。
func (s *BuildSnapshot) WeaponPanel(slot string) map[string]any {
	return s.Engine.WeaponPanel(slot)
}

// SkillLevelsFinal 最终技能等级。
func (s *BuildSnapshot) SkillLevelsFinal() [][2]any {
	return s.Engine.SkillLevelsFinal()
}

// SkillFields 技能字段面板数值。
func (s *BuildSnapshot) SkillFields(skillName string) []map[string]any {
	return s.Engine.SkillFields(skillName)
}

// Summary 一页总览。
func (s *BuildSnapshot) Summary() map[string]any {
	attrs := s.Attrs()
	levels := s.SkillLevelsFinal()
	var first3 [][2]any
	if len(levels) > 3 {
		first3 = levels[:3]
	} else {
		first3 = levels
	}
	return map[string]any{
		"bdid": s.Bdid, "title": s.Title, "charId": s.CharID,
		"target": S(s.Settings, "targetFunction"), "targetValue": s.Calculate(""),
		"attack": attrs["攻击"], "skillLevels": first3,
	}
}

// SnapshotOptions 快照参数（数据只走抽象数据源一条通路）。
type SnapshotOptions struct {
	Backend     *BackendClient
	Source      any // GameDataTables | TableSource（DataPackStore / ModuleStore 的 LoadTables）
	SkillLevels []int
	HasLevels   bool
	Traces      int
	HasTraces   bool
}

// SnapshotFromOnline 拉线上 BD JSON 并经 Engine 通路装配完整快照。
//
// 数据只走抽象数据源一条通路：source（DataPackStore / ModuleStore 的
// LoadTables，与直接构造 Engine 用的表完全同构）。
func SnapshotFromOnline(bdid string, opts SnapshotOptions) (*BuildSnapshot, error) {
	client := opts.Backend
	if client == nil {
		client = NewBackendClient("", 0)
	}
	build, err := client.Build(bdid)
	if err != nil {
		return nil, err
	}
	if build == nil {
		return nil, &DobApiError{Message: "构筑不存在: " + bdid, Code: "build_not_found"}
	}
	var settings map[string]any
	switch raw := build["charSettings"].(type) {
	case string:
		var v any
		dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
		dec.UseNumber()
		if err := dec.Decode(&v); err != nil {
			return nil, &DobApiError{Message: "构筑设置解析失败: " + bdid, Code: "bad_settings"}
		}
		m, ok := v.(map[string]any)
		if !ok {
			return nil, &DobApiError{Message: "构筑缺少设置: " + bdid, Code: "bad_settings"}
		}
		settings = m
	case map[string]any:
		settings = raw
	default:
		return nil, &DobApiError{Message: "构筑缺少设置: " + bdid, Code: "bad_settings"}
	}
	resolved, err := ResolveTables(opts.Source)
	if err != nil {
		return nil, err
	}
	charID := int(Num(build["charId"]))
	state, err := BuildState(charID, settings, resolved, BuildOptions{SkillLevels: opts.SkillLevels, HasLevels: opts.HasLevels, Traces: opts.Traces, HasTraces: opts.HasTraces})
	if err != nil {
		return nil, err
	}
	engine := NewEngine(state, resolved)
	id, _ := build["id"].(string)
	if id == "" {
		id = bdid
	}
	title, _ := build["title"].(string)
	stored, _ := state["settings"].(map[string]any)
	if stored == nil {
		stored = settings
	}
	return &BuildSnapshot{Bdid: id, Title: title, CharID: charID, Settings: stored, Engine: engine}, nil
}

// SnapshotFromSettings 本地 BD JSON 装配快照（不联网）。
func SnapshotFromSettings(charID int, settings map[string]any, tables *GameDataTables, opts BuildOptions) (*BuildSnapshot, error) {
	state, err := BuildState(charID, settings, tables, opts)
	if err != nil {
		return nil, err
	}
	stored, _ := state["settings"].(map[string]any)
	if stored == nil {
		stored = settings
	}
	return &BuildSnapshot{CharID: charID, Settings: stored, Engine: NewEngine(state, tables)}, nil
}
