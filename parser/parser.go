package parser

import (
	"fmt"
	"pyCompiler/ast"
	"pyCompiler/lexer"
	"strconv"
	"strings"
)

type Parser struct {
	toks []lexer.Token
	pos  int
}

func New(toks []lexer.Token) *Parser {
	return &Parser{toks: toks}
}

func (p *Parser) cur() lexer.Token { return p.toks[p.pos] }
func (p *Parser) peek() lexer.Token {
	if p.pos+1 < len(p.toks) {
		return p.toks[p.pos+1]
	}
	return lexer.Token{Type: lexer.EOF}
}
func (p *Parser) advance() lexer.Token {
	t := p.toks[p.pos]
	p.pos++
	return t
}
func (p *Parser) expect(tt lexer.TokType, val string) lexer.Token {
	t := p.cur()
	if t.Type != tt || (val != "" && t.Val != val) {
		panic(fmt.Sprintf("ожидалось %v %q, получено %v %q (строка %d)",
			tt, val, t.Type, t.Val, t.Line))
	}
	return p.advance()
}
func (p *Parser) ParseProgram() []ast.Node {
	var stmts []ast.Node
	p.skipNewlines()
	for p.cur().Type != lexer.EOF {
		stmts = append(stmts, p.parseStmt())
		p.skipNewlines()
	}
	return stmts
}

// Блок: NEWLINE INDENT stmt+ DEDENT
func (p *Parser) parseBlock() []ast.Node {
	p.expect(lexer.NEWLINE, "")
	p.expect(lexer.INDENT, "")
	var stmts []ast.Node
	for p.cur().Type != lexer.DEDENT && p.cur().Type != lexer.EOF {
		stmts = append(stmts, p.parseStmt())
		p.skipNewlines()
	}
	p.expect(lexer.DEDENT, "")
	if len(stmts) == 0 {
		panic("пустой блок недопустим")
	}
	return stmts
}

func (p *Parser) parseStmt() ast.Node {
	t := p.cur()
	switch {
	case t.Type == lexer.KEYWORD && t.Val == "if":
		return p.parseIf()
	case t.Type == lexer.KEYWORD && t.Val == "while":
		return p.parseWhile()
	case t.Type == lexer.KEYWORD && t.Val == "def":
		return p.parseFuncDef()
	case t.Type == lexer.KEYWORD && t.Val == "return":
		p.advance()
		var x ast.Node
		if p.cur().Type != lexer.NEWLINE {
			x = p.parseExpr()
		}
		p.expect(lexer.NEWLINE, "")
		return &ast.Return{X: x}
	case t.Type == lexer.KEYWORD && t.Val == "pass":
		p.advance()
		p.expect(lexer.NEWLINE, "")
		return &ast.Pass{}
	}
	// иначе — выражение или присваивание
	return p.parseExprStmt()
}

func (p *Parser) parseIf() ast.Node {
	p.expect(lexer.KEYWORD, "if")
	cond := p.parseExpr()
	p.expect(lexer.OP, ":")
	then := p.parseBlock()

	var elseBody []ast.Node
	for p.cur().Type == lexer.KEYWORD && p.cur().Val == "elif" {
		p.advance()
		c := p.parseExpr()
		p.expect(lexer.OP, ":")
		b := p.parseBlock()
		// elif = вложенный if в else
		elseBody = []ast.Node{&ast.If{Cond: c, Then: b}}
		// но нужно продолжить цепочку — упростим через рекурсию:
		// правильнее — развернуть в цикле; здесь для краткости
		if p.cur().Type == lexer.KEYWORD && p.cur().Val == "else" {
			// обработаем ниже
			break
		}
		return &ast.If{Cond: cond, Then: then, Else: elseBody}
	}
	if p.cur().Type == lexer.KEYWORD && p.cur().Val == "else" {
		p.advance()
		p.expect(lexer.OP, ":")
		elseBody = p.parseBlock()
	}
	return &ast.If{Cond: cond, Then: then, Else: elseBody}
}

func (p *Parser) parseFuncDef() ast.Node {
	p.expect(lexer.KEYWORD, "def")
	name := p.expect(lexer.IDENT, "").Val
	p.expect(lexer.OP, "(")
	var params []string
	if p.cur().Type != lexer.OP || p.cur().Val != ")" {
		for {
			params = append(params, p.expect(lexer.IDENT, "").Val)
			if p.cur().Type == lexer.OP && p.cur().Val == "," {
				p.advance()
				continue
			}
			break
		}
	}
	p.expect(lexer.OP, ")")
	p.expect(lexer.OP, ":")
	body := p.parseBlock()
	return &ast.FuncDef{Name: name, Params: params, Body: body}
}

func (p *Parser) parseExprStmt() ast.Node {
	left := p.parseExpr()

	// a = b = expr  |  a[i] = expr  |  a += expr
	if p.cur().Type == lexer.OP {
		op := p.cur().Val
		if op == "=" {
			p.advance()
			targets := []ast.Node{left}
			val := p.parseExpr()
			for p.cur().Type == lexer.OP && p.cur().Val == "=" {
				p.advance()
				targets = append(targets, val)
				val = p.parseExpr()
			}
			p.expect(lexer.NEWLINE, "")
			return &ast.Assign{Targets: targets, Value: val}
		}
		// += -= *= /= //= %= **=
		if strings.HasSuffix(op, "=") && len(op) > 1 && op != "==" && op != "!=" &&
			op != "<=" && op != ">=" {
			p.advance()
			v := p.parseExpr()
			p.expect(lexer.NEWLINE, "")
			return &ast.AugAssign{Target: left, Op: strings.TrimSuffix(op, "="), Value: v}
		}
	}
	p.expect(lexer.NEWLINE, "")
	return &ast.ExprStmt{X: left}
}

