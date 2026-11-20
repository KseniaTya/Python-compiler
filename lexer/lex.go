package lexer

import "fmt"

type TokType int

const (
	EOF TokType = iota
	NEWLINE
	INDENT
	DEDENT
	IDENT
	NUMBER
	STRING
	OP
	KEYWORD
)

type Token struct {
	Type TokType
	Val  string
	Line int
	Col  int
}

type Lexer struct {
	src         []rune
	pos         int
	line        int
	col         int
	indents     []int
	tokens      []Token
	atLineStart bool
	parenDepth  int
}

func New(src string) *Lexer {
	return &Lexer{
		src:         []rune(src),
		line:        1,
		col:         0,
		indents:     []int{0},
		atLineStart: true,
	}
}

// --- ВОТ ЭТИ ДВА ХЕЛПЕРА ТЫ ПРОПУСТИЛ ---

func (l *Lexer) advance() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	c := l.src[l.pos]
	l.pos++
	l.col++
	return c
}

func (l *Lexer) emit(tt TokType, val string) {
	l.tokens = append(l.tokens, Token{Type: tt, Val: val, Line: l.line, Col: l.col})
}

// --- Дальше твой Tokenize ---

func (l *Lexer) Tokenize() []Token {
	for l.pos < len(l.src) {
		if l.atLineStart && l.parenDepth == 0 {
			l.atLineStart = false
			l.handleLineStart()
			// после обработки отступов мог оказаться EOF
			if l.pos >= len(l.src) {
				break
			}
		}

		c := l.src[l.pos]
		switch {
		case c == '\n':
			l.handleNewline()
		case c == ' ' || c == '\t' || c == '\r':
			l.advance()
		case c == '#':
			for l.pos < len(l.src) && l.src[l.pos] != '\n' {
				l.advance()
			}
		case c >= '0' && c <= '9':
			l.lexNumber()
		case c == '"' || c == '\'':
			l.lexString(c)
		case isIdentStart(c):
			l.lexIdentOrKeyword()
		default:
			l.lexOp()
		}
	}

	// в конце файла: закрыть NEWLINE (если нужно) и все отступы
	if len(l.tokens) > 0 && l.tokens[len(l.tokens)-1].Type != NEWLINE {
		l.emit(NEWLINE, "\n")
	}
	for len(l.indents) > 1 {
		l.indents = l.indents[:len(l.indents)-1]
		l.emit(DEDENT, "")
	}
	l.emit(EOF, "")
	return l.tokens
}

func (l *Lexer) handleLineStart() {
	if l.parenDepth > 0 {
		return
	}
	width := 0
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == ' ' {
			width++
			l.advance()
		} else if c == '\t' {
			width += 8
			l.advance()
		} else if c == '#' {
			for l.pos < len(l.src) && l.src[l.pos] != '\n' {
				l.advance()
			}
		} else {
			break
		}
	}
	if l.pos >= len(l.src) || l.src[l.pos] == '\n' {
		return // пустая строка — отступ не трогаем
	}

	top := l.indents[len(l.indents)-1]
	if width > top {
		l.indents = append(l.indents, width)
		l.emit(INDENT, "")
	} else {
		for width < top {
			l.indents = l.indents[:len(l.indents)-1]
			l.emit(DEDENT, "")
			top = l.indents[len(l.indents)-1]
		}
		if width != top {
			panic(fmt.Sprintf("IndentationError на строке %d", l.line))
		}
	}
}

func (l *Lexer) handleNewline() {
	if l.parenDepth == 0 {
		l.emit(NEWLINE, "\n")
		l.atLineStart = true
	}
	l.line++
	l.col = 0
	l.pos++
}

func (l *Lexer) lexNumber() {
	start := l.pos
	for l.pos < len(l.src) && (l.src[l.pos] >= '0' && l.src[l.pos] <= '9') {
		l.advance()
	}
	if l.pos < len(l.src) && l.src[l.pos] == '.' {
		l.advance()
		for l.pos < len(l.src) && (l.src[l.pos] >= '0' && l.src[l.pos] <= '9') {
			l.advance()
		}
	}
	l.emit(NUMBER, string(l.src[start:l.pos]))
}

func (l *Lexer) lexString(quote rune) {
	l.advance() // открывающая кавычка
	start := l.pos
	for l.pos < len(l.src) && l.src[l.pos] != quote {
		if l.src[l.pos] == '\\' {
			l.advance()
		}
		l.advance()
	}
	val := string(l.src[start:l.pos])
	if l.pos < len(l.src) {
		l.advance() // закрывающая
	}
	l.emit(STRING, val)
}

func (l *Lexer) lexIdentOrKeyword() {
	start := l.pos
	for l.pos < len(l.src) && isIdentPart(l.src[l.pos]) {
		l.advance()
	}
	word := string(l.src[start:l.pos])
	if isKeyword(word) {
		l.emit(KEYWORD, word)
	} else {
		l.emit(IDENT, word)
	}
}

func (l *Lexer) lexOp() {
	two := ""
	if l.pos+1 < len(l.src) {
		two = string(l.src[l.pos : l.pos+2])
	}
	switch two {
	case "**", "//", "==", "!=", "<=", ">=":
		l.pos += 2
		l.col += 2
		l.emit(OP, two)
		return
	}
	c := l.advance()
	switch c {
	case '(', '[', '{':
		l.parenDepth++
	case ')', ']', '}':
		if l.parenDepth > 0 {
			l.parenDepth--
		}
	}
	l.emit(OP, string(c))
}

func isIdentStart(c rune) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
func isIdentPart(c rune) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

var keywords = map[string]bool{
	"if": true, "elif": true, "else": true, "while": true,
	"def": true, "return": true, "pass": true, "break": true, "continue": true,
	"and": true, "or": true, "not": true,
	"True": true, "False": true, "None": true,
}

func isKeyword(s string) bool { return keywords[s] }
