// wasm 计算后端：把 dob-calc-core 编译产物直接嵌入 Go 进程（wazero 即时实例化，纯 Go 无需 cgo）。
//
// 与纯 Go Engine 的分工：数据加载（DataPackStore / ModuleStore /
// GameDataTables）仍走 Go，只把最热的「装配 + 属性 + 面板 + 目标函数」
// 求值 offload 到 wasm。等价性见 wasm_test.go（7 线上 BD + shard 42 式）。
//
// 线程模型：每个 WasmBackend 拥有独立 wazero 运行时与 Store，不可跨协程共享；
// 多协程请每协程各建一个 Engine(backend="wasm")。
package dob

import (
	"context"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

//go:embed _wasm/dob_calc_core.wasm
var dobCalcWasm []byte

// WasmPackageVersion 随包 wasm 对应的 dob-calc-core 版本。
const WasmPackageVersion = "dob-calc-core 0.2.4"

// WasmEvaluateError wasm 求值返回 ok:false 信封。
type WasmEvaluateError struct{ Message string }

func (e *WasmEvaluateError) Error() string { return "wasm 求值失败: " + e.Message }

// tablesToModulesJSON GameDataTables → Rust ModulesJson 口径的模块 JSON。
func tablesToModulesJSON(tables *GameDataTables) ([]byte, error) {
	raw := map[string]any{}
	if tables != nil && tables.Raw != nil {
		raw = tables.Raw
	}
	strList := func(v any) []any {
		out := []any{}
		for _, e := range AsList(v) {
			out = append(out, e)
		}
		if out == nil {
			return []any{}
		}
		return out
	}
	nonNull := func(v any) []any {
		out := []any{}
		for _, e := range AsList(v) {
			if e != nil {
				out = append(out, e)
			}
		}
		if out == nil {
			return []any{}
		}
		return out
	}
	modules := map[string]any{
		"char.data":    strList(raw["chars"]),
		"mod.data":     strList(raw["mods"]),
		"weapon.data":  strList(raw["weapons"]),
		"buff.data":    strList(raw["buffs"]),
		"effect.data":  nonNull(raw["effects"]),
		"monster.data": nonNull(raw["monsters"]),
		"pet.data":     strList(raw["pets"]),
		"petTraits":    strList(raw["pet_entries"]),
	}
	return json.Marshal(modules)
}

// WasmBackend 一次 wasm 会话：建表一次，多次求值（calc 会话复用装配）。
type WasmBackend struct {
	runtime       wazero.Runtime
	module        api.Module
	mem           api.Memory
	alloc         api.Function
	dealloc       api.Function
	eval          api.Function
	skillFieldsFn api.Function
	panelsFn      api.Function
	session       uint64
	calc          uint64
	closed        bool
}

// IsTrue wasm 信封 ok 字段判定。
func IsTrue(v any) bool {
	b, _ := v.(bool)
	return b
}

// NewWasmBackend 建表会话（modules JSON 注表 + 计算会话）。
func NewWasmBackend(tables *GameDataTables) (*WasmBackend, error) {
	ctx := context.Background()
	// -Oz 产物含 tail-call 与 typed function references：wazero 默认仅启用 CoreFeaturesV2，
	// 需显式开启 experimental 特性（wazero 自带 spectest 覆盖，无需 cgo 换库）。
	runtime := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesTailCall | experimental.CoreFeaturesTypedFunctionReferences))
	mod, err := runtime.InstantiateWithConfig(ctx, dobCalcWasm, wazero.NewModuleConfig())
	if err != nil {
		runtime.Close(ctx)
		return nil, fmt.Errorf("wasm 实例化失败: %w", err)
	}
	b := &WasmBackend{runtime: runtime, module: mod, mem: mod.Memory()}
	b.alloc = mod.ExportedFunction("dobcc_alloc")
	b.dealloc = mod.ExportedFunction("dobcc_dealloc")
	b.eval = mod.ExportedFunction("dobcc_calc_evaluate")
	b.skillFieldsFn = mod.ExportedFunction("dobcc_skill_fields")
	b.panelsFn = mod.ExportedFunction("dobcc_panels")
	openSession := mod.ExportedFunction("dobcc_open_session")
	calcOpen := mod.ExportedFunction("dobcc_calc_open")
	blob, err := tablesToModulesJSON(tables)
	if err != nil {
		b.Close()
		return nil, err
	}
	tablesPtr, err := b.callAlloc(ctx, uint64(len(blob)))
	if err != nil {
		b.Close()
		return nil, err
	}
	if !b.mem.Write(uint32(tablesPtr), blob) {
		b.callDealloc(ctx, tablesPtr, uint64(len(blob)))
		b.Close()
		return nil, errors.New("wasm 内存写入失败（建表）")
	}
	session, err := callU64(openSession, ctx, tablesPtr, uint64(len(blob)))
	b.callDealloc(ctx, tablesPtr, uint64(len(blob)))
	if err != nil {
		b.Close()
		return nil, fmt.Errorf("wasm 建表失败: %w", err)
	}
	if session == 0 {
		b.Close()
		return nil, errors.New("wasm 建表失败（open_session 返回 0）")
	}
	calc, err := callU64(calcOpen, ctx, session)
	if err != nil {
		b.Close()
		return nil, fmt.Errorf("wasm 计算会话创建失败: %w", err)
	}
	if calc == 0 {
		b.Close()
		return nil, errors.New("wasm 计算会话创建失败（calc_open 返回 0）")
	}
	b.session = session
	b.calc = calc
	return b, nil
}

