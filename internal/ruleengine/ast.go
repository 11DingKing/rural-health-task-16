package ruleengine

import (
	"fmt"
	"strconv"
)

// ValueType represents the type system for the rule expression language.
type ValueType int

const (
	TypeUnknown ValueType = iota
	TypeBool
	TypeNumber
	TypeString
	TypeList
)

func (t ValueType) String() string {
	switch t {
	case TypeBool:
		return "bool"
	case TypeNumber:
		return "number"
	case TypeString:
		return "string"
	case TypeList:
		return "list"
	default:
		return "unknown"
	}
}

// Node is the interface for all AST nodes.
type Node interface {
	Type() ValueType
	String() string
	Pos() (int, int) // line, column
}

// LiteralNode holds a literal value (number, string, bool).
type LiteralNode struct {
	Value  any
	VType  ValueType
	Line   int
	Column int
}

func (n *LiteralNode) Type() ValueType { return n.VType }
func (n *LiteralNode) String() string  { return fmt.Sprintf("%v", n.Value) }
func (n *LiteralNode) Pos() (int, int) { return n.Line, n.Column }

// IdentifierNode represents a field reference like submission.riskLevel.
type IdentifierNode struct {
	Path   []string
	VType  ValueType
	Line   int
	Column int
}

func (n *IdentifierNode) Type() ValueType { return n.VType }
func (n *IdentifierNode) String() string {
	s := ""
	for i, p := range n.Path {
		if i > 0 {
			s += "."
		}
		s += p
	}
	return s
}
func (n *IdentifierNode) Pos() (int, int) { return n.Line, n.Column }

// BinaryOpNode represents a binary operation.
type BinaryOpNode struct {
	Op          string
	Left, Right Node
	Line        int
	Column      int
}

func (n *BinaryOpNode) Type() ValueType {
	switch n.Op {
	case "&&", "||", "==", "!=", "<", ">", "<=", ">=":
		return TypeBool
	case "+", "-", "*", "/":
		return TypeNumber
	}
	return TypeUnknown
}
func (n *BinaryOpNode) String() string {
	return fmt.Sprintf("(%s %s %s)", n.Left.String(), n.Op, n.Right.String())
}
func (n *BinaryOpNode) Pos() (int, int) { return n.Line, n.Column }

// UnaryOpNode represents a unary operation (!, -).
type UnaryOpNode struct {
	Op      string
	Operand Node
	Line    int
	Column  int
}

func (n *UnaryOpNode) Type() ValueType {
	switch n.Op {
	case "!":
		return TypeBool
	case "-":
		return TypeNumber
	}
	return TypeUnknown
}
func (n *UnaryOpNode) String() string {
	return fmt.Sprintf("(%s%s)", n.Op, n.Operand.String())
}
func (n *UnaryOpNode) Pos() (int, int) { return n.Line, n.Column }

// CallNode represents a function call.
type CallNode struct {
	FuncName string
	Args     []Node
	VType    ValueType
	Line     int
	Column   int
}

func (n *CallNode) Type() ValueType { return n.VType }
func (n *CallNode) String() string {
	s := n.FuncName + "("
	for i, a := range n.Args {
		if i > 0 {
			s += ", "
		}
		s += a.String()
	}
	return s + ")"
}
func (n *CallNode) Pos() (int, int) { return n.Line, n.Column }

// Parser is a recursive descent parser that produces an AST from tokens.
type Parser struct {
	tokens []Token
	pos    int
}

func NewParser(tokens []Token) *Parser {
	return &Parser{tokens: tokens}
}

func (p *Parser) Parse() (Node, error) {
	node, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek().Type != TokenEOF {
		tok := p.peek()
		return nil, fmt.Errorf("parse error at line %d col %d: unexpected token %q", tok.Line, tok.Column, tok.Value)
	}
	return node, nil
}

func (p *Parser) parseOr() (Node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().Type == TokenOp && p.peek().Value == "||" {
		tok := p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &BinaryOpNode{Op: tok.Value, Left: left, Right: right, Line: tok.Line, Column: tok.Column}
	}
	return left, nil
}

func (p *Parser) parseAnd() (Node, error) {
	left, err := p.parseEquality()
	if err != nil {
		return nil, err
	}
	for p.peek().Type == TokenOp && p.peek().Value == "&&" {
		tok := p.advance()
		right, err := p.parseEquality()
		if err != nil {
			return nil, err
		}
		left = &BinaryOpNode{Op: tok.Value, Left: left, Right: right, Line: tok.Line, Column: tok.Column}
	}
	return left, nil
}

