// 在线快照示例：bdid → 线上 JSON → 完整快照 → 求值/属性查看。
//
// 数据源只走抽象接口（TableSource.LoadTables，与直接构造 Engine 用的表完全同构）：
//
//	go run ./examples/snapshot Svmqw3LGoY
//	go run ./examples/snapshot rJHVA0BVOM modules
//	go run ./examples/snapshot Svmqw3LGoY datapack "角色::攻击!"
package main

import (
	"fmt"
	"os"

	dob "dob"
)

// EngineTables 是 Engine 用的 8 张标准表。
var EngineTables = []string{"chars", "mods", "buffs", "effects", "weapons", "pets", "pet_entries", "monsters"}

func makeSource(kind, cacheDir string) (dob.TableSource, error) {
	switch kind {
	case "modules":
		return dob.NewModuleStore(dob.NewGameDataClient(dob.NewBackendClient("", 0)), cacheDir, EngineTables, nil)
	case "datapack", "":
		return dob.NewDataPackStore(cacheDir, "", ""), nil
	default:
		return nil, fmt.Errorf("未知通道: %s（可选 datapack / modules）", kind)
	}
}

func main() {
	bdid := "Svmqw3LGoY"
	if len(os.Args) > 1 {
		bdid = os.Args[1]
	}
	rest := os.Args[2:]
	kind := "datapack"
	if len(rest) > 0 && (rest[0] == "datapack" || rest[0] == "modules") {
		kind, rest = rest[0], rest[1:]
	}
	source, err := makeSource(kind, "")
	if err != nil {
		panic(err)
	}
	snapshot, err := dob.SnapshotFromOnline(bdid, dob.SnapshotOptions{Source: source})
	if err != nil {
		panic(err)
	}
	fmt.Printf("== %s（%s）[%s] ==\n", snapshot.Title, snapshot.Bdid, kind)
	fmt.Println("目标函数:", dob.S(snapshot.Settings, "targetFunction"))
	fmt.Println("目标值:", snapshot.Calculate(""))
	fmt.Println("攻击:", snapshot.Attrs()["攻击"], "增伤:", snapshot.Attrs()["增伤"])
	fmt.Println("最终技能等级:", snapshot.SkillLevelsFinal()[:3])
	fmt.Println("近战面板攻击:", dob.AsMap(snapshot.WeaponPanel("melee"))["攻击"])
	fmt.Println("表达式求值 [角色::攻击!]:", snapshot.Evaluate("角色::攻击!"))
	for _, expr := range rest {
		fmt.Printf("表达式求值 [%s]: %v\n", expr, snapshot.Evaluate(expr))
	}
}
