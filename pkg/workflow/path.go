package workflow

import (
	"context"
	"iter"
	"slices"

	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/slicekit"
)

// Path is the location of a step within the Definition of a Process:
// the Scopes the execution entered, from the root of the Definition down to the step.
//
// A Path is also where the variables of the step live.
// The Scopes marked as Var are the variable scopes of the Path,
// the way the blocks of a Go function are its variable scopes,
// so the variables visible to a step are the ones declared along its Path.
type Path []Scope

// Scope is a single segment of a Path.
type Scope struct {
	// Name identifies the Scope among its siblings.
	Name string
	// Var tells that the Scope is a variable scope as well, a block in Go terms:
	// the variables declared within it are local to it and to the Scopes nested in it.
	Var bool
}

// Equal reports whether two Scopes are the same location.
// A location is identified by its name alone,
// whether it is a variable scope is a property of the location.
func (s Scope) Equal(oth Scope) bool {
	return s.Name == oth.Name
}

func (p Path) Equal(oth Path) bool {
	return slicekit.Equal(p, oth)
}

// MatchPrefix reports whether the path begins with the given prefix.
func (p Path) MatchPrefix(prefix Path) bool {
	return slicekit.MatchPrefix(p, prefix)
}

// commonPrefixLen is the number of leading Scopes the two Paths share.
func commonPrefixLen(a, b Path) int {
	var n int
	for n < len(a) && n < len(b) && a[n].Equal(b[n]) {
		n++
	}
	return n
}

// VarScope returns the innermost variable scope the Path is in,
// which is the Path up to and including its last Var Scope.
//
// A Path without a Var Scope is in the root variable scope of the Process,
// which is the empty Path.
func (p Path) VarScope() Path {
	for i := len(p) - 1; 0 <= i; i-- {
		if p[i].Var {
			return slices.Clip(p[:i+1])
		}
	}
	return nil
}

//--- CurrentPath ---

// CurrentPath returns the location ctx is at within the Definition of the Process.
func CurrentPath(ctx context.Context) Path {
	node, ok := ctxCurrentPath.Lookup(ctx)
	if !ok {
		return nil
	}
	var p Path
	for node := range node.FromRoot() {
		p = append(p, node.Scope)
	}
	return p
}

// WithName extends the current Path with a Scope of the given name.
//
// A Definition that runs other Definitions names the Scope they run in,
// so that every step of the workflow has a location of its own.
func WithName(ctx context.Context, name string) context.Context {
	return withScope(ctx, Scope{Name: name})
}

// WithVarScope extends the current Path with a Scope of the given name,
// which is a variable scope as well.
//
// The variables declared within the Scope are local to it,
// they shadow the variables of the same name from the enclosing variable scopes,
// and they are gone once the execution leaves the Scope.
// The variables of the enclosing scopes stay visible, and an assignment to them writes through.
//
// As the variable scope is a segment of the Path, its identity is the location it is at:
// two Scopes at different locations never share their variables, even if their names are the same.
func WithVarScope(ctx context.Context, name string) context.Context {
	return withScope(ctx, Scope{Name: name, Var: true})
}

var ctxCurrentPath contextkit.ValueHandler[ctxKeyCurrentPath, pathNode]

type ctxKeyCurrentPath struct{}

// --- utils --- //

type pathNode struct {
	Parent *pathNode
	Scope  Scope
}

func (n pathNode) FromRoot() iter.Seq[*pathNode] {
	return func(yield func(*pathNode) bool) {
		var nodes []*pathNode
		var current *pathNode = &n
		for {
			nodes = append(nodes, current)
			if current.Parent != nil {
				current = current.Parent
				continue
			}
			break
		}
		slices.Reverse(nodes)
		for _, node := range nodes {
			if !yield(node) {
				return
			}
		}
	}
}

func withScope(ctx context.Context, scope Scope) context.Context {
	var next = pathNode{Scope: scope}
	if node, ok := ctxCurrentPath.Lookup(ctx); ok {
		next.Parent = &node
	}
	return ctxCurrentPath.ContextWith(ctx, next)
}

// withPath positions ctx at the given Path, replacing the current one.
// An empty Path is the root of the Process.
func withPath(ctx context.Context, path Path) context.Context {
	var node *pathNode
	for _, scope := range path {
		node = &pathNode{Parent: node, Scope: scope}
	}
	if node == nil {
		return ctxCurrentPath.ContextWithout(ctx)
	}
	return ctxCurrentPath.ContextWith(ctx, *node)
}

// atProcessRoot positions ctx at the root of the Process, outside of every loop body.
func atProcessRoot(ctx context.Context) context.Context {
	ctx = withPath(ctx, nil)
	return ctxIterationVars.ContextWithout(ctx)
}
