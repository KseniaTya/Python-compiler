package main

import (
	"fmt"
	"os"
	"pyCompiler/interpreter"
	"pyCompiler/lexer"
	"pyCompiler/parser"
)

func main() {
	//src, _ := os.ReadFile(os.Args[1])
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: pyComp <file.py>")
		os.Exit(1)
	}

	src, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "read error:", err)
		os.Exit(1)
	}
	lx := lexer.New(string(src))
	toks := lx.Tokenize()

	p := parser.New(toks)
	prog := p.ParseProgram()

	it := interpreter.New()
	it.Run(prog)
}
