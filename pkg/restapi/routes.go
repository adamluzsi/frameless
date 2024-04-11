package restapi

import (
	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/pkg/restapi/internal"
	"go.llib.dev/frameless/spechelper/testent"
	"net/http"
)

func NewRouter(configure ...func(*Router)) *Router {
	router := &Router{}
	for _, c := range configure {
		c(router)
	}
	return router
}

// RouterFrom
//
// DEPRECATED
func RouterFrom[V Routes](v V) *Router {
	switch v := any(v).(type) {
	case Routes:
		r := NewRouter()
		r.MountRoutes(v)
		return r

	default:
		panic("not implemented")
	}
}

type Router struct {
	rootNode *_Node
}

type _Node struct {
	methodsH map[string] /* method */ http.Handler
	defaultH http.Handler

	nodes _Nodes
	mux   *http.ServeMux
}

type _Nodes map[string] /* path */ *_Node

func (r *_Node) LookupNode(path string) (*_Node, bool) {
	if r == nil {
		return nil, false
	}
	if r.nodes == nil {
		return nil, false
	}
	sr, ok := r.nodes[path]
	return sr, ok
}

func (r *_Node) LookupHandler(method string) (http.Handler, bool) {
	if r.methodsH != nil {
		if handler, ok := r.methodsH[method]; ok {
			return handler, ok
		}
	}
	return r.defaultH, r.defaultH != nil
}

func (r *_Node) Merge(oth *_Node) {
	r.init()
	for k, v := range oth.methodsH {
		r.methodsH[k] = v
	}
	for k, v := range oth.nodes {
		r.nodes[k] = v
	}
	if oth.mux != nil {
		if r.mux == nil {
			r.mux = oth.mux
		} else {
			r.mux.Handle("/", oth.mux)
		}
	}
}

func (r *_Node) init() {
	if r.methodsH == nil {
		r.methodsH = make(map[string]http.Handler)
	}
	if r.nodes == nil {
		r.nodes = make(_Nodes)
	}
}

func (router *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if router.rootNode == nil {
		defaultErrorHandler.HandleError(w, r, ErrPathNotFound)
		return
	}
	r, route := internal.WithRoutingContext(r)
	var (
		path     []string
		node     = router.rootNode
		mux      = node.mux
		muxRoute = *route
	)
	for _, part := range pathkit.Split(route.PathLeft) {
		snode, ok := node.LookupNode(part)
		if !ok {
			break
		}
		if snode.mux != nil {
			// mux route should not include the final path part
			// since the mux contain that information if that should be handled or not
			muxRoute = route.Peek(pathkit.Join(path...))
			mux = snode.mux
		}
		path = append(path, part)
		node = snode
	}
	route.Travel(pathkit.Join(path...))
	endpoint, ok := node.LookupHandler(r.Method)
	if ok {
		endpoint.ServeHTTP(w, r)
		return
	}
	if mux != nil {
		var handler http.Handler = mux
		if cur := muxRoute.Current; cur != "/" {
			handler = http.StripPrefix(cur, node.mux)
		}
		handler.ServeHTTP(w, r)
		return
	}
	defaultErrorHandler.HandleError(w, r, ErrPathNotFound)
}

// Mount
func (router *Router) Mount(path string, handler http.Handler) {
	ro := router.mkpath(path)
	switch h := handler.(type) {
	case *Router:
		ro.Merge(h.rootNode)
	default:
		ro.defaultH = h
	}
}

func (router *Router) Namespace(path string, blk func(r *Router)) {
	if pathkit.Canonical(path) == "/" { // TODO: testme
		blk(router)
		return
	}
	router.Mount(path, NewRouter(blk))
}

// Handle registers the handler for the given pattern.
// If a handler already exists for pattern, Handle panics.
func (router *Router) Handle(pattern string, handler http.Handler) {
	ro := router.mkpath(pattern)
	switch h := handler.(type) {
	case *Router:
		ro.Merge(h.rootNode)
	default:
		if ro.mux == nil {
			ro.mux = http.NewServeMux()
		}
		ro.mux.Handle(pattern, handler)
	}
}

func (router *Router) init() {
	if router.rootNode == nil {
		router.rootNode = &_Node{}
		router.rootNode.init()
	}
}

func (router *Router) mkpath(path string) *_Node {
	router.init()
	var ro *_Node
	ro = router.rootNode
	for _, part := range pathkit.Split(path) {
		if ro.nodes == nil {
			ro.nodes = make(_Nodes)
		}
		if _, ok := ro.nodes[part]; !ok {
			ro.nodes[part] = &_Node{}
		}
		ro = ro.nodes[part]
	}
	return ro
}

// Routes
//
// DEPRECATED: this early implementation will be removed in the near future. Use Router directly instead.
type Routes map[string]http.Handler

// MountRoutes
//
// DEPRECATED
func (router *Router) MountRoutes(routes Routes) {
	for path, handler := range routes {
		router.Mount(path, handler)
	}
}

func (router *Router) verb(method, path string, handler http.Handler) {
	router.Namespace(path, func(r *Router) {
		r.init()
		r.rootNode.methodsH[method] = handler
	})
}

func (router *Router) Get(path string, handler http.Handler) {
	router.verb(http.MethodGet, path, handler)
}

func (router *Router) Post(path string, handler http.Handler) {
	router.verb(http.MethodPost, path, handler)
}

func (router *Router) Delete(path string, handler http.Handler) {
	router.verb(http.MethodDelete, path, handler)
}

func (router *Router) Put(path string, handler http.Handler) {
	router.verb(http.MethodPut, path, handler)
}

func (router *Router) Patch(path string, handler http.Handler) {
	router.verb(http.MethodPatch, path, handler)
}

func (router *Router) Head(path string, handler http.Handler) {
	router.verb(http.MethodHead, path, handler)
}

func (router *Router) Connect(path string, handler http.Handler) {
	router.verb(http.MethodConnect, path, handler)
}

func (router *Router) Options(path string, handler http.Handler) {
	router.verb(http.MethodOptions, path, handler)
}

func (router *Router) Trace(path string, handler http.Handler) {
	router.verb(http.MethodTrace, path, handler)
}

func (router *Router) Resource(identifier string, r Resource[testent.Foo, testent.FooID]) {
	Mount(router, pathkit.Canonical(identifier), r)
}
