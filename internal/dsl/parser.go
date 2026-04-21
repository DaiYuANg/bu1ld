package dsl

import (
	"fmt"
	"io"
	"strings"

	"bu1ld/internal/build"
)

type Parser struct {
	lexer *Lexer
	cur   Token
	peek  Token
}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) Parse(reader io.Reader) (build.Project, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return build.Project{}, err
	}

	file, err := p.ParseFile(string(data))
	if err != nil {
		return build.Project{}, err
	}
	return Evaluate(file)
}

func (p *Parser) ParseFile(source string) (*File, error) {
	p.lexer = NewLexer(source)
	p.next()
	p.next()

	file := &File{}
	for p.cur.Type != TokenEOF {
		task, err := p.parseTask()
		if err != nil {
			return nil, err
		}
		file.Tasks = append(file.Tasks, task)
	}
	return file, nil
}

func (p *Parser) parseTask() (*TaskNode, error) {
	if err := p.expectIdent("task"); err != nil {
		return nil, err
	}
	pos := p.cur.Position

	name, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if err := p.expect(TokenLBrace); err != nil {
		return nil, err
	}

	task := &TaskNode{
		Name: name,
		Pos:  pos,
	}
	for p.cur.Type != TokenRBrace {
		if p.cur.Type == TokenEOF {
			return nil, p.errorf(p.cur, "unterminated task")
		}
		assignment, err := p.parseAssignment()
		if err != nil {
			return nil, err
		}
		task.Assignments = append(task.Assignments, assignment)
	}
	p.next()
	return task, nil
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
	case TokenIdent:
		if p.peek.Type == TokenLParen {
			return p.parseCall()
		}
		expr := &IdentExpr{Name: p.cur.Literal, Pos: p.cur.Position}
		p.next()
		return expr, nil
	case TokenLBrack:
		return p.parseArray()
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
