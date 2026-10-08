// 数据包 revive/编解码往返用例（纯本地，无网络）。
package dob

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 手工 msgpack 编码 {"a": 1}：0x81 0xa1 'a' 0x01。
var packModMsgpack = []byte{0x81, 0xa1, 'a', 0x01}

func TestRevive(t *testing.T) {
	value := map[string]any{
		"m": map[string]any{"__dnaPackType": "Map", "value": []any{[]any{map[string]any{"__dnaPackType": "Undefined"}, 1}}},
		"s": map[string]any{"__dnaPackType": "Set", "value": []any{1, 2}},
		"d": map[string]any{"__dnaPackType": "Date", "value": "2026-01-01T00:00:00.000Z"},
		"u": map[string]any{"__dnaPackType": "Undefined"},
		"n": []any{1, map[string]any{"x": 2}},
	}
	out := RevivePackedValue(value).(map[string]any)
	pairs, ok := out["m"].([]any)
	if !ok || len(pairs) != 1 {
		t.Fatalf("m: %v", out["m"])
	}
	pair, ok := pairs[0].([2]any)
	if !ok || pair[0] != nil || Num(pair[1]) != 1 {
		t.Errorf("pair: %v", pairs[0])
	}
	s, _ := out["s"].([]any)
	if len(s) != 2 {
		t.Errorf("s: %v", out["s"])
	}
	if out["u"] != nil {
		t.Errorf("u: %v", out["u"])
	}
	if _, ok := out["d"].(time.Time); !ok {
		t.Errorf("d 非 time: %T", out["d"])
	}
}

func TestZipManifestRoundtrip(t *testing.T) {
	dir, err := os.MkdirTemp("", "dob-pack")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("manifest.json")
	_, _ = w.Write([]byte(`{"version":"t1","modules":{},"rag":{}}`))
	w2, _ := zw.Create("modules/mod.msgpack")
	_, _ = w2.Write(packModMsgpack)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	verDir := filepath.Join(dir, "t1")
	if err := os.MkdirAll(verDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(verDir, "package.zip"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	pack := NewDataPackStore(dir, "", "-")
	m, err := pack.Activate("t1")
	if err != nil {
		t.Fatal(err)
	}
	if S(m, "version") != "t1" {
		t.Errorf("manifest: %v", m)
	}
	mod, err := pack.LoadModule("mod")
	if err != nil {
		t.Fatal(err)
	}
	if Num(mod["a"]) != 1 {
		t.Errorf("mod: %v", mod)
	}
	// 未激活即读模块应报错
	pack2 := NewDataPackStore(dir, "", "-")
	if _, err := pack2.LoadModule("mod"); err == nil {
		t.Errorf("未激活应报错")
	}
}
