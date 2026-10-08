// AST 词法/语法：逐行对齐 src/data/ast.ts（Tokenizer + Parser）。
package dob

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// AstError 是解析错误。
type AstError struct{ Msg string }

func (e *AstError) Error() string { return e.Msg }

// jsFloatRe 是 JS parseFloat 可接受的最长数字前缀（TS parseFactor 用 parseFloat
// 转 NUMBER token，如 parseFloat("0.0.0039") === 0；strconv 严格语义会报错，必须镜像）。
var jsFloatRe = regexp.MustCompile(`^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?`)

// jsParseFloat 镜像 JS parseFloat：取最长合法前缀，无有效前缀返回 NaN。
func jsParseFloat(s string) float64 {
	t := strings.TrimLeftFunc(s, unicode.IsSpace)
	rest := t
	if strings.HasPrefix(rest, "+") || strings.HasPrefix(rest, "-") {
		rest = rest[1:]
	}
	if strings.HasPrefix(rest, "Infinity") {
		if strings.HasPrefix(t, "-") {
			return math.Inf(-1)
		}
		return math.Inf(1)
	}
	m := jsFloatRe.FindString(t)
	if m == "" {
		return math.NaN()
	}
	f, err := strconv.ParseFloat(m, 64)
	if err != nil {
		return math.NaN()
	}
	return f
}

// TokKind 词法记号类型。
type TokKind string

const (
	TokEOF    TokKind = "EOF"
	TokNumber TokKind = "NUMBER"
	TokIdent  TokKind = "IDENT"
	TokOp     TokKind = "OP"
	TokDot    TokKind = "DOT"
	TokDColon TokKind = "DCOLON"
	TokColon  TokKind = "COLON"
	TokLParen TokKind = "LPAREN"
	TokRParen TokKind = "RPAREN"
	TokComma  TokKind = "COMMA"
	TokLBrace TokKind = "LBRACE"
	TokRBrace TokKind = "RBRACE"
	TokBang   TokKind = "BANG"
)

// Token 词法记号。
type Token struct {
	Kind  TokKind
	Value string
	Pos   int
}

// NodeType AST 节点类型。
type NodeType string

const (
	NodeBinary   NodeType = "binary"
	NodeUnary    NodeType = "unary"
	NodeProperty NodeType = "property"
	NodeFunction NodeType = "function"
	NodeMember   NodeType = "member_access"
	NodeTemp     NodeType = "temporary_attributes"
	NodeNumber   NodeType = "number"
)

// TempAttr 临时属性项。
type TempAttr struct {
	Name  string
	Value *Node
}

// Node AST 节点。
type Node struct {
	Type      NodeType
	Operator  string
	Left      *Node
	Right     *Node
	Argument  *Node
	Name      string
	Namespace string
	Args      []*Node
	Object    *Node
	Property  string
	Target    *Node
	Attrs     []TempAttr
	ForceAttr bool
	Value     float64
}

type astTokenizer struct {
	text   []rune
	pos    int
	macros map[string]string
}

func isCJK(r rune) bool { return r >= 0x4e00 && r <= 0x9fa5 }

func (t *astTokenizer) skipWs() {
	for t.pos < len(t.text) && unicode.IsSpace(t.text[t.pos]) {
		t.pos++
	}
}

