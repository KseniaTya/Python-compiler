package interpreter

import (
	"fmt"
	"pyCompiler/ast"
	"pyCompiler/pyval"
	"strings"
)

type Interp struct {
	globals *pyval.Env
}

type controlFlow int

const (
	flowNormal controlFlow = iota
	flowReturn
	flowBreak
	flowContinue
)

type execResult struct {
	flow controlFlow
	val  pyval.Value
}

func New() *Interp {
	i := &Interp{
		globals: pyval.NewEnv(nil),
	}
	i.installBuiltins()
	return i
}

func (i *Interp) Run(prog []ast.Node) error {
	// Перехватываем паники интерпретатора (NameError, TypeError и т.п.),
	// чтобы они не роняли всю программу Go-трейсом.
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("RuntimeError:", r)
		}
	}()

	i.execBlock(prog, i.globals)
	return nil
}

func (i *Interp) execBlock(stmts []ast.Node, env *pyval.Env) execResult {
	for _, s := range stmts {
		r := i.execStmt(s, env)
		if r.flow != flowNormal {
			return r
		}
	}
	return execResult{}
}

func (i *Interp) execStmt(s ast.Node, env *pyval.Env) execResult {
	switch n := s.(type) {
	case *ast.Assign:
		v := i.eval(n.Value, env)
		for _, tgt := range n.Targets {
			i.assign(tgt, v, env)
		}
	case *ast.AugAssign:
		cur := i.eval(n.Target, env)
		rhs := i.eval(n.Value, env)
		res := i.binOp(n.Op, cur, rhs)
		i.assign(n.Target, res, env)
	case *ast.ExprStmt:
		i.eval(n.X, env)
	case *ast.If:
		if pyval.Truthy(i.eval(n.Cond, env)) {
			return i.execBlock(n.Then, env)
		}
		return i.execBlock(n.Else, env)
	case *ast.While:
		for pyval.Truthy(i.eval(n.Cond, env)) {
			r := i.execBlock(n.Body, env)
			if r.flow == flowBreak {
				break
			}
			if r.flow == flowReturn {
				return r
			}
		}
	case *ast.FuncDef:
		fn := &pyval.Func{Name: n.Name, Params: n.Params, Body: n.Body, Env: env}
		env.Declare(n.Name, fn)
	case *ast.Return:
		var v pyval.Value = pyval.None{}
		if n.X != nil {
			v = i.eval(n.X, env)
		}
		return execResult{flow: flowReturn, val: v}
	case *ast.Pass:
	}
	return execResult{}
}

func (i *Interp) assign(target ast.Node, v pyval.Value, env *pyval.Env) {
	switch t := target.(type) {
	case *ast.Ident:
		env.Set(t.Name, v)
	case *ast.Index:
		container := i.eval(t.X, env)
		idx := i.eval(t.Idx, env)
		switch c := container.(type) {
		case *pyval.List:
			n, ok := idx.(pyval.Int)
			if !ok {
				panic("list index must be int")
			}
			// отрицательный индекс
			j := int(n)
			if j < 0 {
				j += len(c.Items)
			}
			if j < 0 || j >= len(c.Items) {
				panic("IndexError")
			}
			c.Items[j] = v
		case *pyval.Dict:
			if _, exists := c.Values[idx]; !exists {
				c.Keys = append(c.Keys, idx)
			}
			c.Values[idx] = v
		}
	case *ast.Slice:
		// a[1:3] = [...] — упрощённо
	}
}
func (i *Interp) eval(e ast.Node, env *pyval.Env) pyval.Value {
	switch n := e.(type) {
	case *ast.NumLit:
		if n.Val == float64(int64(n.Val)) {
			return pyval.Int(int64(n.Val))
		}
		return pyval.Float(n.Val)
	case *ast.StrLit:
		return pyval.Str(n.Val)
	case *ast.BoolLit:
		return pyval.Bool(n.Val)
	case *ast.NoneLit:
		return pyval.None{}
	case *ast.Ident:
		return env.Get(n.Name)

	case *ast.BinOp:
		l := i.eval(n.Left, env)
		r := i.eval(n.Right, env)
		return i.binOp(n.Op, l, r)

	case *ast.UnOp:
		x := i.eval(n.X, env)
		switch n.Op {
		case "-":
			if v, ok := x.(pyval.Int); ok {
				return -v
			}
			if v, ok := x.(pyval.Float); ok {
				return -v
			}
		case "not":
			return pyval.Bool(!pyval.Truthy(x))
		}
		panic("bad unary op")

	case *ast.Compare:
		l := i.eval(n.Left, env)
		r := i.eval(n.Right, env)
		return pyval.Bool(i.compare(n.Op, l, r))

	case *ast.BoolOp:
		l := i.eval(n.Left, env)
		if n.Op == "and" {
			if !pyval.Truthy(l) {
				return l
			}
			return i.eval(n.Right, env)
		}
		// or
		if pyval.Truthy(l) {
			return l
		}
		return i.eval(n.Right, env)

	case *ast.Call:
		fn := i.eval(n.Fn, env)
		var args []pyval.Value
		for _, a := range n.Args {
			args = append(args, i.eval(a, env))
		}
		return i.callFunction(fn, args)

	case *ast.ListLit:
		lst := &pyval.List{Items: make([]pyval.Value, 0, len(n.Elems))}
		for _, e := range n.Elems {
			lst.Items = append(lst.Items, i.eval(e, env))
		}
		return lst

	case *ast.DictLit:
		d := &pyval.Dict{Values: map[pyval.Value]pyval.Value{}}
		for k := 0; k < len(n.Keys); k++ {
			key := i.eval(n.Keys[k], env)
			val := i.eval(n.Vals[k], env)
			d.Keys = append(d.Keys, key)
			d.Values[key] = val
		}
		return d

	case *ast.Index:
		c := i.eval(n.X, env)
		idx := i.eval(n.Idx, env)
		return i.index(c, idx)

	case *ast.Slice:
		c := i.eval(n.X, env)
		var lo, hi int
		// Вычисляем нижнюю границу (если она есть)
		if n.Lo != nil {
			loVal := i.eval(n.Lo, env)
			loInt, ok := loVal.(pyval.Int)
			if !ok {
				panic("TypeError: slice indices must be integers")
			}
			lo = int(loInt)
		}

		// Вычисляем верхнюю границу (если она есть)
		if n.Hi != nil {
			hiVal := i.eval(n.Hi, env)
			hiInt, ok := hiVal.(pyval.Int)
			if !ok {
				panic("TypeError: slice indices must be integers")
			}
			hi = int(hiInt)
		} else {
			// Если верхней границы нет (например lst[1:]), берем длину
			switch x := c.(type) {
			case *pyval.List:
				hi = len(x.Items)
			case pyval.Str:
				hi = len([]rune(string(x)))
			default:
				panic("TypeError: object is not sliceable")
			}
		}
		return sliceValue(c, lo, hi)
	}
	panic("unhandled expression")
}

