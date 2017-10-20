package defs

import (
	"go/ast"
	"go/types"
	"strings"

	"github.com/fatih/structtag"
	"github.com/matthewmueller/golly/golang/def"
	"github.com/matthewmueller/golly/golang/index"

	"golang.org/x/tools/go/loader"
)

// Valuer interface
type Valuer interface {
	def.Definition
	Node() *ast.ValueSpec
}

var _ Valuer = (*values)(nil)

type values struct {
	exported  bool
	path      string
	name      string
	id        string
	index     *index.Index
	node      *ast.ValueSpec
	kind      types.Type
	processed bool
	deps      []def.Definition
	imports   map[string]string
	async     bool
	omit      bool
	tag       *structtag.Tag
}

// Value fn
func Value(index *index.Index, info *loader.PackageInfo, gn *ast.GenDecl, n *ast.ValueSpec) (def.Definition, error) {
	packagePath := info.Pkg.Path()
	names := []string{}
	exported := false

	for _, ident := range n.Names {
		obj := info.ObjectOf(ident)
		if obj.Exported() {
			exported = true
		}
		names = append(names, ident.Name)
	}

	name := strings.Join(names, ",")
	idParts := []string{packagePath, name}
	id := strings.Join(idParts, " ")

	return &values{
		exported: exported,
		path:     packagePath,
		name:     name,
		id:       id,
		index:    index,
		node:     n,
		imports:  map[string]string{},
	}, nil
}

func (d *values) process() (err error) {
	state, e := process(d.index, d, d.node)
	if e != nil {
		return e
	}

	// copy state into function
	d.processed = true
	d.async = state.async
	d.deps = state.deps
	d.imports = state.imports
	d.omit = state.omit
	d.tag = state.tag

	return nil
}

func (d *values) ID() string {
	return d.id
}

func (d *values) Name() string {
	return d.name
}

func (d *values) Kind() string {
	return "VALUE"
}

func (d *values) Path() string {
	return d.path
}

func (d *values) Dependencies() (defs []def.Definition, err error) {
	if d.processed {
		return d.deps, nil
	}
	e := d.process()
	if e != nil {
		return defs, e
	}
	return d.deps, nil
}

func (d *values) Exported() bool {
	return d.exported
}

func (d *values) Omitted() bool {
	return false
}

func (d *values) Node() *ast.ValueSpec {
	return d.node
}

func (d *values) Type() types.Type {
	return d.kind
}

func (d *values) Imports() map[string]string {
	return d.index.GetImports(d.path)
}

func (d *values) FromRuntime() bool {
	return false
}