func (t *astTokenizer) nextToken() (Token, error) {
	t.skipWs()
	if t.pos >= len(t.text) {
		return Token{Kind: TokEOF, Pos: t.pos}, nil
	}
	ch := t.text[t.pos]
	if unicode.IsDigit(ch) {
		start := t.pos
		var sb strings.Builder
		for t.pos < len(t.text) && (unicode.IsDigit(t.text[t.pos]) || t.text[t.pos] == '.') {
			sb.WriteRune(t.text[t.pos])
			t.pos++
		}
		return Token{Kind: TokNumber, Value: sb.String(), Pos: start}, nil
	}
	if unicode.IsLetter(ch) || ch == '_' || ch == '[' || isCJK(ch) || ch == '·' {
		start := t.pos
		var sb strings.Builder
		for t.pos < len(t.text) {
			c := t.text[t.pos]
			if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' || isCJK(c) || c == '·' || c == '[' || c == ']' {
				sb.WriteRune(c)
				t.pos++
			} else {
				break
			}
		}
		buf := sb.String()
		if repl, ok := t.macros[buf]; ok {
			runes := []rune(t.text[:start])
			runes = append(runes, []rune(repl)...)
			runes = append(runes, t.text[t.pos:]...)
			t.text = runes
			t.pos = start
			return t.nextToken()
		}
		return Token{Kind: TokIdent, Value: buf, Pos: start}, nil
	}
	if ch == '/' && t.pos+1 < len(t.text) && t.text[t.pos+1] == '/' {
		t.pos += 2
		return Token{Kind: TokOp, Value: "//", Pos: t.pos - 2}, nil
	}
	if strings.ContainsRune("+-*/%", ch) {
		t.pos++
		return Token{Kind: TokOp, Value: string(ch), Pos: t.pos - 1}, nil
	}
	if ch == '.' {
		t.pos++
		return Token{Kind: TokDot, Value: ".", Pos: t.pos - 1}, nil
	}
	if ch == ':' {
		if t.pos+1 < len(t.text) && t.text[t.pos+1] == ':' {
			t.pos += 2
			return Token{Kind: TokDColon, Value: "::", Pos: t.pos - 2}, nil
		}
		t.pos++
		return Token{Kind: TokColon, Value: ":", Pos: t.pos - 1}, nil
	}
	switch ch {
	case '(':
		t.pos++
		return Token{Kind: TokLParen, Value: "(", Pos: t.pos - 1}, nil
	case ')':
		t.pos++
		return Token{Kind: TokRParen, Value: ")", Pos: t.pos - 1}, nil
	case ',':
		t.pos++
		return Token{Kind: TokComma, Value: ",", Pos: t.pos - 1}, nil
	case '{':
		t.pos++
		return Token{Kind: TokLBrace, Value: "{", Pos: t.pos - 1}, nil
	case '}':
		t.pos++
		return Token{Kind: TokRBrace, Value: "}", Pos: t.pos - 1}, nil
	case '!':
		t.pos++
		return Token{Kind: TokBang, Value: "!", Pos: t.pos - 1}, nil
	}
	return Token{}, &AstError{Msg: fmt.Sprintf("未知字符 '%c' 位于位置 %d", ch, t.pos)}
}

type astParser struct {
	tokens []Token
	cur    int
}

func (p *astParser) peek() Token {
	if p.cur < len(p.tokens) {
		return p.tokens[p.cur]
	}
	return Token{Kind: TokEOF}
}

func (p *astParser) prev() Token { return p.tokens[p.cur-1] }

func (p *astParser) atEnd() bool { return p.cur >= len(p.tokens) }

func (p *astParser) check(kind TokKind, value string) bool {
	if p.atEnd() {
		return false
	}
	t := p.peek()
	if t.Kind != kind {
		return false
	}
	return value == "" || t.Value == value
}

func (p *astParser) match(kind TokKind, value string) bool {
	if p.check(kind, value) {
		p.cur++
		return true
	}
	return false
}

func (p *astParser) consume(kind TokKind, msg string) (Token, error) {
	if p.check(kind, "") {
		p.cur++
		return p.prev(), nil
	}
	return Token{}, &AstError{Msg: msg}
}

func (p *astParser) parse() (*Node, error) {
	if len(p.tokens) == 0 {
		return nil, &AstError{Msg: "表达式为空"}
	}
	node, err := p.expr()
	if err != nil {
		return nil, err
	}
	if p.cur < len(p.tokens) {
		if p.peek().Kind == TokColon {
			return nil, &AstError{Msg: "单个冒号 ':' 不支持,请使用 '::' 进行命名空间访问"}
		}
		return nil, &AstError{Msg: fmt.Sprintf("表达式末尾发现意外的标记 '%s'", p.peek().Value)}
	}
	return node, nil
}