func (i *Interp) binOp(op string, l, r pyval.Value) pyval.Value {
	// int + int -> int;  int + float -> float;  str + str -> str; list + list -> list
	switch op {
	case "+":
		if a, ok := l.(pyval.Int); ok {
			if b, ok := r.(pyval.Int); ok {
				return a + b
			}
			if b, ok := r.(pyval.Float); ok {
				return pyval.Float(float64(a) + float64(b))
			}
		}
		if a, ok := l.(pyval.Float); ok {
			if b, ok := r.(pyval.Int); ok {
				return pyval.Float(float64(a) + float64(b))
			}
			if b, ok := r.(pyval.Float); ok {
				return a + b
			}
		}
		if a, ok := l.(pyval.Str); ok {
			if b, ok := r.(pyval.Str); ok {
				return a + b
			}
		}
		if a, ok := l.(*pyval.List); ok {
			if b, ok := r.(*pyval.List); ok {
				out := make([]pyval.Value, 0, len(a.Items)+len(b.Items))
				out = append(out, a.Items...)
				out = append(out, b.Items...)
				return &pyval.List{Items: out}
			}
		}
		panic("unsupported operand types for +")
	case "-":
		if a, ok := l.(pyval.Int); ok {
			if b, ok := r.(pyval.Int); ok {
				return a - b
			}
			if b, ok := r.(pyval.Float); ok {
				return pyval.Float(float64(a) - float64(b))
			}
		}
		if a, ok := l.(pyval.Float); ok {
			if b, ok := r.(pyval.Int); ok {
				return pyval.Float(float64(a) - float64(b))
			}
			if b, ok := r.(pyval.Float); ok {
				return a - b
			}
		}
		panic("unsupported operand types for +")
	case "*":
		// str * int -> повторение, list * int
	case "/":
		// всегда float в Python 3
	case "//":
		// floor division
	case "%", "**":
		// ...
	}
	panic("unknown op " + op)
}

func (i *Interp) callFunction(fn pyval.Value, args []pyval.Value) pyval.Value {
	switch f := fn.(type) {
	case *pyval.Builtin:
		return f.Fn(args)
	case *pyval.Func:
		if len(args) != len(f.Params) {
			panic(fmt.Sprintf("TypeError: %s() takes %d args (%d given)",
				f.Name, len(f.Params), len(args)))
		}
		callEnv := pyval.NewEnv(f.Env) // лексическая область!
		for k, p := range f.Params {
			callEnv.Set(p, args[k])
		}
		r := i.execBlock(f.Body, callEnv)
		if r.flow == flowReturn {
			return r.val
		}
		return pyval.None{}
	}
	panic("object is not callable")
}

