// 表达式 AST 解析与求值用例（纯本地，无网络）。
package dob

import (
	"math"
	"testing"
)

func TestArithmetic(t *testing.T) {
	if got := Evaluate("10 + 4 * 5", nil, nil, nil, nil, nil, nil); got != 30 {
		t.Errorf("got %v", got)
	}
	if got := Evaluate("(10 + 4) * 5", nil, nil, nil, nil, nil, nil); got != 70 {
		t.Errorf("got %v", got)
	}
	if got := Evaluate("-50", nil, nil, nil, nil, nil, nil); got != -50 {
		t.Errorf("got %v", got)
	}
	if got := Evaluate("7 // 2", nil, nil, nil, nil, nil, nil); got != 3 {
		t.Errorf("got %v", got)
	}
	if got := Evaluate("-7 // 2", nil, nil, nil, nil, nil, nil); got != -4 {
		t.Errorf("got %v", got)
	}
	if got := Evaluate("7 % 3", nil, nil, nil, nil, nil, nil); got != 1 {
		t.Errorf("got %v", got)
	}
}

func TestDivZero(t *testing.T) {
	for _, e := range []string{"1 / 0", "1 // 0", "1 % 0"} {
		if got := Evaluate(e, nil, nil, nil, nil, nil, nil); got != 0 {
			t.Errorf("%s got %v", e, got)
		}
	}
}

func TestPropertyAndNamespace(t *testing.T) {
	attrs := map[string]any{"攻击": 100, "ns::攻击": 7}
	if got := Evaluate("攻击 * 2", attrs, nil, nil, nil, nil, nil); got != 200 {
		t.Errorf("got %v", got)
	}
	if got := Evaluate("ns::攻击 + 1", attrs, nil, nil, nil, nil, nil); got != 8 {
		t.Errorf("got %v", got)
	}
	if got := Evaluate("不存在 + 1", attrs, nil, nil, nil, nil, nil); got != 1 {
		t.Errorf("got %v", got)
	}
}

func TestBuiltins(t *testing.T) {
	if got := Evaluate("min(3, 1, 2)", nil, nil, nil, nil, nil, nil); got != 1 {
		t.Errorf("got %v", got)
	}
	if got := Evaluate("max(3, 1, 2)", nil, nil, nil, nil, nil, nil); got != 3 {
		t.Errorf("got %v", got)
	}
	if got := Evaluate("floor(2.7)", nil, nil, nil, nil, nil, nil); got != 2 {
		t.Errorf("got %v", got)
	}
	if got := Evaluate("ceil(2.2)", nil, nil, nil, nil, nil, nil); got != 3 {
		t.Errorf("got %v", got)
	}
	if got := Evaluate("or(0, 0, 5)", nil, nil, nil, nil, nil, nil); got != 5 {
		t.Errorf("got %v", got)
	}
	if got := Evaluate("or(0, 0, 0)", nil, nil, nil, nil, nil, nil); got != 0 {
		t.Errorf("got %v", got)
	}
	if got, want := Evaluate("log(10)", map[string]any{}, nil, nil, nil, nil, nil), math.Log(10); math.Abs(got-want) > 1e-12 {
		t.Errorf("got %v want %v", got, want)
	}
	if got := Evaluate("power(2, 10)", nil, nil, nil, nil, nil, nil); got != 1024 {
		t.Errorf("got %v", got)
	}
}

func TestHP(t *testing.T) {
	attrs := map[string]any{"昂扬": 0.5, "背水": 0.5}
	got := Evaluate("hp(0.5)", attrs, nil, nil, nil, nil, nil)
	want := (1 + 4*0.5*0.5*1.0) * (1 + 0.5*0.5)
	if math.Abs(got-want) > 1e-12 {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestTemporaryAttributes(t *testing.T) {
	attrs := map[string]any{"攻击": 100, "增伤": 0.5}
	a := Evaluate("[攻击]{增伤:0.1}", attrs, nil, nil, nil, nil, nil)
	b := Evaluate("[攻击]", map[string]any{"攻击": 100, "增伤": 0.6}, nil, nil, nil, nil, nil)
	if math.Abs(a-b) > 1e-9 {
		t.Errorf("got %v want %v", a, b)
	}
	if Num(attrs["增伤"]) != 0.5 {
		t.Errorf("原 attrs 被污染")
	}
}

func TestScopeAndCustom(t *testing.T) {
	if got := Evaluate("x * 2", map[string]any{}, map[string]any{"x": 21}, nil, nil, nil, nil); got != 42 {
		t.Errorf("got %v", got)
	}
	vars := map[string]string{"[花刺]层数": "2 + 3"}
	if got := Evaluate("10 + 4 * [花刺]层数", map[string]any{}, nil, vars, nil, nil, nil); got != 30 {
		t.Errorf("got %v", got)
	}
	funcs := map[string]CustomFunc{"double": {Params: []string{"n"}, Body: "n * 2"}}
	if got := Evaluate("double(21)", map[string]any{}, nil, nil, funcs, nil, nil); got != 42 {
		t.Errorf("got %v", got)
	}
	if got := Evaluate("a + 1", map[string]any{}, nil, map[string]string{"a": "a + 1"}, nil, nil, nil); got != 2 {
		t.Errorf("got %v", got)
	}
}

func TestMemberAccess(t *testing.T) {
	attrs := map[string]any{"攻击": 100, "buff": map[string]any{"攻击": 55}}
	if got := Evaluate("buff.攻击", attrs, nil, nil, nil, nil, nil); got != 55 {
		t.Errorf("got %v", got)
	}
	if got := Evaluate("攻击.暴击", attrs, nil, nil, nil, nil, nil); got != 100 {
		t.Errorf("got %v", got)
	}
	if got := Evaluate("攻击.不存在分支", attrs, nil, nil, nil, nil, nil); got != 0 {
		t.Errorf("got %v", got)
	}
}

func TestMacrosAndErrors(t *testing.T) {
	node, err := ParseAST("A", map[string]string{"A": "5 + 5"})
	if err != nil {
		t.Fatal(err)
	}
	if node.Type != NodeBinary {
		t.Errorf("type %v", node.Type)
	}
	if got := Evaluate(node, nil, nil, nil, nil, nil, nil); got != 10 {
		t.Errorf("got %v", got)
	}
	if _, err := ParseAST("", nil); err == nil {
		t.Errorf("空表达式应抛错")
	}
}
