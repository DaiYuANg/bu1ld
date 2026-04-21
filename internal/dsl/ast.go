package dsl

type Node interface {
	Position() Position
}

type File struct {
	Tasks []*TaskNode
}

func (f *File) Position() Position {
	if f == nil || len(f.Tasks) == 0 {
		return Position{}
	}
	return f.Tasks[0].Position()
}

type TaskNode struct {
	Name        Expr
	Assignments []*AssignmentNode
	Pos         Position
}

func (n *TaskNode) Position() Position {
	if n == nil {
		return Position{}
	}
	return n.Pos
}

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