func (p *Parser) parseEquality() (Node, error) {
	left, err := p.parseComparison()
	if err != nil {
		return nil, err
	}
	for p.peek().Type == TokenOp && (p.peek().Value == "==" || p.peek().Value == "!=") {
		tok := p.advance()
		right, err := p.parseComparison()
		if err != nil {
			return nil, err
		}
		left = &BinaryOpNode{Op: tok.Value, Left: left, Right: right, Line: tok.Line, Column: tok.Column}
	}
	return left, nil
}

func (p *Parser) parseComparison() (Node, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	for p.peek().Type == TokenOp && (p.peek().Value == "<" || p.peek().Value == ">" || p.peek().Value == "<=" || p.peek().Value == ">=") {
		tok := p.advance()
		right, err := p.parseAdditive()
		if err != nil {
			return nil, err
		}
		left = &BinaryOpNode{Op: tok.Value, Left: left, Right: right, Line: tok.Line, Column: tok.Column}
	}
	return left, nil
}

func (p *Parser) parseAdditive() (Node, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for p.peek().Type == TokenOp && (p.peek().Value == "+" || p.peek().Value == "-") {
		tok := p.advance()
		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		left = &BinaryOpNode{Op: tok.Value, Left: left, Right: right, Line: tok.Line, Column: tok.Column}
	}
	return left, nil
}

func (p *Parser) parseMultiplicative() (Node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.peek().Type == TokenOp && (p.peek().Value == "*" || p.peek().Value == "/") {
		tok := p.advance()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &BinaryOpNode{Op: tok.Value, Left: left, Right: right, Line: tok.Line, Column: tok.Column}
	}
	return left, nil
}

func (p *Parser) parseUnary() (Node, error) {
	if p.peek().Type == TokenOp && (p.peek().Value == "!" || p.peek().Value == "-") {
		tok := p.advance()
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &UnaryOpNode{Op: tok.Value, Operand: operand, Line: tok.Line, Column: tok.Column}, nil
	}
	return p.parsePrimary()
}

func (p *Parser) parsePrimary() (Node, error) {
	tok := p.peek()
	switch tok.Type {
	case TokenNumber:
		p.advance()
		val, err := strconv.ParseFloat(tok.Value, 64)
		if err != nil {
			return nil, fmt.Errorf("parse error at line %d col %d: invalid number %q", tok.Line, tok.Column, tok.Value)
		}
		return &LiteralNode{Value: val, VType: TypeNumber, Line: tok.Line, Column: tok.Column}, nil
	case TokenString:
		p.advance()
		return &LiteralNode{Value: tok.Value, VType: TypeString, Line: tok.Line, Column: tok.Column}, nil
	case TokenBool:
		p.advance()
		return &LiteralNode{Value: tok.Value == "true", VType: TypeBool, Line: tok.Line, Column: tok.Column}, nil
	case TokenLParen:
		p.advance()
		node, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek().Type != TokenRParen {
			return nil, fmt.Errorf("parse error at line %d col %d: expected ')'", p.peek().Line, p.peek().Column)
		}
		p.advance()
		return node, nil
	case TokenIdent:
		return p.parseIdentOrCall()
	default:
		return nil, fmt.Errorf("parse error at line %d col %d: unexpected token %q", tok.Line, tok.Column, tok.Value)
	}
}

func (p *Parser) parseIdentOrCall() (Node, error) {
	tok := p.advance()
	path := []string{tok.Value}
	for p.peek().Type == TokenDot {
		p.advance()
		if p.peek().Type != TokenIdent {
			return nil, fmt.Errorf("parse error at line %d col %d: expected identifier after '.'", p.peek().Line, p.peek().Column)
		}
		next := p.advance()
		path = append(path, next.Value)
	}
	if p.peek().Type == TokenLParen {
		return p.parseCall(tok, path)
	}
	return &IdentifierNode{Path: path, Line: tok.Line, Column: tok.Column}, nil
}

func (p *Parser) parseCall(firstTok Token, path []string) (Node, error) {
	funcName := path[0]
	if len(path) > 1 {
		return nil, fmt.Errorf("parse error at line %d col %d: function call with dotted name not supported", firstTok.Line, firstTok.Column)
	}
	p.advance() // consume '('
	args := []Node{}
	if p.peek().Type != TokenRParen {
		arg, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
		for p.peek().Type == TokenComma {
			p.advance()
			arg, err := p.parseOr()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
		}
	}
	if p.peek().Type != TokenRParen {
		return nil, fmt.Errorf("parse error at line %d col %d: expected ')' in call to %s", p.peek().Line, p.peek().Column, funcName)
	}
	p.advance()
	return &CallNode{FuncName: funcName, Args: args, Line: firstTok.Line, Column: firstTok.Column}, nil
}

func (p *Parser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) advance() Token {
	tok := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return tok
}
