package ruleengine

import (
	"fmt"
	"strings"
	"unicode"
)

// TokenType identifies the kind of token produced by the lexer.
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenNumber
	TokenString
	TokenBool
	TokenIdent
	TokenDot
	TokenComma
	TokenLParen
	TokenRParen
	TokenOp
)

// Token is a single lexical unit with position information for error reporting.
type Token struct {
	Type   TokenType
	Value  string
	Line   int
	Column int
	Pos    int
}

func (t Token) String() string {
	return fmt.Sprintf("{type:%d val:%q line:%d col:%d}", t.Type, t.Value, t.Line, t.Column)
}

// Lexer turns a rule expression string into a stream of tokens.
type Lexer struct {
	input  string
	pos    int
	line   int
	col    int
	tokens []Token
}

func NewLexer(input string) *Lexer {
	return &Lexer{input: input, pos: 0, line: 1, col: 1}
}

func (l *Lexer) Lex() ([]Token, error) {
	for {
		if err := l.skipWhitespaceAndComments(); err != nil {
			return nil, err
		}
		if l.pos >= len(l.input) {
			l.emit(TokenEOF, "")
			break
		}
		ch := l.input[l.pos]
		switch {
		case ch == '.':
			l.emit(TokenDot, ".")
		case ch == ',':
			l.emit(TokenComma, ",")
		case ch == '(':
			l.emit(TokenLParen, "(")
		case ch == ')':
			l.emit(TokenRParen, ")")
		case ch == '"' || ch == '\'':
			tok, err := l.readString(ch)
			if err != nil {
				return nil, err
			}
			l.tokens = append(l.tokens, tok)
		case unicode.IsDigit(rune(ch)):
			if err := l.readNumber(); err != nil {
				return nil, err
			}
		case isIdentStart(ch):
			l.readIdent()
		case isOpChar(ch):
			if err := l.readOp(); err != nil {
				return nil, err
			}
		default:
			return nil, l.errorf("unexpected character %q", ch)
		}
	}
	return l.tokens, nil
}

func (l *Lexer) emit(typ TokenType, val string) {
	l.tokens = append(l.tokens, Token{
		Type:   typ,
		Value:  val,
		Line:   l.line,
		Column: l.col,
		Pos:    l.pos,
	})
	l.advance(len(val))
}

func (l *Lexer) skipBlockComment() error {
	l.advance(2)
	for l.pos < len(l.input) {
		if l.input[l.pos] == '*' && l.pos+1 < len(l.input) && l.input[l.pos+1] == '/' {
			l.advance(2)
			return nil
		}
		l.advance(1)
	}
	return l.errorf("unterminated block comment")
}

func (l *Lexer) advance(n int) {
	for i := 0; i < n && l.pos < len(l.input); i++ {
		if l.input[l.pos] == '\n' {
			l.line++
			l.col = 1
		} else {
			l.col++
		}
		l.pos++
	}
}

func (l *Lexer) skipWhitespaceAndComments() error {
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		switch {
		case ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r':
			l.advance(1)
		case ch == '/' && l.pos+1 < len(l.input) && l.input[l.pos+1] == '/':
			for l.pos < len(l.input) && l.input[l.pos] != '\n' {
				l.advance(1)
			}
		case ch == '/' && l.pos+1 < len(l.input) && l.input[l.pos+1] == '*':
			if err := l.skipBlockComment(); err != nil {
				return err
			}
		default:
			return nil
		}
	}
	return nil
}

func (l *Lexer) readString(quote byte) (Token, error) {
	startLine, startCol, startPos := l.line, l.col, l.pos
	l.advance(1)
	var sb strings.Builder
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch == quote {
			l.advance(1)
			return Token{Type: TokenString, Value: sb.String(), Line: startLine, Column: startCol, Pos: startPos}, nil
		}
		if ch == '\\' && l.pos+1 < len(l.input) {
			next := l.input[l.pos+1]
			switch next {
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case 'r':
				sb.WriteByte('\r')
			case '\\':
				sb.WriteByte('\\')
			case '"':
				sb.WriteByte('"')
			case '\'':
				sb.WriteByte('\'')
			default:
				sb.WriteByte(next)
			}
			l.advance(2)
			continue
		}
		sb.WriteByte(ch)
		l.advance(1)
	}
	return Token{}, l.errorfAt(startLine, startCol, "unterminated string literal")
}

