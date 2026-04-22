package dsl

import (
	"fmt"
	"strconv"
	"unicode"
)

type Lexer struct {
	input      []rune
	position   int
	readPos    int
	ch         rune
	line       int
	column     int
	nextColumn int
}

func NewLexer(input string) *Lexer {
	l := &Lexer{
		input:      []rune(input),
		line:       1,
		nextColumn: 1,
	}
	l.readChar()
	return l
}

func (l *Lexer) NextToken() Token {
	l.skipWhitespaceAndComments()

	pos := l.currentPosition()
	switch l.ch {
	case 0:
		return Token{Type: TokenEOF, Position: pos}
	case '=':
		return l.single(TokenAssign)
	case ',':
		return l.single(TokenComma)
	case '{':
		return l.single(TokenLBrace)
	case '}':
		return l.single(TokenRBrace)
	case '[':
		return l.single(TokenLBrack)
	case ']':
		return l.single(TokenRBrack)
	case '(':
		return l.single(TokenLParen)
	case ')':
		return l.single(TokenRParen)
	case '"', '`':
		return l.readStringToken()
	case '$':
		if l.peekChar() == '(' || l.peekChar() == '{' {
			return l.readScriptExprToken()
		}
		literal := string(l.ch)
		l.readChar()
		return Token{Type: TokenIllegal, Literal: literal, Position: pos}
	default:
		if isIdentStart(l.ch) {
			literal := l.readIdent()
			return Token{Type: TokenIdent, Literal: literal, Position: pos}
		}
		literal := string(l.ch)
		l.readChar()
		return Token{Type: TokenIllegal, Literal: literal, Position: pos}
	}
}

func (l *Lexer) single(tokenType TokenType) Token {
	token := Token{Type: tokenType, Literal: string(l.ch), Position: l.currentPosition()}
	l.readChar()
	return token
}

func (l *Lexer) readStringToken() Token {
	pos := l.currentPosition()
	quote := l.ch
	raw := []rune{quote}
	l.readChar()

	for l.ch != 0 && l.ch != quote {
		if quote == '"' && l.ch == '\\' {
			raw = append(raw, l.ch)
			l.readChar()
			if l.ch == 0 {
				break
			}
		}
		raw = append(raw, l.ch)
		l.readChar()
	}

	if l.ch != quote {
		return Token{Type: TokenIllegal, Literal: "unterminated string", Position: pos}
	}
	raw = append(raw, quote)
	l.readChar()

	value, err := strconv.Unquote(string(raw))
	if err != nil {
		return Token{Type: TokenIllegal, Literal: fmt.Sprintf("invalid string: %v", err), Position: pos}
	}
	return Token{Type: TokenString, Literal: value, Position: pos}
}

func (l *Lexer) readScriptExprToken() Token {
	pos := l.currentPosition()
	l.readChar()

	open := l.ch
	close := ')'
	if open == '{' {
		close = '}'
	}
	l.readChar()

	depth := 1
	expr := []rune{}
	for l.ch != 0 {
		if l.ch == '"' || l.ch == '\'' || l.ch == '`' {
			quoted, ok := l.readScriptQuoted()
			expr = append(expr, quoted...)
			if !ok {
				return Token{Type: TokenIllegal, Literal: "unterminated expression string", Position: pos}
			}
			continue
		}

		if l.ch == open {
			depth++
		}
		if l.ch == close {
			depth--
			if depth == 0 {
				l.readChar()
				return Token{Type: TokenExpr, Literal: string(expr), Position: pos}
			}
		}
		expr = append(expr, l.ch)
		l.readChar()
	}

	return Token{Type: TokenIllegal, Literal: "unterminated expression", Position: pos}
}

func (l *Lexer) readScriptQuoted() ([]rune, bool) {
	quote := l.ch
	value := []rune{quote}
	l.readChar()
	for l.ch != 0 {
		value = append(value, l.ch)
		if l.ch == '\\' && quote != '`' {
			l.readChar()
			if l.ch == 0 {
				return value, false
			}
			value = append(value, l.ch)
			l.readChar()
			continue
		}
		if l.ch == quote {
			l.readChar()
			return value, true
		}
		l.readChar()
	}
	return value, false
}

func (l *Lexer) readIdent() string {
	start := l.position
	for isIdentPart(l.ch) {
		l.readChar()
	}
	return string(l.input[start:l.position])
}

func (l *Lexer) skipWhitespaceAndComments() {
	for {
		for unicode.IsSpace(l.ch) {
			l.readChar()
		}

		switch {
		case l.ch == '#':
			l.skipLineComment()
			continue
		case l.ch == '/' && l.peekChar() == '/':
			l.skipLineComment()
			continue
		case l.ch == '/' && l.peekChar() == '*':
			l.readChar()
			l.readChar()
			l.skipBlockComment()
			continue
		default:
			return
		}
	}
}

func (l *Lexer) skipLineComment() {
	for l.ch != 0 && l.ch != '\n' {
		l.readChar()
	}
}

func (l *Lexer) skipBlockComment() {
	for l.ch != 0 {
		if l.ch == '*' && l.peekChar() == '/' {
			l.readChar()
			l.readChar()
			return
		}
		l.readChar()
	}
}

func (l *Lexer) readChar() {
	if l.readPos >= len(l.input) {
		l.position = l.readPos
		l.ch = 0
		l.column = l.nextColumn
		return
	}

	l.position = l.readPos
	l.ch = l.input[l.readPos]
	l.column = l.nextColumn
	l.readPos++

	if l.ch == '\n' {
		l.line++
		l.nextColumn = 1
	} else {
		l.nextColumn++
	}
}

func (l *Lexer) peekChar() rune {
	if l.readPos >= len(l.input) {
		return 0
	}
	return l.input[l.readPos]
}

func (l *Lexer) currentPosition() Position {
	return Position{Line: l.line, Column: l.column}
}

func isIdentStart(ch rune) bool {
	return unicode.IsLetter(ch) || ch == '_'
}

func isIdentPart(ch rune) bool {
	return isIdentStart(ch) || unicode.IsDigit(ch) || ch == '-' || ch == '/' || ch == ':' || ch == '*' || ch == '.'
}