func (p *astParser) expr() (*Node, error) {
	left, err := p.term()
	if err != nil {
		return nil, err
	}
	for p.match(TokOp, "+") || p.match(TokOp, "-") {
		op := p.prev().Value
		if p.atEnd() || p.check(TokOp, "") || p.check(TokRParen, "") || p.check(TokComma, "") {
			return nil, &AstError{Msg: fmt.Sprintf("运算符 '%s' 后缺少操作数", op)}
		}
		right, err := p.term()
		if err != nil {
			return nil, err
		}
		left = &Node{Type: NodeBinary, Operator: op, Left: left, Right: right}
	}
	return left, nil
}

func (p *astParser) term() (*Node, error) {
	left, err := p.unary()
	if err != nil {
		return nil, err
	}
	for p.match(TokOp, "*") || p.match(TokOp, "/") || p.match(TokOp, "//") || p.match(TokOp, "%") {
		op := p.prev().Value
		if p.atEnd() || p.check(TokOp, "") || p.check(TokRParen, "") || p.check(TokComma, "") {
			return nil, &AstError{Msg: fmt.Sprintf("运算符 '%s' 后缺少操作数", op)}
		}
		right, err := p.unary()
		if err != nil {
			return nil, err
		}
		left = &Node{Type: NodeBinary, Operator: op, Left: left, Right: right}
	}
	return left, nil
}

func (p *astParser) unary() (*Node, error) {
	if p.match(TokOp, "-") || p.match(TokOp, "+") {
		op := p.prev().Value
		arg, err := p.unary()
		if err != nil {
			return nil, err
		}
		return &Node{Type: NodeUnary, Operator: op, Argument: arg}, nil
	}
	return p.factor()
}

func (p *astParser) factor() (*Node, error) {
	var node *Node
	if p.match(TokNumber, "") {
		f := jsParseFloat(p.prev().Value)
		node = &Node{Type: NodeNumber, Value: f}
	} else if p.match(TokIdent, "") {
		name := p.prev().Value
		var ns string
		if p.match(TokDColon, "") {
			ns = name
			nxt, err := p.consume(TokIdent, "命名空间 '::' 后缺少标识符")
			if err != nil {
				return nil, err
			}
			name = nxt.Value
			if p.match(TokLParen, "") {
				args, err := p.argList()
				if err != nil {
					return nil, err
				}
				node = &Node{Type: NodeFunction, Name: name, Namespace: ns, Args: args}
			} else {
				node = &Node{Type: NodeProperty, Name: name, Namespace: ns}
				node = p.forceSuffix(node)
			}
		} else if p.match(TokLParen, "") {
			args, err := p.argList()
			if err != nil {
				return nil, err
			}
			node = &Node{Type: NodeFunction, Name: name, Args: args}
		} else {
			node = &Node{Type: NodeProperty, Name: name}
			node = p.forceSuffix(node)
		}
	} else if p.match(TokLParen, "") {
		inner, err := p.expr()
		if err != nil {
			return nil, err
		}
		if _, err := p.consume(TokRParen, "表达式后缺少 ')'"); err != nil {
			return nil, err
		}
		node = inner
	} else {
		return nil, &AstError{Msg: fmt.Sprintf("意外的标记: %s", p.peek().Value)}
	}
	for {
		if p.match(TokDot, "") {
			prop, err := p.consume(TokIdent, "成员访问 '.' 后缺少属性名称")
			if err != nil {
				return nil, err
			}
			node = &Node{Type: NodeMember, Object: node, Property: prop.Value}
			continue
		}
		if p.match(TokLBrace, "") {
			if node.Type != NodeProperty && node.Type != NodeMember && node.Type != NodeTemp {
				return nil, &AstError{Msg: "临时属性只能应用于字段"}
			}
			if p.check(TokRBrace, "") {
				return nil, &AstError{Msg: "临时属性不能为空"}
			}
			var attrs []TempAttr
			seen := map[string]bool{}
			for {
				a, err := p.consume(TokIdent, "临时属性缺少属性名")
				if err != nil {
					return nil, err
				}
				if _, err := p.consume(TokColon, fmt.Sprintf("临时属性 '%s' 后缺少 ':'", a.Value)); err != nil {
					return nil, err
				}
				if seen[a.Value] {
					return nil, &AstError{Msg: fmt.Sprintf("临时属性 '%s' 重复", a.Value)}
				}
				seen[a.Value] = true
				val, err := p.expr()
				if err != nil {
					return nil, err
				}
				attrs = append(attrs, TempAttr{Name: a.Value, Value: val})
				if !p.match(TokComma, "") {
					break
				}
			}
			if _, err := p.consume(TokRBrace, "临时属性后缺少 '}'"); err != nil {
				return nil, err
			}
			node = &Node{Type: NodeTemp, Target: node, Attrs: attrs}
			continue
		}
		break
	}
	return node, nil
}

