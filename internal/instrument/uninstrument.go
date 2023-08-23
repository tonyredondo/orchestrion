// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package instrument

import (
	"bytes"
	"fmt"
	"go/token"
	"io"
	"strings"

	"github.com/datadog/orchestrion/internal/config"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/dave/dst/decorator/resolver/goast"
)

// unwrappers contains the list of helpers responsible for
// removing wrapping-based instrumentation.
var unwrappers = []func(n dst.Node) bool{
	unwrapClient,
	unwrapHandlerExpr,
	unwrapHandlerAssign,
	unwrapSqlExpr,
	unwrapSqlAssign,
	unwrapSqlReturn,
	unwrapGRPC,
}

// removers contains the list of helpers responsible for
// removing instrumentation that adds code.
var removers = []func(stmt dst.Stmt) bool{
	removeGin,
	removeEchoV4,
	removeChiV5,
}

func UninstrumentFile(name string, r io.Reader, conf config.Config) (io.Reader, error) {
	fset := token.NewFileSet()
	resolver := newResolver()
	d := decorator.NewDecoratorWithImports(fset, name, goast.WithResolver(resolver))
	f, err := d.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("error parsing content in %s: %w", name, err)
	}

	for _, decl := range f.Decls {
		if decl, ok := decl.(*dst.FuncDecl); ok {
			decl.Body.List = removeStartEndWrap(decl.Body.List)
			decl.Body.List = removeStartEndInstrument(decl.Body.List)
			// recurse for function literals
			for _, stmt := range decl.Body.List {
				switch stmt := stmt.(type) {
				case *dst.AssignStmt:
					for _, expr := range stmt.Rhs {
						if compLit, ok := expr.(*dst.CompositeLit); ok {
							for _, v := range compLit.Elts {
								if kv, ok := v.(*dst.KeyValueExpr); ok {
									if funLit, ok := kv.Value.(*dst.FuncLit); ok {
										funLit.Body.List = removeStartEndWrap(funLit.Body.List)
										funLit.Body.List = removeStartEndInstrument(funLit.Body.List)
									}
								}
							}
						}
						if funLit, ok := expr.(*dst.FuncLit); ok {
							funLit.Body.List = removeStartEndWrap(funLit.Body.List)
							funLit.Body.List = removeStartEndInstrument(funLit.Body.List)
						}
					}
				case *dst.ExprStmt:
					if call, ok := stmt.X.(*dst.CallExpr); ok {
						switch funLit := call.Fun.(type) {
						case *dst.FuncLit:
							funLit.Body.List = removeStartEndWrap(funLit.Body.List)
							funLit.Body.List = removeStartEndInstrument(funLit.Body.List)
						}
					}
				}
			}
		}
	}

	res := decorator.NewRestorerWithImports(name, resolver)
	var out bytes.Buffer
	err = res.Fprint(&out, f)
	return &out, err
}

func removeDecl(prefix string, ds dst.Decorations) []string {
	var rds []string
	for i := range ds {
		if strings.HasPrefix(ds[i], prefix) {
			continue
		}
		rds = append(rds, ds[i])
	}
	return rds
}

func removeDecoration(deco string, s dst.Stmt) {
	s.Decorations().Start.Replace(removeDecl(deco, s.Decorations().Start)...)
	s.Decorations().End.Replace(removeDecl(deco, s.Decorations().End)...)
}

func removeStartEndWrap(list []dst.Stmt) []dst.Stmt {
	unwrap := func(l []dst.Stmt) {
		for _, s := range l {
			for _, unwrap := range unwrappers {
				dst.Inspect(s, unwrap)
			}
		}
	}

	remove := func(l []dst.Stmt, left, right int) []dst.Stmt {
		for i := left; i < right; i++ {
			for _, rm := range removers {
				if rm(l[i]) {
					l = append(l[:i], l[i+1:]...)
					break
				}
			}
		}
		return l
	}

	for i, stmt := range list {
		if hasLabel(dd_startwrap, stmt.Decorations().Start.All()) {
			stmt.Decorations().Start.Replace(
				removeDecl(dd_startwrap, stmt.Decorations().Start)...)
			if hasLabel(dd_endwrap, stmt.Decorations().End.All()) {
				// dd:endwrap is at the end decorations of the same line as //dd:startwrap.
				// We only need to unwrap() this one line.
				stmt.Decorations().End.Replace(
					removeDecl(dd_endwrap, stmt.Decorations().End)...)
				unwrap(list[i : i+1])
				list = remove(list, i, i+1)
			} else {
				// search for dd:endwrap and then unwrap all the lines between
				// dd:startwrap and dd:endwrap
				for j, stmt := range list[i:] {
					if hasLabel(dd_endwrap, stmt.Decorations().Start.All()) {
						stmt.Decorations().Start.Replace(
							removeDecl(dd_endwrap, stmt.Decorations().Start)...)
						unwrap(list[i : i+j])
						list = remove(list, i, i+j)
					}
				}
			}
		}
	}
	return list
}

func removeStartEndInstrument(list []dst.Stmt) []dst.Stmt {
	var start, end int
	for i, stmt := range list {
		if hasLabel(dd_startinstrument, stmt.Decorations().Start.All()) {
			start = i
		}
		if hasLabel(dd_endinstrument, stmt.Decorations().Start.All()) {
			end = i
			removeDecoration(dd_endinstrument, list[end])
			list = append(list[:start], list[end:]...)
			// We found one. There may be others, so recurse.
			// We can make this more efficient...
			return removeStartEndInstrument(list)
		}
		if hasLabel(dd_endinstrument, stmt.Decorations().End.All()) {
			list = list[:start]
			// We found one. There may be others, so recurse.
			// We can make this more efficient...
			return removeStartEndInstrument(list)
		}
		if hasLabel(dd_instrumented, stmt.Decorations().Start.All()) {
			removeDecoration(dd_instrumented, stmt)
		}
	}
	return list
}

func removeUseMiddleware(stmt dst.Stmt, name string) bool {
	es, ok := stmt.(*dst.ExprStmt)
	if !ok {
		return false
	}
	f, ok := es.X.(*dst.CallExpr)
	if !ok {
		return false
	}
	selexpr, ok := f.Fun.(*dst.SelectorExpr)
	if !ok {
		return false
	}
	if selexpr.Sel.Name != "Use" {
		return false
	}
	if len(f.Args) != 1 {
		return false
	}
	fun, ok := funcIdent(f.Args[0])
	if !ok {
		return false
	}
	return fun.Name == name && fun.Path == "github.com/datadog/orchestrion/instrument"
}
