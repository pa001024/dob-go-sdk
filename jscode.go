// BUFF 动态 code 的 JS 子集解释器（语料 14 段：var、if/else、块、表达式语句、
// ?:、||、&&、比较、四则、%、一元 -/!、Math.*、成员读写链、赋值、逗号序列）。
package dob

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// JsNaN 即 JS 的 NaN（undefined 参与算术的值）。
var JsNaN = math.NaN()

// JsError 是解释器错误。
type JsError struct{ Msg string }

func (e *JsError) Error() string { return e.Msg }

func jsTruthy(v V) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case float64:
		return !math.IsNaN(t) && t != 0
	case float32:
		return !math.IsNaN(float64(t)) && t != 0
	case int:
		return t != 0
	case int64:
		return t != 0
	case string:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	case ZeroMap:
		return len(t) > 0
	case []any:
		return len(t) > 0
	default:
		return true
	}
}

func jsNum(v V) float64 {
	if v == nil {
		return math.NaN()
	}
	if b, ok := v.(bool); ok {
		if b {
			return 1
		}
		return 0
	}
	// json.Number（含大 id）与各整数类型统一经 Num 口径转换
	if IsNum(v) {
		return Num(v)
	}
	if s, ok := v.(string); ok {
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return math.NaN()
		}
		return f
	}
	return math.NaN()
}

func jsEq(a, b V) bool {
	if _, ok := a.(bool); ok {
		return jsNum(a) == jsNum(b)
	}
	if _, ok := b.(bool); ok {
		return jsNum(a) == jsNum(b)
	}
	if a == nil {
		return false
	}
	if b == nil {
		return false
	}
	if IsNum(a) && math.IsNaN(Num(a)) {
		return false
	}
	if IsNum(b) && math.IsNaN(Num(b)) {
		return false
	}
	if IsNum(a) && IsNum(b) {
		return jsNum(a) == jsNum(b)
	}
	// 其余按 Python `==`：字符串仅与字符串相等，其余一律不等
	sa, oka := a.(string)
	sb, okb := b.(string)
	if oka && okb {
		return sa == sb
	}
	return false
}

func jsSafeDiv(l, r float64) float64 {
	if r == 0 {
		if l == 0 || math.IsNaN(l) {
			return math.NaN()
		}
		if l > 0 {
			return math.Inf(1)
		}
		return math.Inf(-1)
	}
	return l / r
}

func jsSafeMod(l, r float64) float64 {
	if r == 0 || math.IsNaN(r) {
		return math.NaN()
	}
	if math.IsNaN(l) || math.IsInf(l, 0) {
		return math.NaN()
	}
	if math.IsInf(r, 0) {
		return l
	}
	return math.Mod(l, r)
}

func jsMin(args []float64) float64 {
	for _, a := range args {
		if math.IsNaN(a) {
			return math.NaN()
		}
	}
	m := args[0]
	for _, a := range args[1:] {
		if a < m {
			m = a
		}
	}
	return m
}

func jsMax(args []float64) float64 {
	for _, a := range args {
		if math.IsNaN(a) {
			return math.NaN()
		}
	}
	m := args[0]
	for _, a := range args[1:] {
		if a > m {
			m = a
		}
	}
	return m
}

func jsMathTotal(f func(float64) float64, x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	return f(x)
}

func jsSafePow(l, r float64) float64 {
	p := math.Pow(l, r)
	if math.IsNaN(p) {
		return math.NaN()
	}
	return p
}

// ---------- 词法 ----------

type jsTokKind string

const (
	jsEOF jsTokKind = "EOF"
	jsNUM jsTokKind = "NUM"
	jsSTR jsTokKind = "STR"
	jsID  jsTokKind = "ID"
	jsKW  jsTokKind = "KW"
	jsOP  jsTokKind = "OP"
)

type jsToken struct {
	Kind  jsTokKind
	Value string
}

var jsKeywords = map[string]bool{"var": true, "if": true, "else": true, "true": true, "false": true, "undefined": true, "null": true}

