package dsl

type TokenType string

const (
	TokenIllegal TokenType = "ILLEGAL"
	TokenEOF     TokenType = "EOF"

	TokenIdent  TokenType = "IDENT"
	TokenString TokenType = "STRING"

	TokenAssign TokenType = "="
	TokenComma  TokenType = ","
	TokenLBrace TokenType = "{"
	TokenRBrace TokenType = "}"
	TokenLBrack TokenType = "["
	TokenRBrack TokenType = "]"
	TokenLParen TokenType = "("
	TokenRParen TokenType = ")"
)

type Position struct {
	Line   int
	Column int
}

type Token struct {
	Type     TokenType
	Literal  string
	Position Position
}