func (p *astParser) argList() ([]*Node, error) {
	var args []*Node
	if !p.check(TokRParen, "") {
		for {
			a, err := p.expr()
			if err != nil {
				return nil, err
			}
			args = append(args, a)
			if !p.match(TokComma, "") {
				break
			}
		}
	}
	if _, err := p.consume(TokRParen, "函数参数后缺少 ')'"); err != nil {
		return nil, err
	}
	return args, nil
}

func (p *astParser) forceSuffix(node *Node) *Node {
	if p.match(TokBang, "") {
		node.ForceAttr = true
	}
	return node
}

// CharMacros 是 CharBuild.macros：AST 表达式宏替换。
var CharMacros = map[string]string{
	"ATK": "攻击", "DEF": "防御", "HP": "生命", "SP": "神智",
	"DPH": "or(多重,1)*伤害", "总伤": "max(1,召唤物攻击次数)*伤害",
	"暴击伤害": "伤害.暴击", "DPS": "or(攻速,1+技能速度)*or(多重,1)*伤害",
	"范围收益": "技能范围*伤害", "耐久收益": "技能耐久*伤害", "效益收益": "技能效益*伤害",
	"每神智DPH": "1/神智消耗*伤害", "每持续神智DPH": "1/每秒神智消耗*伤害",
	"每神智DPS":   "or(攻速,1+技能速度)/神智消耗*伤害",
	"每持续神智DPS": "or(攻速,1+技能速度)/每秒神智消耗*伤害",
}

// ParseAST 解析表达式为 AST（macros 为 nil 沿用 CharMacros）。
func ParseAST(expr string, macros map[string]string) (*Node, error) {
	if macros == nil {
		macros = CharMacros
	}
	tok := &astTokenizer{text: []rune(expr), macros: macros}
	var tokens []Token
	for {
		t, err := tok.nextToken()
		if err != nil {
			return nil, err
		}
		if t.Kind == TokEOF {
			break
		}
		tokens = append(tokens, t)
	}
	return (&astParser{tokens: tokens}).parse()
}

// TokenizeAST 按 AST 词法扫描（宏名原样返回，不替换），供位置扫描使用。
func TokenizeAST(expr string, maxLength int) ([]Token, error) {
	tok := &astTokenizer{text: []rune(expr), macros: map[string]string{}}
	var out []Token
	for {
		t, err := tok.nextToken()
		if err != nil {
			return nil, err
		}
		if t.Kind == TokEOF {
			break
		}
		if maxLength >= 0 && t.Pos >= maxLength {
			break
		}
		out = append(out, t)
	}
	return out, nil
}