func jsTokenize(text string) ([]jsToken, error) {
	var toks []jsToken
	runes := []rune(text)
	n := len(runes)
	pos := 0
	three := map[string]bool{"===": true, "!==": true}
	two := map[string]bool{"==": true, "!=": true, "<=": true, ">=": true, "&&": true, "||": true, "+=": true, "-=": true, "*=": true, "/=": true, "%=": true}
	for pos < n {
		ch := runes[pos]
		if unicode.IsSpace(ch) {
			pos++
			continue
		}
		if pos+3 <= n && three[string(runes[pos:pos+3])] {
			toks = append(toks, jsToken{Kind: jsOP, Value: string(runes[pos : pos+3])})
			pos += 3
			continue
		}
		if pos+2 <= n && two[string(runes[pos:pos+2])] {
			toks = append(toks, jsToken{Kind: jsOP, Value: string(runes[pos : pos+2])})
			pos += 2
			continue
		}
		if strings.ContainsRune("+-*/%<>=!?:;,(){}._$", ch) || ch == '.' {
			toks = append(toks, jsToken{Kind: jsOP, Value: string(ch)})
			pos++
			continue
		}
		if unicode.IsDigit(ch) || (ch == '.' && pos+1 < n && unicode.IsDigit(runes[pos+1])) {
			start := pos
			for pos < n && (unicode.IsDigit(runes[pos]) || strings.ContainsRune(".eE+-", runes[pos])) {
				if (runes[pos] == '+' || runes[pos] == '-') && (pos == 0 || (runes[pos-1] != 'e' && runes[pos-1] != 'E')) {
					break
				}
				pos++
			}
			toks = append(toks, jsToken{Kind: jsNUM, Value: string(runes[start:pos])})
			continue
		}
		if ch == '\'' || ch == '"' {
			quote := ch
			pos++
			var sb strings.Builder
			for pos < n && runes[pos] != quote {
				if runes[pos] == '\\' && pos+1 < n {
					sb.WriteRune(runes[pos+1])
					pos += 2
				} else {
					sb.WriteRune(runes[pos])
					pos++
				}
			}
			pos++
			toks = append(toks, jsToken{Kind: jsSTR, Value: sb.String()})
			continue
		}
		if unicode.IsLetter(ch) || ch == '_' || ch == '$' || ch > 127 {
			start := pos
			for pos < n && (unicode.IsLetter(runes[pos]) || unicode.IsDigit(runes[pos]) || runes[pos] == '_' || runes[pos] == '$' || runes[pos] > 127) {
				pos++
			}
			word := string(runes[start:pos])
			if jsKeywords[word] {
				toks = append(toks, jsToken{Kind: jsKW, Value: word})
			} else {
				toks = append(toks, jsToken{Kind: jsID, Value: word})
			}
			continue
		}
		return nil, &JsError{Msg: fmt.Sprintf("未知字符 %q 位于 %d", ch, pos)}
	}
	return append(toks, jsToken{Kind: jsEOF}), nil
}

// ---------- 语法 ----------

type jsNodeKind string

const (
	jsProgram jsNodeKind = "program"
	jsNoop    jsNodeKind = "noop"
	jsBlock   jsNodeKind = "block"
	jsVar     jsNodeKind = "var"
	jsIf      jsNodeKind = "if"
	jsExpr    jsNodeKind = "expr"
	jsNumber  jsNodeKind = "num"
	jsStr     jsNodeKind = "str"
	jsIDN     jsNodeKind = "id"
	jsMember  jsNodeKind = "member"
	jsCall    jsNodeKind = "call"
	jsUnary   jsNodeKind = "unary"
	jsArith   jsNodeKind = "arith"
	jsCmp     jsNodeKind = "cmp"
	jsEqN     jsNodeKind = "eq"
	jsOr      jsNodeKind = "or"
	jsAnd     jsNodeKind = "and"
	jsCond    jsNodeKind = "cond"
	jsComma   jsNodeKind = "comma"
	jsAssign  jsNodeKind = "assign"
)

// JsNode 是解释器节点。
type JsNode struct {
	Kind     jsNodeKind
	Op       string
	Name     string
	Value    float64
	Str      string
	Children []*JsNode
	Decls    []JsDecl
	Cond     *JsNode
	Then     *JsNode
	Else     *JsNode
}

// JsDecl 是 var 声明器。
type JsDecl struct {
	Name  string
	Value *JsNode
}

type jsParser struct {
	tokens []jsToken
	pos    int
}

func (p *jsParser) peek() jsToken { return p.tokens[p.pos] }

func (p *jsParser) next() jsToken {
	t := p.tokens[p.pos]
	p.pos++
	return t
}

func (p *jsParser) match(kind jsTokKind, value string) bool {
	t := p.peek()
	if t.Kind != kind || (value != "" && t.Value != value) {
		return false
	}
	p.pos++
	return true
}

