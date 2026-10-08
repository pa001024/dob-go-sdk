// 面板示例：指定等级 + MOD 列表 → 角色/武器面板；技能等级 trio + 溯源 → 最终等级与字段面板。
//
// 运行（sdk/go 目录，离线 fixture 默认；联网可换通道）：
//
//	go run ./examples/panels
//	go run ./examples/panels datapack
//	go run ./examples/panels modules
package main

import (
	"fmt"
	"os"

	dob "dob"
)

var goldenDir = "../python/tests/golden"

// EngineTables 是 Engine 用的 8 张标准表。
var EngineTables = []string{"chars", "mods", "buffs", "effects", "weapons", "pets", "pet_entries", "monsters"}

func loadTables(kind string) (*dob.GameDataTables, error) {
	switch kind {
	case "datapack":
		return dob.NewDataPackStore("", "", "").LoadTables()
	case "modules":
		store, err := dob.NewModuleStore(dob.NewGameDataClient(dob.NewBackendClient("", 0)), "", EngineTables, nil)
		if err != nil {
			return nil, err
		}
		return store.LoadTables()
	case "fixture", "":
		raw, err := os.ReadFile(goldenDir + "/shardBuild1_data.json")
		if err != nil {
			return nil, err
		}
		v, err := dob.DecodeJSON(raw)
		if err != nil {
			return nil, err
		}
		data := v.(map[string]any)
		effects := []any{}
		for _, e := range dob.AsList(data["effects"]) {
			if e != nil {
				effects = append(effects, e)
			}
		}
		return dob.TablesFromDict(map[string]any{
			"chars": data["chars"], "mods": data["mods"], "buffs": data["buffs"],
			"effects": effects, "weapons": data["weapons"], "pets": data["pets"],
			"pet_entries": data["petEntries"], "monsters": data["monsters"],
		})
	default:
		return nil, fmt.Errorf("未知通道: %s（可选 fixture / datapack / modules）", kind)
	}
}

func loadBD() (int, map[string]any, error) {
	raw, err := os.ReadFile(goldenDir + "/shardBuild1_bd.json")
	if err != nil {
		return 0, nil, err
	}
	v, err := dob.DecodeJSON(raw)
	if err != nil {
		return 0, nil, err
	}
	bd := v.(map[string]any)
	return int(dob.Num(bd["charId"])), bd["settings"].(map[string]any), nil
}

func scenarioCharPanel(tables *dob.GameDataTables, charID int, settings map[string]any) {
	state, err := dob.BuildState(charID, settings, tables, dob.BuildOptions{})
	if err != nil {
		panic(err)
	}
	engine := dob.NewEngine(state, tables)
	fmt.Println("== 角色面板 ==")
	fmt.Println("目标值:", engine.Calculate(""))
	fmt.Println("攻击:", engine.CharPanel()["攻击"])
}

func scenarioWeaponPanel(tables *dob.GameDataTables, charID int, settings map[string]any) {
	state, err := dob.BuildState(charID, settings, tables, dob.BuildOptions{})
	if err != nil {
		panic(err)
	}
	engine := dob.NewEngine(state, tables)
	fmt.Println("== 武器面板 ==")
	fmt.Println("近战攻击:", dob.AsMap(engine.WeaponPanel("melee"))["攻击"])
	fmt.Println("远程攻击:", dob.AsMap(engine.WeaponPanel("ranged"))["攻击"])
}

func scenarioSkillLevels(tables *dob.GameDataTables, charID int, settings map[string]any) {
	state, err := dob.BuildState(charID, settings, tables, dob.BuildOptions{
		SkillLevels: []int{10, 10, 10}, HasLevels: true, Traces: 7, HasTraces: true,
	})
	if err != nil {
		panic(err)
	}
	engine := dob.NewEngine(state, tables)
	fmt.Println("== 技能等级 + 字段 ==")
	for _, pair := range engine.SkillLevelsFinal()[:3] {
		fmt.Println(" ", pair[0], pair[1])
	}
	fields := engine.SkillFields("以坚忍之名")
	fmt.Println("字段数:", len(fields))
}

func main() {
	kind := "fixture"
	if len(os.Args) > 1 {
		kind = os.Args[1]
	}
	tables, err := loadTables(kind)
	if err != nil {
		panic(err)
	}
	fmt.Println("-- 原始表通道:", kind, "--")
	charID, settings, err := loadBD()
	if err != nil {
		panic(err)
	}
	scenarioCharPanel(tables, charID, settings)
	scenarioWeaponPanel(tables, charID, settings)
	scenarioSkillLevels(tables, charID, settings)
}
