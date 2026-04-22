package dsl

type Node interface {
	Position() Position
}

type File struct {
	Statements []Statement
}

func (f *File) Position() Position {
	if f == nil || len(f.Statements) == 0 {
		return Position{}
	}
	return f.Statements[0].Position()
}

type Statement interface {
	Node
	statementNode()
}

type BlockNode struct {
	Kind        string
	Name        Expr
	Assignments []*AssignmentNode
	Pos         Position
}

func (n *BlockNode) Position() Position {
	if n == nil {
		return Position{}
	}
	return n.Pos
}
func (n *BlockNode) statementNode() {}

type RuleNode struct {
	Call        *CallExpr
	Assignments []*AssignmentNode
	Pos         Position
}

func (n *RuleNode) Position() Position {
	if n == nil {
		return Position{}
	}
	return n.Pos
}
func (n *RuleNode) statementNode() {}

type AssignmentNode struct {
	Name  string
	Value Expr
	Pos   Position
}

func (n *AssignmentNode) Position() Position {
	if n == nil {
		return Position{}
	}
	return n.Pos
}

type Expr interface {
	Node
	exprNode()
}

type StringExpr struct {
	Value string
	Pos   Position
}

func (e *StringExpr) Position() Position { return e.Pos }
func (e *StringExpr) exprNode()          {}

type ScriptExpr struct {
	Source string
	Pos    Position
}

func (e *ScriptExpr) Position() Position { return e.Pos }
func (e *ScriptExpr) exprNode()          {}

type IdentExpr struct {
	Name string
	Pos  Position
}

func (e *IdentExpr) Position() Position { return e.Pos }
func (e *IdentExpr) exprNode()          {}

type ArrayExpr struct {
	Elements []Expr
	Pos      Position
}

func (e *ArrayExpr) Position() Position { return e.Pos }
func (e *ArrayExpr) exprNode()          {}

type CallExpr struct {
	Name string
	Args []Expr
	Pos  Position
}

func (e *CallExpr) Position() Position { return e.Pos }
func (e *CallExpr) exprNode()          {}

type ObjectEntry struct {
	Key   string
	Value Expr
	Pos   Position
}

func (e *ObjectEntry) Position() Position {
	if e == nil {
		return Position{}
	}
	return e.Pos
}

type ObjectExpr struct {
	Entries []*ObjectEntry
	Pos     Position
}

func (e *ObjectExpr) Position() Position { return e.Pos }
func (e *ObjectExpr) exprNode()          {}