func callU64(fn api.Function, ctx context.Context, params ...uint64) (uint64, error) {
	results, err := fn.Call(ctx, params...)
	if err != nil {
		return 0, err
	}
	if len(results) == 0 {
		return 0, errors.New("wasm 函数无返回值")
	}
	return results[0], nil
}

func (b *WasmBackend) callAlloc(ctx context.Context, size uint64) (uint64, error) {
	return callU64(b.alloc, ctx, size)
}

func (b *WasmBackend) callDealloc(ctx context.Context, ptr, size uint64) {
	if ptr == 0 {
		return
	}
	_, _ = b.dealloc.Call(ctx, ptr, size)
}

// callRaw 调 dobcc_*evaluate 系函数，返回解包后的信封 map。
func (b *WasmBackend) callRaw(ctx context.Context, fn api.Function, session uint64, req []byte) (map[string]any, error) {
	reqPtr, err := b.callAlloc(ctx, uint64(len(req)))
	if err != nil {
		return nil, err
	}
	defer b.callDealloc(ctx, reqPtr, uint64(len(req)))
	if !b.mem.Write(uint32(reqPtr), req) {
		return nil, errors.New("wasm 内存写入失败（请求）")
	}
	lenPtr, err := b.callAlloc(ctx, 8)
	if err != nil {
		return nil, err
	}
	defer b.callDealloc(ctx, lenPtr, 8)
	zero := make([]byte, 8)
	if !b.mem.Write(uint32(lenPtr), zero) {
		return nil, errors.New("wasm 内存写入失败（长度槽）")
	}
	outPtr, err := callU64(fn, ctx, session, reqPtr, uint64(len(req)), lenPtr)
	if err != nil {
		return nil, fmt.Errorf("wasm 求值调用失败: %w", err)
	}
	lenBytes, ok := b.mem.Read(uint32(lenPtr), 8)
	if !ok {
		return nil, errors.New("wasm 长度回读失败")
	}
	outLen := binary.LittleEndian.Uint32(lenBytes)
	var out []byte
	if outPtr != 0 && outLen != 0 {
		out, ok = b.mem.Read(uint32(outPtr), outLen)
		if !ok {
			return nil, errors.New("wasm 结果回读失败")
		}
		out = append([]byte{}, out...)
		b.callDealloc(ctx, outPtr, uint64(outLen))
	}
	if len(out) == 0 {
		return nil, errors.New("wasm 返回空结果")
	}
	var envelope map[string]any
	if err := json.Unmarshal(out, &envelope); err != nil {
		return nil, fmt.Errorf("wasm 结果解析失败: %w", err)
	}
	return envelope, nil
}