func (p *jsParser) expect(kind jsTokKind, value string) (jsToken, error) {
	t := p.next()
	if t.Kind != kind || (value != "" && t.Value != value) {
		return jsToken{}, &JsError{Msg: fmt.Sprintf("期望 %s %s，实际 %v", kind, value, t)}
	}
	return t, nil
}

func jsParse(text string) (*JsNode, error) {
	toks, err := jsTokenize(text)
	if err != nil {
		return nil, err
	}
	p := &jsParser{tokens: toks}
	var body []*JsNode
	for p.peek().Kind != jsEOF {
		if p.peek().Kind == jsOP && p.peek().Value == ";" {
			p.pos++
			continue
		}
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		body = append(body, stmt)
	}
	return &JsNode{Kind: jsProgram, Children: body}, nil
}

func (p *jsParser) parseStatement() (*JsNode, error) {
	t := p.peek()
	if t.Kind == jsKW && t.Value == "var" {
		return p.parseVar()
	}
	if t.Kind == jsKW && t.Value == "if" {
		return p.parseIf()
	}
	if t.Kind == jsOP && t.Value == "{" {
		return p.parseBlock()
	}
	if t.Kind == jsOP && t.Value == ";" {
		p.pos++
		return &JsNode{Kind: jsNoop}, nil
	}
	node, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	p.match(jsOP, ";")
	return &JsNode{Kind: jsExpr, Children: []*JsNode{node}}, nil
}

func (p *jsParser) parseVar() (*JsNode, error) {
	if _, err := p.expect(jsKW, "var"); err != nil {
		return nil, err
	}
	var decls []JsDecl
	for {
		name, err := p.expect(jsID, "")
		if err != nil {
			return nil, err
		}
		var value *JsNode
		if p.match(jsOP, "=") {
			value, err = p.parseAssign()
			if err != nil {
				return nil, err
			}
		}
		decls = append(decls, JsDecl{Name: name.Value, Value: value})
		if !p.match(jsOP, ",") {
			break
		}
	}
	p.match(jsOP, ";")
	return &JsNode{Kind: jsVar, Decls: decls}, nil
}

func (p *jsParser) parseIf() (*JsNode, error) {
	if _, err := p.expect(jsKW, "if"); err != nil {
		return nil, err
	}
	if _, err := p.expect(jsOP, "("); err != nil {
		return nil, err
	}
	cond, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(jsOP, ")"); err != nil {
		return nil, err
	}
	then, err := p.parseStatement()
	if err != nil {
		return nil, err
	}
	var otherwise *JsNode
	if p.match(jsKW, "else") {
		otherwise, err = p.parseStatement()
		if err != nil {
			return nil, err
		}
	}
	return &JsNode{Kind: jsIf, Cond: cond, Then: then, Else: otherwise}, nil
}

func (p *jsParser) parseBlock() (*JsNode, error) {
	if _, err := p.expect(jsOP, "{"); err != nil {
		return nil, err
	}
	var body []*JsNode
	for !p.match(jsOP, "}") {
		if p.peek().Kind == jsEOF {
			return nil, &JsError{Msg: "块未闭合"}
		}
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		body = append(body, stmt)
	}
	return &JsNode{Kind: jsBlock, Children: body}, nil
}

func (p *jsParser) parseExpression() (*JsNode, error) { return p.parseComma() }

func (p *jsParser) parseComma() (*JsNode, error) {
	node, err := p.parseAssign()
	if err != nil {
		return nil, err
	}
	for p.match(jsOP, ",") {
		right, err := p.parseAssign()
		if err != nil {
			return nil, err
		}
		node = &JsNode{Kind: jsComma, Children: []*JsNode{node, right}}
	}
	return node, nil
}

var jsAssignOps = map[string]bool{"=": true, "+=": true, "-=": true, "*=": true, "/=": true, "%=": true}

func (p *jsParser) parseAssign() (*JsNode, error) {
	node, err := p.parseConditional()
	if err != nil {
		return nil, err
	}
	t := p.peek()
	if t.Kind == jsOP && jsAssignOps[t.Value] {
		p.pos++
		if node.Kind != jsIDN && node.Kind != jsMember {
			return nil, &JsError{Msg: "赋值目标非法"}
		}
		right, err := p.parseAssign()
		if err != nil {
			return nil, err
		}
		return &JsNode{Kind: jsAssign, Op: t.Value, Children: []*JsNode{node, right}}, nil
	}
	return node, nil
}