func (i *Interp) installBuiltins() {
	i.globals.Declare("print", &pyval.Builtin{
		Fn: func(args []pyval.Value) pyval.Value {
			parts := make([]string, len(args))
			for k, a := range args {
				parts[k] = pyval.StrRepr(a) // str() без кавычек для строк
			}
			fmt.Println(strings.Join(parts, " "))
			return pyval.None{}
		},
	})
	i.globals.Declare("len", &pyval.Builtin{
		Fn: func(args []pyval.Value) pyval.Value {
			switch x := args[0].(type) {
			case pyval.Str:
				return pyval.Int(len(x))
			case *pyval.List:
				return pyval.Int(len(x.Items))
			case *pyval.Dict:
				return pyval.Int(len(x.Keys))
			}
			panic("TypeError: object has no len()")
		},
	})
	i.globals.Declare("range", &pyval.Builtin{
		Fn: func(args []pyval.Value) pyval.Value {
			// range(n) | range(a,b) | range(a,b,step)
			var start, stop, step int64 = 0, 0, 1
			switch len(args) {
			case 1:
				stop = int64(args[0].(pyval.Int))
			case 2:
				start = int64(args[0].(pyval.Int))
				stop = int64(args[1].(pyval.Int))
			case 3:
				start = int64(args[0].(pyval.Int))
				stop = int64(args[1].(pyval.Int))
				step = int64(args[2].(pyval.Int))
			}
			var items []pyval.Value
			if step > 0 {
				for v := start; v < stop; v += step {
					items = append(items, pyval.Int(v))
				}
			} else {
				for v := start; v > stop; v += step {
					items = append(items, pyval.Int(v))
				}
			}
			return &pyval.List{Items: items}
		},
	})
}

// ---------- Сравнения ----------

func (i *Interp) compare(op string, l, r pyval.Value) bool {
	// Числа (int, float, bool)
	lf, lok := toFloat(l)
	rf, rok := toFloat(r)
	if lok && rok {
		switch op {
		case "==":
			return lf == rf
		case "!=":
			return lf != rf
		case "<":
			return lf < rf
		case "<=":
			return lf <= rf
		case ">":
			return lf > rf
		case ">=":
			return lf >= rf
		}
	}

	// Строки
	ls, lsok := l.(pyval.Str)
	rs, rsok := r.(pyval.Str)
	if lsok && rsok {
		switch op {
		case "==":
			return ls == rs
		case "!=":
			return ls != rs
		case "<":
			return ls < rs
		case "<=":
			return ls <= rs
		case ">":
			return ls > rs
		case ">=":
			return ls >= rs
		}
	}

	// bool / None — только == и !=
	switch op {
	case "==":
		return l == r
	case "!=":
		return l != r
	}
	panic("TypeError: unsupported comparison")
}

func toFloat(v pyval.Value) (float64, bool) {
	switch x := v.(type) {
	case pyval.Int:
		return float64(x), true
	case pyval.Float:
		return float64(x), true
	case pyval.Bool:
		if x {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

// ---------- Индексация ----------

func (i *Interp) index(c, idx pyval.Value) pyval.Value {
	switch x := c.(type) {
	case *pyval.List:
		n, ok := idx.(pyval.Int)
		if !ok {
			panic("TypeError: list indices must be integers")
		}
		j := int(n)
		if j < 0 {
			j += len(x.Items)
		}
		if j < 0 || j >= len(x.Items) {
			panic("IndexError: list index out of range")
		}
		return x.Items[j]

	case *pyval.Dict:
		v, ok := x.Values[idx]
		if !ok {
			panic("KeyError: " + pyval.Repr(idx))
		}
		return v

	case pyval.Str:
		n, ok := idx.(pyval.Int)
		if !ok {
			panic("TypeError: string indices must be integers")
		}
		s := []rune(string(x))
		j := int(n)
		if j < 0 {
			j += len(s)
		}
		if j < 0 || j >= len(s) {
			panic("IndexError: string index out of range")
		}
		return pyval.Str(string(s[j]))
	}
	panic("TypeError: object is not subscriptable")
}

// ---------- Срезы ----------

func sliceValue(v pyval.Value, lo, hi int) pyval.Value {
	switch x := v.(type) {
	case *pyval.List:
		n := len(x.Items)
		lo, hi = clamp(lo, hi, n)
		return &pyval.List{Items: x.Items[lo:hi]}
	case pyval.Str:
		s := []rune(string(x))
		n := len(s)
		lo, hi = clamp(lo, hi, n)
		return pyval.Str(string(s[lo:hi]))
	}
	panic("TypeError: object is not sliceable")
}

func clamp(lo, hi, n int) (int, int) {
	if lo < 0 {
		lo += n
	}
	if hi < 0 {
		hi += n
	}
	if lo < 0 {
		lo = 0
	}
	if hi > n {
		hi = n
	}
	if hi < lo {
		hi = lo
	}
	return lo, hi
}
