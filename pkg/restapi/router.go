package restapi

import (
	"context"
	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/pkg/restapi/internal"
	"go.llib.dev/frameless/pkg/slicekit"
	"net/http"
)

func NewRouter(configure ...func(*Router)) *Router {
	router := &Router{}
	for _, c := range configure {
		c(router)
	}
	return router
}

type Router struct {
	root *_Node
}

type _Node struct {
	middlewares []httpkit.MiddlewareFactoryFunc
	methodsH    map[string] /* method */ http.Handler
	defaultH    http.Handler

	fixNodes _FixNodes
	dynNodes _DynNode

	mux *http.ServeMux
}

type _FixNodes map[string] /* path */ *_Node

type _DynNode struct {
	node     *_Node
	varnames []string
}

func (dn *_DynNode) NodeFor(pathParamName string) *_Node {
	if dn.node == nil {
		dn.node = &_Node{}
		dn.node.init()
	}
	if slicekit.Contains(dn.varnames, pathParamName) {
		return dn.node
	}
	dn.varnames = append(dn.varnames, pathParamName)
	return dn.node
}

func (dn *_DynNode) LookupNode(ctx context.Context, pathpart string) (context.Context, *_Node, bool) {
	if len(dn.varnames) == 0 || dn.node == nil {
		return nil, nil, false
	}
	for _, varname := range dn.varnames {
		ctx = WithPathParam(ctx, varname, pathpart)
	}
	return ctx, dn.node, true
}

func (r *_Node) LookupNode(ctx context.Context, pathpart string) (context.Context, *_Node, bool) {
	if r == nil {
		return nil, nil, false
	}
	if r.fixNodes != nil {
		if sr, ok := r.fixNodes[pathpart]; ok {
			return ctx, sr, true
		}
	}
	if ctx, sr, ok := r.dynNodes.LookupNode(ctx, pathpart); ok {
		return ctx, sr, true
	}
	return nil, nil, false
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
	if oth == nil {
		return
	}
	r.init()
	for k, v := range oth.methodsH {
		r.methodsH[k] = v
	}
	for k, v := range oth.fixNodes {
		r.fixNodes[k] = v
	}
	if oth.mux != nil {
		if r.mux == nil {
			r.mux = oth.mux
		} else {
			r.mux.Handle("/", oth.mux)
		}
	}
	if 0 < len(oth.middlewares) {
		r.middlewares = append(r.middlewares, oth.middlewares...)
	}
	if oth.dynNodes.node != nil {
		for _, vn := range oth.dynNodes.varnames {
			r.dynNodes.NodeFor(vn)
		}
		r.dynNodes.node.Merge(oth.dynNodes.node)
	}
}

func (r *_Node) init() {
	if r.methodsH == nil {
		r.methodsH = make(map[string]http.Handler)
	}
	if r.fixNodes == nil {
		r.fixNodes = make(_FixNodes)
	}
}

func (router *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if router.root == nil {
		defaultErrorHandler.HandleError(w, r, ErrPathNotFound)
		return
	}
	r, route := internal.WithRoutingContext(r)
	var (
		ctx      = r.Context()
		path     []string
		node     = router.root
		mux      = node.mux
		muxRoute = *route
		mws      = slicekit.Clone(router.root.middlewares)
	)
	for _, pathpart := range pathkit.Split(route.PathLeft) {
		sctx, snode, ok := node.LookupNode(ctx, pathpart)
		if !ok {
			break
		}
		if 0 < len(snode.middlewares) {
			mws = append(mws, snode.middlewares...)
		}
		path = append(path, pathpart)
		node = snode
		ctx = sctx
		if snode.mux != nil { // mux fallback handler
			// mux route should not include the final path part
			// since the mux contain that information if that should be handled or not
			muxRoute = route.Peek(pathkit.Join(path...))
			mux = snode.mux
		}
	}
	r = r.WithContext(ctx)
	route.Travel(pathkit.Join(path...))
	handler := router.toHTTPHandler(r, node, mux, muxRoute)
	handler = httpkit.WithMiddleware(handler, mws...)
	handler.ServeHTTP(w, r)
}