func (p *jsParser) parseConditional() (*JsNode, error) {
	node, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.match(jsOP, "?") {
		then, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(jsOP, ":"); err != nil {
			return nil, err
		}
		otherwise, err := p.parseConditional()
		if err != nil {
			return nil, err
		}
		return &JsNode{Kind: jsCond, Children: []*JsNode{node, then, otherwise}}, nil
	}
	return node, nil
}

func (p *jsParser) parseOr() (*JsNode, error) {
	node, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.match(jsOP, "||") {
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		node = &JsNode{Kind: jsOr, Children: []*JsNode{node, right}}
	}
	return node, nil
}

func (p *jsParser) parseAnd() (*JsNode, error) {
	node, err := p.parseEq()
	if err != nil {
		return nil, err
	}
	for p.match(jsOP, "&&") {
		right, err := p.parseEq()
		if err != nil {
			return nil, err
		}
		node = &JsNode{Kind: jsAnd, Children: []*JsNode{node, right}}
	}
	return node, nil
}

var jsEqOps = map[string]bool{"==": true, "!=": true, "===": true, "!==": true}

func (p *jsParser) parseEq() (*JsNode, error) {
	node, err := p.parseCmp()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.Kind == jsOP && jsEqOps[t.Value] {
			p.pos++
			right, err := p.parseCmp()
			if err != nil {
				return nil, err
			}
			node = &JsNode{Kind: jsEqN, Op: t.Value, Children: []*JsNode{node, right}}
		} else {
			return node, nil
		}
	}
}

var jsCmpOps = map[string]bool{"<": true, "<=": true, ">": true, ">=": true}

func (p *jsParser) parseCmp() (*JsNode, error) {
	node, err := p.parseAdd()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.Kind == jsOP && jsCmpOps[t.Value] {
			p.pos++
			right, err := p.parseAdd()
			if err != nil {
				return nil, err
			}
			node = &JsNode{Kind: jsCmp, Op: t.Value, Children: []*JsNode{node, right}}
		} else {
			return node, nil
		}
	}
}

func (p *jsParser) parseAdd() (*JsNode, error) {
	node, err := p.parseMul()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.Kind == jsOP && (t.Value == "+" || t.Value == "-") {
			p.pos++
			right, err := p.parseMul()
			if err != nil {
				return nil, err
			}
			node = &JsNode{Kind: jsArith, Op: t.Value, Children: []*JsNode{node, right}}
		} else {
			return node, nil
		}
	}
}

func (p *jsParser) parseMul() (*JsNode, error) {
	node, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.Kind == jsOP && (t.Value == "*" || t.Value == "/" || t.Value == "%") {
			p.pos++
			right, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			node = &JsNode{Kind: jsArith, Op: t.Value, Children: []*JsNode{node, right}}
		} else {
			return node, nil
		}
	}
}

func (p *jsParser) parseUnary() (*JsNode, error) {
	t := p.peek()
	if t.Kind == jsOP && (t.Value == "-" || t.Value == "!" || t.Value == "+") {
		p.pos++
		inner, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &JsNode{Kind: jsUnary, Op: t.Value, Children: []*JsNode{inner}}, nil
	}
	return p.parsePostfix()
}

func (p *jsParser) parsePostfix() (*JsNode, error) {
	node, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.Kind == jsOP && t.Value == "." {
			p.pos++
			prop, err := p.expect(jsID, "")
			if err != nil {
				return nil, err
			}
			node = &JsNode{Kind: jsMember, Name: prop.Value, Children: []*JsNode{node}}
		} else if t.Kind == jsOP && t.Value == "(" {
			p.pos++
			var args []*JsNode
			if !(p.peek().Kind == jsOP && p.peek().Value == ")") {
				for {
					a, err := p.parseAssign()
					if err != nil {
						return nil, err
					}
					args = append(args, a)
					if !p.match(jsOP, ",") {
						break
					}
				}
			}
			if _, err := p.expect(jsOP, ")"); err != nil {
				return nil, err
			}
			node = &JsNode{Kind: jsCall, Children: append([]*JsNode{node}, args...)}
		} else {
			return node, nil
		}
	}
}

