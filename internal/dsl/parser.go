package dsl

import (
	"fmt"
	"io"
	"strings"

	"bu1ld/internal/build"
	buildplugin "bu1ld/internal/plugin"
	"bu1ld/internal/plugins/golang"
)

type Parser struct {
	lexer    *Lexer
	cur      Token
	peek     Token
	registry *buildplugin.Registry
}

func NewParser() *Parser {
	registry, err := buildplugin.NewRegistry(buildplugin.LoadOptions{}, golang.New())
	if err != nil {
		panic(err)
	}
	return NewParserWithRegistry(registry)
}

func NewParserWithRegistry(registry *buildplugin.Registry) *Parser {
	return &Parser{registry: registry}
}

func (p *Parser) Schemas() ([]buildplugin.Metadata, error) {
	return p.registry.Schemas()
}

func (p *Parser) Parse(reader io.Reader) (build.Project, error) {
	return p.ParseWithOptions(reader, buildplugin.LoadOptions{})
}

func (p *Parser) ParseWithOptions(reader io.Reader, options buildplugin.LoadOptions) (build.Project, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return build.Project{}, err
	}

	file, err := p.ParseFile(string(data))
	if err != nil {
		return build.Project{}, err
	}
	return EvaluateWithRegistry(file, p.registry.CloneWithOptions(options))
}

func (p *Parser) ParseFile(source string) (*File, error) {
	p.lexer = NewLexer(source)
	p.next()
	p.next()

	file := &File{}
	for p.cur.Type != TokenEOF {
		statement, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		file.Statements = append(file.Statements, statement)
	}
	return file, nil
}

func (p *Parser) parseStatement() (Statement, error) {
	if p.cur.Type != TokenIdent {
		return nil, p.errorf(p.cur, "expected block or rule name, got %s", p.cur.Type)
	}

	if p.cur.Literal == "import" {
		return p.parseImport()
	}
	if p.peek.Type == TokenLParen {
		return p.parseRule()
	}
	return p.parseBlock()
}

func (p *Parser) parseImport() (*ImportNode, error) {
	pos := p.cur.Position
	p.next()
	if p.cur.Type != TokenString {
		return nil, p.errorf(p.cur, "expected import path string, got %s", p.cur.Type)
	}
	path := p.cur.Literal
	p.next()
	return &ImportNode{
		Path: path,
		Pos:  pos,
	}, nil
}

func (p *Parser) parseBlock() (*BlockNode, error) {
	kind := p.cur.Literal
	pos := p.cur.Position
	p.next()

	var name Expr
	if p.cur.Type != TokenLBrace {
		var err error
		name, err = p.parseExpr()
		if err != nil {
			return nil, err
		}
	}
	assignments, actions, err := p.parseBodyBlock(kind)
	if err != nil {
		return nil, err
	}

	return &BlockNode{
		Kind:        kind,
		Name:        name,
		Assignments: assignments,
		Actions:     actions,
		Pos:         pos,
	}, nil
}

func (p *Parser) parseRule() (*RuleNode, error) {
	expr, err := p.parseCall()
	if err != nil {
		return nil, err
	}
	call, ok := expr.(*CallExpr)
	if !ok {
		return nil, p.errorf(p.cur, "expected rule call")
	}
	assignments, _, err := p.parseBodyBlock(call.Name)
	if err != nil {
		return nil, err
	}

	return &RuleNode{
		Call:        call,
		Assignments: assignments,
		Pos:         call.Position(),
	}, nil
}

func (p *Parser) parseBodyBlock(name string) ([]*AssignmentNode, []*ActionNode, error) {
	if err := p.expect(TokenLBrace); err != nil {
		return nil, nil, err
	}

	assignments := []*AssignmentNode{}
	actions := []*ActionNode{}
	for p.cur.Type != TokenRBrace {
		if p.cur.Type == TokenEOF {
			return nil, nil, p.errorf(p.cur, "unterminated block %q", name)
		}
		if p.cur.Type == TokenIdent && p.cur.Literal == "run" && p.peek.Type == TokenLBrace {
			if name != "task" {
				return nil, nil, p.errorf(p.cur, "run block is only supported in task blocks")
			}
			parsed, err := p.parseRunBlock()
			if err != nil {
				return nil, nil, err
			}
			actions = append(actions, parsed...)
			continue
		}
		assignment, err := p.parseAssignment()
		if err != nil {
			return nil, nil, err
		}
		assignments = append(assignments, assignment)
	}
	p.next()
	return assignments, actions, nil
}

func (p *Parser) parseRunBlock() ([]*ActionNode, error) {
	p.next()
	if err := p.expect(TokenLBrace); err != nil {
		return nil, err
	}

	actions := []*ActionNode{}
	for p.cur.Type != TokenRBrace {
		if p.cur.Type == TokenEOF {
			return nil, p.errorf(p.cur, "unterminated run block")
		}
		if p.cur.Type != TokenIdent || p.peek.Type != TokenLParen {
			return nil, p.errorf(p.cur, "expected action call, got %s", p.cur.Type)
		}
		expr, err := p.parseCall()
		if err != nil {
			return nil, err
		}
		call, ok := expr.(*CallExpr)
		if !ok {
			return nil, p.errorf(p.cur, "expected action call")
		}
		actions = append(actions, &ActionNode{
			Call: call,
			Pos:  call.Position(),
		})
	}
	p.next()
	return actions, nil
}