func (router *Router) toHTTPHandler(r *http.Request, node *_Node, mux *http.ServeMux, muxRoute internal.Routing) http.Handler {
	endpoint, ok := node.LookupHandler(r.Method)
	if ok {
		return endpoint
	}
	if mux != nil {
		var handler http.Handler = mux
		if cur := muxRoute.Current; cur != "/" {
			handler = http.StripPrefix(cur, node.mux)
		}
		return handler
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defaultErrorHandler.HandleError(w, r, ErrPathNotFound)
	})
}

// Mount will mount a handler to the router.
// Mounting a handler will make the path observed as its root point to the handler.
// TODO: make this true :D
func (router *Router) Mount(path string, handler http.Handler) {
	ro := router.mkpath(path)
	switch h := handler.(type) {
	case *Router:
		ro.Merge(h.root)
	default:
		ro.defaultH = h
	}
}

func (router *Router) Namespace(path string, blk func(r *Router)) {
	ro := router.mkpath(path)
	blk(&Router{root: ro})
	//if pathkit.Canonical(path) == "/" { // TODO: testme
	//	blk(router)
	//	return
	//}
	//router.Mount(path, NewRouter(blk))
}

// Handle registers the handler for the given pattern.
// If a handler already exists for pattern, Handle panics.
func (router *Router) Handle(pattern string, handler http.Handler) {
	router.init()
	switch h := handler.(type) {
	case *Router:
		ro := router.mkpath(pattern)
		ro.Merge(h.root)
	default:
		// alternatively, you can traverse the path,
		// and memorise what path must be stripped from the handler
		ro := router.root
		if ro.mux == nil {
			ro.mux = http.NewServeMux()
		}
		ro.mux.Handle(pattern, handler)
	}
}

func (router *Router) init() {
	if router.root == nil {
		router.root = &_Node{}
	}
	router.root.init()
}

func (router *Router) mkpath(path string) *_Node {
	router.init()
	var ro *_Node
	ro = router.root
	for _, part := range pathkit.Split(path) {
		ro.init()
		if ro.fixNodes == nil {
			ro.fixNodes = make(_FixNodes)
		}
		if vn, ok := isPathParamPlaceholder(part); ok {
			ro = ro.dynNodes.NodeFor(vn)
			continue
		}
		if _, ok := ro.fixNodes[part]; !ok {
			ro.fixNodes[part] = &_Node{}
		}
		ro = ro.fixNodes[part]
	}
	return ro
}

func (router *Router) On(method, path string, handler http.Handler) {
	router.Namespace(path, func(r *Router) {
		r.init()
		r.root.methodsH[method] = handler
	})
}

func (router *Router) Get(path string, handler http.Handler) {
	router.On(http.MethodGet, path, handler)
}

func (router *Router) Post(path string, handler http.Handler) {
	router.On(http.MethodPost, path, handler)
}

func (router *Router) Delete(path string, handler http.Handler) {
	router.On(http.MethodDelete, path, handler)
}

func (router *Router) Put(path string, handler http.Handler) {
	router.On(http.MethodPut, path, handler)
}

func (router *Router) Patch(path string, handler http.Handler) {
	router.On(http.MethodPatch, path, handler)
}

func (router *Router) Head(path string, handler http.Handler) {
	router.On(http.MethodHead, path, handler)
}

func (router *Router) Connect(path string, handler http.Handler) {
	router.On(http.MethodConnect, path, handler)
}

func (router *Router) Options(path string, handler http.Handler) {
	router.On(http.MethodOptions, path, handler)
}

func (router *Router) Trace(path string, handler http.Handler) {
	router.On(http.MethodTrace, path, handler)
}

// Resource will register a restful resource path using the Resource handler.
//
// Paths for Router.Resource("/users", restapi.Resource[User, UserID]):
//
//	GET 	/users
//	POST 	/users
//	GET 	/users/:id
//	PUT 	/users/:id
//	DELETE	/users/:id
func (router *Router) Resource(identifier string, resource resource) {
	router.Mount(pathkit.Canonical(identifier), resource)
}

// Use will instruct the router to use a given MiddlewareFactoryFunc to
func (router *Router) Use(mws ...httpkit.MiddlewareFactoryFunc) {
	router.init()
	router.root.middlewares = append(router.root.middlewares, mws...)
}
