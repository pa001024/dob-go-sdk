// 游戏原始表加载：datapack 模块与测试 fixture 的统一入口。
package dob

// ModuleDefaults 是 datapack 模块键 → 表名。
var ModuleDefaults = map[string]string{
	"chars":       "char.data",
	"mods":        "mod.data",
	"buffs":       "buff.data",
	"effects":     "effect.data",
	"weapons":     "weapon.data",
	"pets":        "pet.data",
	"pet_entries": "pet.data",
	"monsters":    "monster.data",
}

// PetExports 是表名 → pet.data 内的导出名（魔灵表即 default 导出，对齐 gameData pet 数据集）。
var PetExports = map[string]string{"pets": "default", "pet_entries": "petEntrys"}

// DatasetDefaults 是表名 → gameData 数据集 id。
var DatasetDefaults = map[string]string{
	"chars":       "char",
	"mods":        "mod",
	"buffs":       "buff",
	"effects":     "effect",
	"weapons":     "weapon",
	"pets":        "pet",
	"pet_entries": "pet:petEntrys",
	"monsters":    "monster",
}

// GameDataTables 是原始表容器：lists + 派生索引。
type GameDataTables struct {
	Raw        map[string]any
	Chars      []any
	Mods       []any
	Buffs      []any
	Effects    []any
	Weapons    []any
	Pets       []any
	PetEntries []any
	Monsters   []any
	Curves     map[string]any

	CharByID       map[string]map[string]any
	CharByName     map[string]map[string]any
	ModByID        map[string]map[string]any
	BuffByName     map[string]map[string]any
	WeaponByID     map[string]map[string]any
	WeaponByName   map[string]map[string]any
	ModEffectByID  map[string]map[string]any
	WeaponEffectBy map[string]map[string]any
	PetByID        map[string]map[string]any
	MonsterByID    map[string]map[string]any
	TraitsByLevel  map[string]map[int]map[string]any
}

func tableList(raw map[string]any, table string) []any {
	out := []any{}
	for _, v := range AsList(raw[table]) {
		if v == nil {
			continue
		}
		out = append(out, v)
	}
	return out
}

// NewGameDataTables 装配派生索引。
func NewGameDataTables(raw map[string]any) *GameDataTables {
	t := &GameDataTables{Raw: raw}
	t.Chars = tableList(raw, "chars")
	t.Mods = tableList(raw, "mods")
	t.Buffs = tableList(raw, "buffs")
	t.Effects = tableList(raw, "effects")
	t.Weapons = tableList(raw, "weapons")
	t.Pets = tableList(raw, "pets")
	t.PetEntries = tableList(raw, "pet_entries")
	t.Monsters = tableList(raw, "monsters")
	if c, ok := raw["curves"].(map[string]any); ok {
		t.Curves = c
	} else {
		t.Curves = map[string]any{}
	}
	t.CharByID = map[string]map[string]any{}
	t.CharByName = map[string]map[string]any{}
	for _, c := range t.Chars {
		if m, ok := c.(map[string]any); ok {
			t.CharByID[IDKey(m["id"])] = m
			if name, ok := m["名称"].(string); ok {
				t.CharByName[name] = m
			}
		}
	}
	t.ModByID = map[string]map[string]any{}
	for _, m := range t.Mods {
		if mm, ok := m.(map[string]any); ok {
			t.ModByID[IDKey(mm["id"])] = mm
		}
	}
	t.BuffByName = map[string]map[string]any{}
	for _, b := range t.Buffs {
		if mm, ok := b.(map[string]any); ok {
			if name, ok := mm["名称"].(string); ok {
				t.BuffByName[name] = mm
			}
		}
	}
	t.WeaponByID = map[string]map[string]any{}
	t.WeaponByName = map[string]map[string]any{}
	for _, w := range t.Weapons {
		if mm, ok := w.(map[string]any); ok {
			t.WeaponByID[IDKey(mm["id"])] = mm
			if name, ok := mm["名称"].(string); ok {
				t.WeaponByName[name] = mm
			}
		}
	}
	t.ModEffectByID = map[string]map[string]any{}
	t.WeaponEffectBy = map[string]map[string]any{}
	for _, e := range t.Effects {
		mm, ok := e.(map[string]any)
		if !ok || mm["id"] == nil {
			continue
		}
		if _, ok := t.ModByID[IDKey(mm["id"])]; ok {
			t.ModEffectByID[IDKey(mm["id"])] = mm
		}
		if _, ok := t.WeaponByID[IDKey(mm["id"])]; ok {
			t.WeaponEffectBy[IDKey(mm["id"])] = mm
		}
	}
	t.PetByID = map[string]map[string]any{}
	for _, p := range t.Pets {
		if mm, ok := p.(map[string]any); ok {
			t.PetByID[IDKey(mm["id"])] = mm
		}
	}
	t.MonsterByID = map[string]map[string]any{}
	for _, m := range t.Monsters {
		if mm, ok := m.(map[string]any); ok {
			t.MonsterByID[IDKey(mm["id"])] = mm
		}
	}
	t.TraitsByLevel = map[string]map[int]map[string]any{}
	for _, e := range t.PetEntries {
		mm, ok := e.(map[string]any)
		if !ok {
			continue
		}
		bid := IDKey(mm["bid"])
		level := 1
		if lv, ok := mm["level"]; ok && IsNum(lv) {
			level = int(Num(lv))
		} else if r, ok := mm["r"]; ok && IsNum(r) {
			level = int(Num(r)) - 2
		}
		inner, ok := t.TraitsByLevel[bid]
		if !ok {
			inner = map[int]map[string]any{}
			t.TraitsByLevel[bid] = inner
		}
		inner[level] = mm
	}
	return t
}

// TablesFromDict 测试/离线入口：直接接受原始表（不接受 nil 条目）。
func TablesFromDict(raw map[string]any) (*GameDataTables, error) {
	for _, table := range []string{"chars", "mods", "buffs", "effects", "weapons", "pets", "pet_entries", "monsters"} {
		for _, v := range AsList(raw[table]) {
			if v == nil {
				return nil, &DobApiError{Message: "原始表 " + table + " 含 null 条目，请先清理", Code: "null_entry"}
			}
		}
	}
	return NewGameDataTables(raw), nil
}

// TraitByLevel 潜质目录查询。
func (t *GameDataTables) TraitByLevel(bid V, level int) map[string]any {
	if inner, ok := t.TraitsByLevel[IDKey(bid)]; ok {
		return inner[level]
	}
	return nil
}

// TableSource 是原始表数据源抽象接口：两种加载通道的统一入口。
type TableSource interface {
	LoadTables() (*GameDataTables, error)
}

// ResolveTables 把 GameDataTables | TableSource 归一为 GameDataTables。
func ResolveTables(source any) (*GameDataTables, error) {
	if t, ok := source.(*GameDataTables); ok && t != nil {
		return t, nil
	}
	if loader, ok := source.(TableSource); ok && loader != nil {
		return loader.LoadTables()
	}
	return nil, &DobApiError{Message: "无法解析原始表数据源（需 GameDataTables 或实现 LoadTables()）", Code: "bad_source"}
}
