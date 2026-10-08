# dob Go SDK

dna-builder 后端服务 + 数据包 + 纯表达式计算的 Go SDK（与 `sdk/python` 功能一致，单包 `dob`，零外部依赖）。

- **后端**：只覆盖无需登录的公开读接口（评论 / 构筑 / 攻略 / 时间轴 / 榜单 / 脚本 / MOD / DPS / `gameData*`）。
- **数据双加载（同一抽象接口）**：全量 datapack 下载（`versions.json` + `<ver>.zip`，内置极简 msgpack 解码），或按模块按需经 `gameData` GraphQL 分页拉取；两者都实现 `TableSource.LoadTables()` → `GameDataTables`，上层只依赖该接口。磁盘持久化并支持运行时热重载。
- **计算**：纯 BD JSON（CharSettings + charId）进，目标值出。attrs/面板/技能表/伤害乘区全由引擎自算，真实 BD 42 个表达式与 bun 真机逐位一致（与 Python 共享 `sdk/python/tests/golden` 同一套 fixtures）。

```go
import dob "github.com/pa001024/dob-go-sdk"

backend := dob.NewBackendClient("", 0) // 默认 https://api.dna-builder.cn
gamedata := dob.NewGameDataClient(backend)

// 数据源抽象接口：两者都实现 LoadTables() → GameDataTables
pack, _ := dob.NewDataPackStore("", "", "").Ensure("") // 本地有缓存即激活最新
tables, _ := pack.LoadTables()

store, _ := dob.NewModuleStore(gamedata, "", []string{"chars", "mods", "weapons", "buffs", "effects"}, nil)
tables, _ = store.LoadTables() // 只拉声明过的表

// 计算：纯 BD JSON 进，目标值出
state, _ := dob.BuildState(charID, settings, tables, dob.BuildOptions{})
engine := dob.NewEngine(state, tables)
engine.Calculate("") // 目标函数值；传表达式即指定求值

// 在线快照：bdid → 线上 JSON → 完整快照（与 Engine 同一条数据通路）
snapshot, _ := dob.SnapshotFromOnline("Svmqw3LGoY", dob.SnapshotOptions{Source: pack})
snapshot.Evaluate("角色::攻击!")
```

## 与 Python 的语义对齐点

- 数值一律 `float64`；fixture/GraphQL 数字经 `json.Number` 解码，id 索引经 `IDKey` 规范化（int/float64/json.Number 同值同键）。
- `Math.round` half-up（`JsRound`）、`%` 取 C 余（`math.Mod`，与 JS 一致）、`min/max` 复刻 Python 不传播 NaN、BUFF code 内 `Math.min/max` 传播 NaN（JS 语义）。
- `sort.Strings` 字节序 = UTF-8 码点序（BMP 内与 TS/Python 一致）。
- 正则用 RE2：`validateCustomVariableKey` 等三处 `\u4e00` 改写为 `\x{4e00}`。

## 测试

```bash
cd sdk/go && go test ./...
```

46 tests 全过（含 2 个性能基线；真实包基线需 `mock/data-pack/1.6.208.6.zip`）。测试清单与 Python 对应：parity 42 式、attrs/panels、技能等级 trio + 溯源、6 线上 BD（DOT 分流）、14 段 BUFF code 语料、快照、数据源抽象、形状归一化、AST 求值。

示例：`go run ./examples/panels [fixture|datapack|modules]`、`go run ./examples/snapshot <bdid> [datapack|modules] [表达式...]`。