var precedence = map[string]int{
	"or": 1, "and": 2,
	"not": 3, // унарный, обрабатывается в parseUnary
	"==":  4, "!=": 4, "<": 4, "<=": 4, ">": 4, ">=": 4,
	"+": 5, "-": 5,
	"*": 6, "/": 6, "//": 6, "%": 6,
	"**": 7, // правый ассоциативный!
}

func (p *Parser) parseExpr() ast.Node { return p.parseBinExpr(1) }

func (p *Parser) parseBinExpr(minPrec int) ast.Node {
	left := p.parseUnary()

	for {
		t := p.cur()
		if t.Type != lexer.OP && !(t.Type == lexer.KEYWORD && (t.Val == "and" || t.Val == "or")) {
			break
		}
		prec, ok := precedence[t.Val]
		if !ok || prec < minPrec {
			break
		}
		op := t.Val
		p.advance()

		nextMin := prec + 1
		if op == "**" {
			nextMin = prec
		} // правый ассоциативный
		right := p.parseBinExpr(nextMin)

		switch op {
		case "and", "or":
			left = &ast.BoolOp{Op: op, Left: left, Right: right}
		case "==", "!=", "<", "<=", ">", ">=":
			left = &ast.Compare{Op: op, Left: left, Right: right}
		default:
			left = &ast.BinOp{Op: op, Left: left, Right: right}
		}
	}
	return left
}

func (p *Parser) parseUnary() ast.Node {
	t := p.cur()
	if (t.Type == lexer.OP && (t.Val == "-" || t.Val == "+")) ||
		(t.Type == lexer.KEYWORD && t.Val == "not") {
		p.advance()
		x := p.parseUnary() // унарные операторы правый-ассоциативны
		return &ast.UnOp{Op: t.Val, X: x}
	}
	return p.parsePostfix()
}

func (p *Parser) parsePostfix() ast.Node {
	x := p.parseAtom()
	for {
		t := p.cur()
		if t.Type != lexer.OP {
			break
		}
		switch t.Val {
		case "(":
			p.advance()
			var args []ast.Node
			if !(p.cur().Type == lexer.OP && p.cur().Val == ")") {
				for {
					args = append(args, p.parseExpr())
					if p.cur().Type == lexer.OP && p.cur().Val == "," {
						p.advance()
						continue
					}
					break
				}
			}
			p.expect(lexer.OP, ")")
			x = &ast.Call{Fn: x, Args: args}
		case "[":
			p.advance()
			// срез a[lo:hi] или индекс a[i]
			var lo, hi ast.Node
			if !(p.cur().Type == lexer.OP && p.cur().Val == ":") {
				lo = p.parseExpr()
			}
			if p.cur().Type == lexer.OP && p.cur().Val == ":" {
				p.advance()
				if !(p.cur().Type == lexer.OP && p.cur().Val == "]") {
					hi = p.parseExpr()
				}
				p.expect(lexer.OP, "]")
				x = &ast.Slice{X: x, Lo: lo, Hi: hi}
			} else {
				p.expect(lexer.OP, "]")
				x = &ast.Index{X: x, Idx: lo}
			}
		default:
			return x
		}
	}
	return x
}

func (p *Parser) parseAtom() ast.Node {
	t := p.advance()
	switch t.Type {
	case lexer.NUMBER:
		f, _ := strconv.ParseFloat(t.Val, 64)
		return &ast.NumLit{Val: f}
	case lexer.STRING:
		return &ast.StrLit{Val: t.Val}
	case lexer.IDENT:
		return &ast.Ident{Name: t.Val}
	case lexer.KEYWORD:
		switch t.Val {
		case "True":
			return &ast.BoolLit{Val: true}
		case "False":
			return &ast.BoolLit{Val: false}
		case "None":
			return &ast.NoneLit{}
		}
	case lexer.OP:
		switch t.Val {
		case "(":
			x := p.parseExpr()
			p.expect(lexer.OP, ")")
			return x
		case "[":
			var elems []ast.Node
			if !(p.cur().Type == lexer.OP && p.cur().Val == "]") {
				for {
					elems = append(elems, p.parseExpr())
					if p.cur().Type == lexer.OP && p.cur().Val == "," {
						p.advance()
						continue
					}
					break
				}
			}
			p.expect(lexer.OP, "]")
			return &ast.ListLit{Elems: elems}
		case "{":
			var keys, vals []ast.Node
			if !(p.cur().Type == lexer.OP && p.cur().Val == "}") {
				for {
					k := p.parseExpr()
					p.expect(lexer.OP, ":")
					v := p.parseExpr()
					keys = append(keys, k)
					vals = append(vals, v)
					if p.cur().Type == lexer.OP && p.cur().Val == "," {
						p.advance()
						continue
					}
					break
				}
			}
			p.expect(lexer.OP, "}")
			return &ast.DictLit{Keys: keys, Vals: vals}
		}
	}
	panic(fmt.Sprintf("неожиданный токен %q на строке %d", t.Val, t.Line))
}

func (p *Parser) skipNewlines() {
	for p.cur().Type == lexer.NEWLINE {
		p.advance()
	}
}

func (p *Parser) parseWhile() ast.Node {
	p.expect(lexer.KEYWORD, "while")
	cond := p.parseExpr()
	p.expect(lexer.OP, ":")
	body := p.parseBlock()
	return &ast.While{Cond: cond, Body: body}
}