func (p *Parser) parseAssignment() (*AssignmentNode, error) {
	if p.cur.Type != TokenIdent {
		return nil, p.errorf(p.cur, "expected assignment name, got %s", p.cur.Type)
	}
	name := p.cur.Literal
	pos := p.cur.Position
	p.next()

	if err := p.expect(TokenAssign); err != nil {
		return nil, err
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, err
	}

	return &AssignmentNode{
		Name:  name,
		Value: value,
		Pos:   pos,
	}, nil
}

func (p *Parser) parseExpr() (Expr, error) {
	switch p.cur.Type {
	case TokenString:
		expr := &StringExpr{Value: p.cur.Literal, Pos: p.cur.Position}
		p.next()
		return expr, nil
	case TokenExpr:
		expr := &ScriptExpr{Source: p.cur.Literal, Pos: p.cur.Position}
		p.next()
		return expr, nil
	case TokenIdent:
		if p.peek.Type == TokenLParen {
			return p.parseCall()
		}
		expr := &IdentExpr{Name: p.cur.Literal, Pos: p.cur.Position}
		p.next()
		return expr, nil
	case TokenLBrack:
		return p.parseArray()
	case TokenLBrace:
		return p.parseObject()
	default:
		return nil, p.errorf(p.cur, "expected expression, got %s", p.cur.Type)
	}
}

func (p *Parser) parseArray() (Expr, error) {
	pos := p.cur.Position
	p.next()

	expr := &ArrayExpr{Pos: pos}
	for p.cur.Type != TokenRBrack {
		if p.cur.Type == TokenEOF {
			return nil, p.errorf(p.cur, "unterminated array")
		}
		element, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		expr.Elements = append(expr.Elements, element)

		if p.cur.Type == TokenComma {
			p.next()
			continue
		}
		if p.cur.Type != TokenRBrack {
			return nil, p.errorf(p.cur, "expected comma or ], got %s", p.cur.Type)
		}
	}
	p.next()
	return expr, nil
}

func (p *Parser) parseObject() (Expr, error) {
	pos := p.cur.Position
	p.next()

	expr := &ObjectExpr{Pos: pos}
	for p.cur.Type != TokenRBrace {
		if p.cur.Type == TokenEOF {
			return nil, p.errorf(p.cur, "unterminated object")
		}
		if p.cur.Type != TokenIdent {
			return nil, p.errorf(p.cur, "expected object key, got %s", p.cur.Type)
		}
		key := p.cur.Literal
		keyPos := p.cur.Position
		p.next()
		if err := p.expect(TokenAssign); err != nil {
			return nil, err
		}
		value, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		expr.Entries = append(expr.Entries, &ObjectEntry{
			Key:   key,
			Value: value,
			Pos:   keyPos,
		})

		if p.cur.Type == TokenComma {
			p.next()
			continue
		}
		if p.cur.Type != TokenRBrace {
			return nil, p.errorf(p.cur, "expected comma or }, got %s", p.cur.Type)
		}
	}
	p.next()
	return expr, nil
}

func (p *Parser) parseCall() (Expr, error) {
	name := p.cur.Literal
	pos := p.cur.Position
	p.next()
	if err := p.expect(TokenLParen); err != nil {
		return nil, err
	}

	call := &CallExpr{Name: name, Pos: pos}
	for p.cur.Type != TokenRParen {
		if p.cur.Type == TokenEOF {
			return nil, p.errorf(p.cur, "unterminated call %s", name)
		}
		arg, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		call.Args = append(call.Args, arg)

		if p.cur.Type == TokenComma {
			p.next()
			continue
		}
		if p.cur.Type != TokenRParen {
			return nil, p.errorf(p.cur, "expected comma or ), got %s", p.cur.Type)
		}
	}
	p.next()
	return call, nil
}

func (p *Parser) expect(tokenType TokenType) error {
	if p.cur.Type != tokenType {
		return p.errorf(p.cur, "expected %s, got %s", tokenType, p.cur.Type)
	}
	p.next()
	return nil
}

func (p *Parser) expectIdent(value string) error {
	if p.cur.Type != TokenIdent || p.cur.Literal != value {
		return p.errorf(p.cur, "expected %q, got %q", value, p.cur.Literal)
	}
	p.next()
	return nil
}

func (p *Parser) next() {
	p.cur = p.peek
	p.peek = p.lexer.NextToken()
}

func (p *Parser) errorf(token Token, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	literal := strings.TrimSpace(token.Literal)
	if literal != "" {
		message += fmt.Sprintf(" near %q", literal)
	}
	return fmt.Errorf("dsl:%d:%d: %s", token.Position.Line, token.Position.Column, message)
}