func (l *Lexer) readNumber() error {
	startLine, startCol, startPos := l.line, l.col, l.pos
	start := l.pos
	for l.pos < len(l.input) && unicode.IsDigit(rune(l.input[l.pos])) {
		l.advance(1)
	}
	if l.pos < len(l.input) && l.input[l.pos] == '.' {
		if l.pos+1 < len(l.input) && unicode.IsDigit(rune(l.input[l.pos+1])) {
			l.advance(1)
			for l.pos < len(l.input) && unicode.IsDigit(rune(l.input[l.pos])) {
				l.advance(1)
			}
		}
	}
	val := l.input[start:l.pos]
	l.tokens = append(l.tokens, Token{
		Type:   TokenNumber,
		Value:  val,
		Line:   startLine,
		Column: startCol,
		Pos:    startPos,
	})
	return nil
}

func (l *Lexer) readIdent() {
	startLine, startCol, startPos := l.line, l.col, l.pos
	start := l.pos
	for l.pos < len(l.input) && isIdentPart(l.input[l.pos]) {
		l.advance(1)
	}
	val := l.input[start:l.pos]
	typ := TokenIdent
	if val == "true" || val == "false" {
		typ = TokenBool
	}
	l.tokens = append(l.tokens, Token{
		Type:   typ,
		Value:  val,
		Line:   startLine,
		Column: startCol,
		Pos:    startPos,
	})
}

func (l *Lexer) readOp() error {
	startLine, startCol, startPos := l.line, l.col, l.pos
	ch := l.input[l.pos]
	next := byte(0)
	if l.pos+1 < len(l.input) {
		next = l.input[l.pos+1]
	}
	switch ch {
	case '=':
		if next == '=' {
			l.tokens = append(l.tokens, Token{Type: TokenOp, Value: "==", Line: startLine, Column: startCol, Pos: startPos})
			l.advance(2)
			return nil
		}
		return l.errorf("expected '==', got '='")
	case '!':
		if next == '=' {
			l.tokens = append(l.tokens, Token{Type: TokenOp, Value: "!=", Line: startLine, Column: startCol, Pos: startPos})
			l.advance(2)
			return nil
		}
		l.tokens = append(l.tokens, Token{Type: TokenOp, Value: "!", Line: startLine, Column: startCol, Pos: startPos})
		l.advance(1)
		return nil
	case '<':
		if next == '=' {
			l.tokens = append(l.tokens, Token{Type: TokenOp, Value: "<=", Line: startLine, Column: startCol, Pos: startPos})
			l.advance(2)
		} else {
			l.tokens = append(l.tokens, Token{Type: TokenOp, Value: "<", Line: startLine, Column: startCol, Pos: startPos})
			l.advance(1)
		}
		return nil
	case '>':
		if next == '=' {
			l.tokens = append(l.tokens, Token{Type: TokenOp, Value: ">=", Line: startLine, Column: startCol, Pos: startPos})
			l.advance(2)
		} else {
			l.tokens = append(l.tokens, Token{Type: TokenOp, Value: ">", Line: startLine, Column: startCol, Pos: startPos})
			l.advance(1)
		}
		return nil
	case '&':
		if next == '&' {
			l.tokens = append(l.tokens, Token{Type: TokenOp, Value: "&&", Line: startLine, Column: startCol, Pos: startPos})
			l.advance(2)
			return nil
		}
		return l.errorf("expected '&&', got '&'")
	case '|':
		if next == '|' {
			l.tokens = append(l.tokens, Token{Type: TokenOp, Value: "||", Line: startLine, Column: startCol, Pos: startPos})
			l.advance(2)
			return nil
		}
		return l.errorf("expected '||', got '|'")
	case '+':
		l.emit(TokenOp, "+")
	case '-':
		l.emit(TokenOp, "-")
	case '*':
		l.emit(TokenOp, "*")
	case '/':
		l.emit(TokenOp, "/")
	default:
		return l.errorf("unexpected operator %q", ch)
	}
	return nil
}

func (l *Lexer) errorf(format string, args ...any) error {
	return l.errorfAt(l.line, l.col, format, args...)
}

func (l *Lexer) errorfAt(line, col int, format string, args ...any) error {
	return fmt.Errorf("lex error at line %d col %d: %s", line, col, fmt.Sprintf(format, args...))
}

func isIdentStart(ch byte) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isIdentPart(ch byte) bool {
	return isIdentStart(ch) || unicode.IsDigit(rune(ch))
}

func isOpChar(ch byte) bool {
	switch ch {
	case '=', '!', '<', '>', '&', '|', '+', '-', '*', '/':
		return true
	}
	return false
}
