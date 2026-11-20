package pyval

import (
	"fmt"
	"pyCompiler/ast"
	"strconv"
	"strings"
)

type Value interface{ isPyValue() }

type (
	Int   int64
	Float float64
	Str   string
	Bool  bool

	None struct{}

	List struct {
		Items []Value
	}
	Dict struct {
		Keys   []Value         // для сохранения порядка (как в Python 3.7+)
		Values map[Value]Value // ключи должны быть hashable
	}
	Func struct {
		Name   string
		Params []string
		Body   []ast.Node
		Env    *Env // замыкание!
	}
	Builtin struct {
		Name string
		Fn   func(args []Value) Value
	}
)

func (Int) isPyValue()      {}
func (Float) isPyValue()    {}
func (Str) isPyValue()      {}
func (Bool) isPyValue()     {}
func (None) isPyValue()     {}
func (*List) isPyValue()    {}
func (*Dict) isPyValue()    {}
func (*Func) isPyValue()    {}
func (*Builtin) isPyValue() {}

func Truthy(v Value) bool {
	switch x := v.(type) {
	case Bool:
		return bool(x)
	case Int:
		return x != 0
	case Float:
		return x != 0
	case Str:
		return len(x) > 0
	case None:
		return false
	case *List:
		return len(x.Items) > 0
	case *Dict:
		return len(x.Keys) > 0
	}
	return true
}

type Env struct {
	vars   map[string]Value
	parent *Env
}

func NewEnv(parent *Env) *Env {
	return &Env{vars: map[string]Value{}, parent: parent}
}

func (e *Env) Get(name string) Value {
	for cur := e; cur != nil; cur = cur.parent {
		if v, ok := cur.vars[name]; ok {
			return v
		}
	}
	panic(fmt.Sprintf("NameError: name '%s' is not defined", name))
}

func (e *Env) Set(name string, v Value) {
	e.vars[name] = v // присваивание всегда в текущей области
}

// Для рекурсивных функций: объявить до исполнения тела
func (e *Env) Declare(name string, v Value) { e.vars[name] = v }

// Repr — как repr() в Python (строки в кавычках)
func Repr(v Value) string {
	switch x := v.(type) {
	case Int:
		return strconv.FormatInt(int64(x), 10)
	case Float:
		return strconv.FormatFloat(float64(x), 'g', -1, 64)
	case Str:
		return strconv.Quote(string(x))
	case Bool:
		if x {
			return "True"
		}
		return "False"
	case None:
		return "None"
	case *List:
		parts := make([]string, len(x.Items))
		for k, it := range x.Items {
			parts[k] = Repr(it)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *Dict:
		parts := make([]string, 0, len(x.Keys))
		for _, k := range x.Keys {
			parts = append(parts, Repr(k)+": "+Repr(x.Values[k]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case *Func:
		return "<function " + x.Name + ">"
	case *Builtin:
		return "<built-in function " + x.Name + ">"
	}
	return "<?>"
}

// Str — как str() в Python (строки без кавычек)
func StrRepr(v Value) string {
	if s, ok := v.(Str); ok {
		return string(s)
	}
	return Repr(v)
}