// Evaluate 求值一份构筑，返回结果 result 段。target 非空时覆盖 settings 的 targetFunction。
func (b *WasmBackend) Evaluate(charID int, settings map[string]any, target string) (map[string]any, error) {
	ctx := context.Background()
	payload := map[string]any{}
	for k, v := range settings {
		payload[k] = v
	}
	if target != "" {
		payload["targetFunction"] = target
	}
	req, err := json.Marshal(map[string]any{"charId": charID, "settings": payload})
	if err != nil {
		return nil, err
	}
	envelope, err := b.callRaw(ctx, b.eval, b.calc, req)
	if err != nil {
		return nil, err
	}
	if IsTrue(envelope["ok"]) {
		if result, ok := envelope["result"].(map[string]any); ok {
			return result, nil
		}
		return map[string]any{}, nil
	}
	msg, _ := envelope["error"].(string)
	if msg == "" {
		msg = "未知错误"
	}
	return nil, &WasmEvaluateError{Message: msg}
}

// SkillFields 取某技能的结算字段表（skill 按名称或 safeName 匹配；未知返回空）。
func (b *WasmBackend) SkillFields(charID int, settings map[string]any, skill string) ([]any, error) {
	ctx := context.Background()
	req, err := json.Marshal(map[string]any{"charId": charID, "settings": settings, "skill": skill})
	if err != nil {
		return nil, err
	}
	envelope, err := b.callRaw(ctx, b.skillFieldsFn, b.session, req)
	if err != nil {
		return nil, err
	}
	if !IsTrue(envelope["ok"]) {
		msg, _ := envelope["error"].(string)
		if msg == "" {
			msg = "未知错误"
		}
		return nil, &WasmEvaluateError{Message: msg}
	}
	result, _ := envelope["result"].(map[string]any)
	fields, _ := result["fields"].([]any)
	if fields == nil {
		return []any{}, nil
	}
	return fields, nil
}

// Panels 取全槽武器面板（键集与上游 weapon_panels 一致）。
func (b *WasmBackend) Panels(charID int, settings map[string]any) (map[string]any, error) {
	ctx := context.Background()
	req, err := json.Marshal(map[string]any{"charId": charID, "settings": settings})
	if err != nil {
		return nil, err
	}
	envelope, err := b.callRaw(ctx, b.panelsFn, b.session, req)
	if err != nil {
		return nil, err
	}
	if !IsTrue(envelope["ok"]) {
		msg, _ := envelope["error"].(string)
		if msg == "" {
			msg = "未知错误"
		}
		return nil, &WasmEvaluateError{Message: msg}
	}
	result, _ := envelope["result"].(map[string]any)
	panels, _ := result["panels"].(map[string]any)
	if panels == nil {
		return map[string]any{}, nil
	}
	return panels, nil
}

// Close 释放计算会话与表会话（重复关闭安全）。
func (b *WasmBackend) Close() {
	if b.closed {
		return
	}
	b.closed = true
	ctx := context.Background()
	if b.calc != 0 {
		if fn := b.module.ExportedFunction("dobcc_calc_close"); fn != nil {
			_, _ = fn.Call(ctx, b.calc)
		}
		b.calc = 0
	}
	if b.session != 0 {
		if fn := b.module.ExportedFunction("dobcc_close_session"); fn != nil {
			_, _ = fn.Call(ctx, b.session)
		}
		b.session = 0
	}
	_ = b.runtime.Close(ctx)
}