func (p *jsParser) parsePrimary() (*JsNode, error) {
	t := p.next()
	switch t.Kind {
	case jsNUM:
		f, err := strconv.ParseFloat(t.Value, 64)
		if err != nil {
			return nil, &JsError{Msg: fmt.Sprintf("非法数字 %q", t.Value)}
		}
		return &JsNode{Kind: jsNumber, Value: f}, nil
	case jsSTR:
		return &JsNode{Kind: jsStr, Str: t.Value}, nil
	case jsID:
		return &JsNode{Kind: jsIDN, Name: t.Value}, nil
	case jsKW:
		switch t.Value {
		case "true":
			return &JsNode{Kind: jsNumber, Value: 1}, nil
		case "false":
			return &JsNode{Kind: jsNumber, Value: 0}, nil
		case "undefined", "null":
			return &JsNode{Kind: jsNumber, Value: math.NaN()}, nil
		}
		return nil, &JsError{Msg: fmt.Sprintf("意外的标记 %v", t)}
	case jsOP:
		if t.Value == "(" {
			node, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(jsOP, ")"); err != nil {
				return nil, err
			}
			return node, nil
		}
	}
	return nil, &JsError{Msg: fmt.Sprintf("意外的标记 %v", t)}
}

// ---------- 求值 ----------

// JsScope 是 with+sloppy 作用域：locals → sandbox → 隐式全局。
type JsScope struct {
	Locals   map[string]V
	Sandbox  map[string]any
	Implicit map[string]V
}

// NewJsScope 构造作用域。
func NewJsScope(sandbox map[string]any) *JsScope {
	return &JsScope{Locals: map[string]V{}, Sandbox: sandbox, Implicit: map[string]V{}}
}

func (s *JsScope) read(name string) V {
	if v, ok := s.Sandbox[name]; ok {
		return v
	}
	if v, ok := s.Locals[name]; ok {
		return v
	}
	if name == "Math" {
		return "Math"
	}
	if v, ok := s.Implicit[name]; ok {
		return v
	}
	return math.NaN()
}

func (s *JsScope) write(name string, value V) {
	if _, ok := s.Sandbox[name]; ok {
		s.Sandbox[name] = value
		return
	}
	if _, ok := s.Locals[name]; ok {
		s.Locals[name] = value
		return
	}
	s.Implicit[name] = value
}

func jsMemberGet(container V, key string) V {
	if container == nil {
		panic(&JsError{Msg: "读取 undefined 的属性: " + key})
	}
	if zm, ok := container.(ZeroMap); ok {
		if v, present := zm[key]; present {
			if v == nil {
				return math.NaN()
			}
			return v
		}
		return 0.0
	}
	if m, ok := container.(map[string]any); ok {
		if v, ok := m[key]; ok {
			if v == nil {
				return math.NaN()
			}
			return v
		}
		return math.NaN()
	}
	return math.NaN()
}

func jsMemberSet(container V, key string, value V) {
	if m, ok := container.(map[string]any); ok {
		m[key] = value
	}
}

func jsEvalMath(name string, args []V) V {
	nums := make([]float64, 0, len(args))
	for _, a := range args {
		nums = append(nums, jsNum(a))
	}
	switch name {
	case "floor":
		return jsMathTotal(math.Floor, nums[0])
	case "ceil":
		return jsMathTotal(math.Ceil, nums[0])
	case "round":
		return jsMathTotal(math.Round, nums[0])
	case "min":
		return jsMin(nums)
	case "max":
		return jsMax(nums)
	case "abs":
		return math.Abs(nums[0])
	case "pow":
		return jsSafePow(nums[0], nums[1])
	case "sqrt":
		if nums[0] >= 0 {
			return math.Sqrt(nums[0])
		}
		return math.NaN()
	case "log":
		if nums[0] > 0 {
			return math.Log(nums[0])
		}
		return math.NaN()
	default:
		panic(&JsError{Msg: "不支持的 Math 方法: " + name})
	}
}

