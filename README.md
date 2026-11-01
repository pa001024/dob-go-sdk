# dob Go SDK

dna-builder 后端服务 + 数据包 + 纯表达式计算的 Go SDK（与 `sdk/python` 功能一致，单包 `dob`）。

- **后端**：只覆盖无需登录的公开读接口（评论 / 构筑 / 攻略 / 时间轴 / 榜单 / 脚本 / MOD / DPS / `gameData*`）。
- **数据双加载（同一抽象接口）**：全量 datapack 下载（`versions.json` + `<ver>.zip`，内置极简 msgpack 解码），或按模块按需经 `gameData` GraphQL 分页拉取；两者都实现 `TableSource.LoadTables()` → `GameDataTables`，上层只依赖该接口。磁盘持久化并支持运行时热重载。
- **计算**：纯 BD JSON（CharSettings + charId）进，目标值出。attrs/面板/技能表/伤害乘区
  全由内嵌 `dob-calc-core` wasm 自算（与 bun 真机逐位一致，
  线上 7 BD 批量约 150 倍于旧纯 Go 实现）。不含编辑语义（无 AutoBuild、无配装寻优）。
  > 0.4.0 起原生 Go 计算引擎已移除（见下文版本记录），`NewEngine` 为纯 wasm facade。

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
- `sort.Strings` 字节序 = UTF-8 码点序（与 TS/Python 一致）。
- 计算语义以 wasm 端（`dob-calc-core`，与 TS 双向 parity）为准，两边 SDK 只做薄封装，结果逐位一致。

## 测试

```bash
cd sdk/go && go test ./...
```

73 tests 全过（含 4 个性能基线，零 skip）。测试清单：parity 42 式、attrs/panels、技能等级 trio + 溯源、7 线上 BD 全量变量、技能字段、快照、数据源抽象、形状归一化、wasm parity。

示例：`go run ./examples/panels [fixture|datapack|modules]`、`go run ./examples/snapshot <bdid> [datapack|modules] [表达式...]`。

### 性能基线（best 实测，供后人对比）

| 基线 | best | 锚点 |
|---|---|---|
| online 七 BD 一批（装配 + 目标值 + 全量属性） | ~1.5ms（best-of-5） | 目标值 + 全量 attrs 对 `online_expected.json` |
| shard 一批（42 表达式 + 全量属性） | 见 `TestWasmShardExpressions` | 逐式对 `shardBuild1_expr.json` 真机值 |
| `activate(1.6.208.8)` / `load_tables()` | ~344ms / ~1ms | 8 表条数 `33/586/209/71/71/152/73/393` |

### 版本记录

- **0.4.0**：移除原生 Go 计算引擎（`damage/evaluate/ast/jscode/curves/entities.go`
  及配套单测），`NewEngine` 改为内嵌 `dob-calc-core` wasm 的纯 facade（`wazero` 转为硬依赖）；
  新增 `dobcc_skill_fields` / `dobcc_panels` Rust 端点支撑技能字段与全槽面板；
  `NewEngineWithBackend` / `SnapshotOptions.EngineBackend` / `BuildOptions.Backend` 取消
  （行为即原 `backend="wasm"`）。
- **上游 wasm 同步**：evaluate 信封新增内联 `panels` 字段：非空时直接采用，
  为空时回退 `dobcc_panels` 端点（行为不变，当前上游该字段恒为空）。

wasm 首建含模块编译成本（约几百毫秒一次），高频调用请复用 `Engine`（会话与装配缓存随实例）。
