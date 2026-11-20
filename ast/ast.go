package ast

// type Node interface{ Pos() (int, int) }
type Node interface{}

// --- Выражения ---
type (
	NumLit struct {
		Val       float64
		Line, Col int
	}
	StrLit struct {
		Val       string
		Line, Col int
	}
	BoolLit struct {
		Val       bool
		Line, Col int
	}
	NoneLit struct{ Line, Col int }
	Ident   struct {
		Name      string
		Line, Col int
	}

	BinOp struct {
		Op          string
		Left, Right Node
		Line, Col   int
	}
	UnOp struct {
		Op        string // "-", "not"
		X         Node
		Line, Col int
	}
	BoolOp struct { // and/or — короткое замыкание!
		Op          string
		Left, Right Node
	}
	Compare struct {
		Op          string // "==", "<"...
		Left, Right Node
	}
	Call struct {
		Fn        Node
		Args      []Node
		Line, Col int
	}
	Index struct {
		X         Node
		Idx       Node
		Line, Col int
	}
	Slice struct {
		X      Node
		Lo, Hi Node // могут быть nil
	}
	ListLit struct {
		Elems     []Node
		Line, Col int
	}
	DictLit struct {
		Keys, Vals []Node
		Line, Col  int
	}
)

// --- Инструкции ---
type (
	Assign struct {
		Targets []Node // для a = b = 1
		Value   Node
	}
	AugAssign struct { // x += 1
		Target Node
		Op     string
		Value  Node
	}
	ExprStmt struct{ X Node }

	If struct {
		Cond Node
		Then []Node
		Else []Node // elif складываем сюда
	}
	While struct {
		Cond Node
		Body []Node
	}
	FuncDef struct {
		Name   string
		Params []string
		Body   []Node
	}
	Return struct{ X Node }
	Pass   struct{}
)