func jsEval(scope *JsScope, node *JsNode) V {
	switch node.Kind {
	case jsNumber:
		return node.Value
	case jsStr:
		return node.Str
	case jsIDN:
		return scope.read(node.Name)
	case jsMember:
		return jsMemberGet(jsEval(scope, node.Children[0]), node.Name)
	case jsCall:
		target := node.Children[0]
		args := make([]V, 0, len(node.Children)-1)
		for _, a := range node.Children[1:] {
			args = append(args, jsEval(scope, a))
		}
		if target.Kind == jsMember && len(target.Children) == 1 && target.Children[0].Kind == jsIDN && target.Children[0].Name == "Math" {
			return jsEvalMath(target.Name, args)
		}
		panic(&JsError{Msg: "仅支持 Math.* 调用"})
	case jsUnary:
		v := jsEval(scope, node.Children[0])
		switch node.Op {
		case "-":
			return -jsNum(v)
		case "+":
			return jsNum(v)
		default:
			if jsTruthy(v) {
				return 0.0
			}
			return 1.0
		}
	case jsArith:
		left, right := jsNum(jsEval(scope, node.Children[0])), jsNum(jsEval(scope, node.Children[1]))
		switch node.Op {
		case "+":
			return left + right
		case "-":
			return left - right
		case "*":
			return left * right
		case "/":
			return jsSafeDiv(left, right)
		default:
			return jsSafeMod(left, right)
		}
	case jsCmp:
		left, right := jsNum(jsEval(scope, node.Children[0])), jsNum(jsEval(scope, node.Children[1]))
		if math.IsNaN(left) || math.IsNaN(right) {
			return 0.0
		}
		var ok bool
		switch node.Op {
		case "<":
			ok = left < right
		case "<=":
			ok = left <= right
		case ">":
			ok = left > right
		default:
			ok = left >= right
		}
		if ok {
			return 1.0
		}
		return 0.0
	case jsEqN:
		result := jsEq(jsEval(scope, node.Children[0]), jsEval(scope, node.Children[1]))
		if node.Op == "==" || node.Op == "===" {
			if result {
				return 1.0
			}
			return 0.0
		}
		if result {
			return 0.0
		}
		return 1.0
	case jsOr:
		left := jsEval(scope, node.Children[0])
		if jsTruthy(left) {
			return left
		}
		return jsEval(scope, node.Children[1])
	case jsAnd:
		left := jsEval(scope, node.Children[0])
		if jsTruthy(left) {
			return jsEval(scope, node.Children[1])
		}
		return left
	case jsCond:
		if jsTruthy(jsEval(scope, node.Children[0])) {
			return jsEval(scope, node.Children[1])
		}
		return jsEval(scope, node.Children[2])
	case jsComma:
		jsEval(scope, node.Children[0])
		return jsEval(scope, node.Children[1])
	case jsAssign:
		target, expr := node.Children[0], node.Children[1]
		value := jsEval(scope, expr)
		if target.Kind == jsIDN {
			name := target.Name
			if node.Op != "=" {
				current := jsNum(scope.read(name))
				value = jsApplyCompound(node.Op, current, jsNum(value))
			}
			scope.write(name, value)
			return value
		}
		// 成员赋值
		container := jsEval(scope, target.Children[0])
		key := target.Name
		current := jsNum(jsMemberGet(container, key))
		if node.Op != "=" {
			value = jsApplyCompound(node.Op, current, jsNum(value))
		}
		if container == nil {
			scope.write(key, value)
		} else {
			jsMemberSet(container, key, value)
		}
		return value
	}
	panic(&JsError{Msg: "未知节点"})
}

func jsApplyCompound(op string, current, value float64) float64 {
	switch op {
	case "+=":
		return current + value
	case "-=":
		return current - value
	case "*=":
		return current * value
	case "/=":
		return jsSafeDiv(current, value)
	default:
		return jsSafeMod(current, value)
	}
}

func jsExec(scope *JsScope, node *JsNode) {
	switch node.Kind {
	case jsProgram, jsBlock:
		for _, stmt := range node.Children {
			jsExec(scope, stmt)
		}
	case jsNoop:
	case jsVar:
		for _, d := range node.Decls {
			if d.Value != nil {
				scope.Locals[d.Name] = jsEval(scope, d.Value)
			} else {
				scope.Locals[d.Name] = math.NaN()
			}
		}
	case jsIf:
		if jsTruthy(jsEval(scope, node.Cond)) {
			jsExec(scope, node.Then)
		} else if node.Else != nil {
			jsExec(scope, node.Else)
		}
	case jsExpr:
		jsEval(scope, node.Children[0])
	default:
		panic(&JsError{Msg: "未知语句"})
	}
}

// JsParseProgram 仅解析（语料回归用）。
func JsParseProgram(code string) (*JsNode, error) { return jsParse(code) }

// RunJsCode 执行一段 BUFF code，返回执行后的 sandbox（原地修改并返回）。
func RunJsCode(code string, sandbox map[string]any) map[string]any {
	program, err := jsParse(code)
	if err != nil {
		panic(err)
	}
	jsExec(NewJsScope(sandbox), program)
	return sandbox
}
