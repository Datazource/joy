package index

import (
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"sort"
	"strings"

	"github.com/matthewmueller/golly/golang/def"
	"github.com/matthewmueller/golly/golang/util"
	"golang.org/x/tools/go/loader"
)

// Index type
type Index struct {
	program *loader.Program
	defs    map[string]def.Definition
	imports map[string]map[string]string
}

// New index
func New(program *loader.Program) *Index {
	return &Index{
		program: program,
		defs:    map[string]def.Definition{},
		imports: map[string]map[string]string{},
	}
}

// AddDefinition adds a definition to the index
func (i *Index) AddDefinition(d def.Definition) {
	i.defs[d.ID()] = d
}

// AddImport fn
func (i *Index) AddImport(parentPath string, alias, depPath string) {
	if i.imports[parentPath] == nil {
		i.imports[parentPath] = map[string]string{}
	}
	i.imports[parentPath][alias] = depPath
}

// GetImports fn
func (i *Index) GetImports(parentPath string) map[string]string {
	deps := i.imports[parentPath]
	if deps == nil {
		return map[string]string{}
	}
	return deps
}

// All gets all definitions from the index
// TODO: remove
func (i *Index) All() map[string]def.Definition {
	return i.defs
}

// Get all definitions from the index
func (i *Index) Get(id string) def.Definition {
	return i.defs[id]
}

// Link is like a symlink to a definition
func (i *Index) Link(alias string, d def.Definition) {
	i.defs[alias] = d
}

// Mains gets all the main functions
func (i *Index) Mains() (mains []def.Definition) {
	var defids []string
	for _, info := range i.program.InitialPackages() {
		p := info.Pkg.Path()
		def := i.defs[p+" main"]
		if def == nil {
			continue
		}
		defids = append(defids, def.ID())
	}

	sort.Strings(defids)
	for _, id := range defids {
		mains = append(mains, i.defs[id])
	}

	return mains
}

// Runtime gets a definition from the runtime
func (i *Index) Runtime(names ...string) (runtimes []def.Definition, err error) {
	var defs []string
	for _, def := range i.defs {
		if !def.FromRuntime() {
			continue
		}

		for _, name := range names {
			if def.Name() == name {
				defs = append(defs, def.ID())
			}
		}
	}

	// sort so we have consistent builds
	sort.Strings(defs)

	for _, def := range defs {
		runtimes = append(runtimes, i.defs[def])
	}

	return runtimes, nil
}

// DefinitionOf fn
// Will return nil when ident is:
// - package name
// - basic type
// - local variable that points to a basic type
// - function parameters & results
func (i *Index) DefinitionOf(packagePath string, n ast.Node) (def.Definition, error) {
	switch t := n.(type) {
	case *ast.SelectorExpr:
		return i.selectorDefinition(packagePath, t)
	case *ast.Ident:
		return i.identDefinition(packagePath, t)
	default:
		id, e := util.GetIdentifier(t)
		if e != nil {
			return nil, e
		} else if id == nil {
			return nil, nil
		}
		return i.identDefinition(packagePath, id)
	}
}

// Finds the definition based on the selector expression
// This is a bit subtle, but we basically do the following:
// 1. Check the rightmost identifier for a definition
//    if we find one and it's not a struct, then return
// 2. If the rightmost identifier is nil or a struct,
//    we find the rightmost parent of the selector
//    expression. If we find that definition, we return
//    it immediately
// 3. If we don't, we return the original rightmost identifier
//    which is either a struct or nil at this point
func (i *Index) selectorDefinition(packagePath string, n *ast.SelectorExpr) (def.Definition, error) {
	sel, e := i.identDefinition(packagePath, n.Sel)
	if e != nil {
		return nil, e
	}

	// prioritize selector definitions for functions
	if sel != nil && sel.Kind() != "STRUCT" {
		return sel, nil
	}

	id, e := util.GetIdentifier(n.X)
	if e != nil {
		return nil, e
	}

	d, e := i.identDefinition(packagePath, id)
	if e != nil {
		return nil, e
	} else if d != nil {
		return d, nil
	}

	return sel, nil
}

func (i *Index) identDefinition(packagePath string, n *ast.Ident) (def.Definition, error) {
	info := i.program.Package(packagePath)
	if info == nil {
		return nil, errors.New("DefinitionOf: no package found")
	}

	// built-in or package name
	obj := info.ObjectOf(n)
	if obj == nil {
		return nil, nil
	}

	// package is nil for types (e.g int)
	// & built-in functions (e.g. append)
	pkg := obj.Pkg()
	if pkg == nil {
		return nil, nil
	}

	// first try getting the definition from
	// the package and object name
	d := i.defs[pkg.Path()+" "+obj.Name()]
	if d != nil {
		return d, nil
	}

	// lookup using the type. Useful for:
	// - local variables
	// - methods (funcs w/ receivers)
	ids, err := i.typeToDef(obj.Name(), obj.Type())
	if err != nil {
		return nil, err
	}

	id := strings.Join(ids, " ")
	return i.Get(id), nil
}

// TypeOf fn
func (i *Index) TypeOf(packagePath string, n ast.Node) (types.Type, error) {
	info := i.program.Package(packagePath)

	id, e := util.GetIdentifier(n)
	if e != nil {
		return nil, e
	}

	return info.TypeOf(id), nil
}

func (i *Index) typeToDef(name string, kind types.Type) (arr []string, err error) {
	switch t := kind.(type) {
	case *types.Basic:
		return arr, nil
	case *types.Interface:
		if t.Empty() {
			return arr, nil
		}
		return arr, fmt.Errorf("typeToDef: unhandled non-interface %s", t)
	case *types.Slice:
		return i.typeToDef(name, t.Elem())
	case *types.Signature:
		recv := t.Recv()
		// TODO: clean this up
		if recv != nil {
			a, e := i.typeToDef(name, recv.Type())
			if e != nil {
				return arr, e
			} else if len(a) == 2 {
				return []string{a[0], name, a[1]}, nil
			}
			return arr, nil
		}
	case *types.Named:
		obj := t.Obj()
		if obj == nil {
			return arr, fmt.Errorf("typeToDef: unhandled unnamed types.Named: %s", t.String())
		}
		// pkg will be nil for Error in error.Error()
		if obj.Pkg() == nil {
			return arr, nil
		}
		return append(arr, obj.Pkg().Path(), obj.Name()), nil
	case *types.Pointer:
		return i.typeToDef(name, t.Elem())
	case *types.Map:
		// TODO: is this always the case?
		return arr, nil
	case *types.Chan:
		return i.typeToDef(name, t.Elem())
	default:
		return arr, fmt.Errorf("typeToDef: unhandled type %T", kind)
	}

	// log.Infof("ident %+v", i/d)
	return arr, nil
}

// func (db *DB) TypeOf(info *loader.PackageInfo, n ast.Node) ()
