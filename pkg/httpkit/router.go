package httpkit

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"

	"go.llib.dev/frameless/pkg/httpkit/internal"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/pathkit"
	rf "go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/synckit"
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
	middlewares []MiddlewareFactoryFunc
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

func (dn *_DynNode) LookupNode(ctx context.Context, rawpathpart string) (context.Context, *_Node, bool) {
	if len(dn.varnames) == 0 || dn.node == nil {
		return nil, nil, false
	}
	for _, varname := range dn.varnames {
		varval, err := url.PathUnescape(rawpathpart)
		if err != nil {
			varval = rawpathpart
		}
		ctx = WithPathParam(ctx, varname, varval)
	}
	return ctx, dn.node, true
}

func (r *_Node) LookupNode(ctx context.Context, rawpathpart string) (context.Context, *_Node, bool) {
	if r == nil {
		return nil, nil, false
	}
	if r.fixNodes != nil {
		if sr, ok := r.fixNodes[rawpathpart]; ok {
			return ctx, sr, true
		}
	}
	if ctx, sr, ok := r.dynNodes.LookupNode(ctx, rawpathpart); ok {
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
		ctx       = r.Context()
		node      = router.root
		mux       = node.mux
		muxRoute  = *route
		mws       = slicekit.Clone(router.root.middlewares)
		pathParts []string
	)
	for _, pathpart := range pathkit.Split(route.PathLeft) {
		sctx, snode, ok := node.LookupNode(ctx, pathpart)
		if !ok {
			break
		}
		if 0 < len(snode.middlewares) {
			mws = append(mws, snode.middlewares...)
		}
		pathParts = append(pathParts, pathpart)
		node = snode
		ctx = sctx
		if snode.mux != nil { // mux fallback handler
			// mux route should not include the final path part
			// since the mux contain that information if that should be handled or not
			muxRoute = route.Peek(pathkit.Join(pathParts...))
			mux = snode.mux
		}
	}
	r = r.WithContext(ctx)
	route.Travel(pathkit.Join(pathParts...))
	handler := router.toHTTPHandler(r, node, mux, muxRoute)
	handler = WithMiddleware(handler, mws...)
	handler.ServeHTTP(w, r)
}

func (router *Router) toHTTPHandler(r *http.Request, node *_Node, mux *http.ServeMux, muxRoute internal.Routing) http.Handler {
	endpoint, ok := node.LookupHandler(r.Method)
	if ok {
		return endpoint
	}
	if mux != nil {
		var handler http.Handler = mux
		if muxRoute.Current != "/" {
			handler = stripPrefix(muxRoute.Current, handler) // TODO: testme
			// handler = http.StripPrefix(muxRoute.Current, handler) // TODO: testme
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

func (router *Router) Namespace(path string, blk func(ro *Router)) {
	blk(router.Sub(path))
}

func (router *Router) Sub(path string) *Router {
	return &Router{root: router.mkpath(path)}
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
// Paths for Router.Resource("/users", httpkit.RESTHandler[User, UserID]):
//
//	INDEX	- GET 		/users
//	CREATE	- POST 		/users
//	SHOW 	- GET 		/users/:id
//	UPDATE	- PUT 		/users/:id
//	DESTROY	- DELETE	/users/:id
func (router *Router) Resource(identifier string, h restHandler) {
	router.Mount(pathkit.Canonical(identifier), h)
}

// restHandler is a generic interface for the typesafe RESTHandler[ENT, ID]
type restHandler interface {
	restHandler()
	http.Handler
}

// Use will instruct the router to use a given MiddlewareFactoryFunc to
func (router *Router) Use(mws ...MiddlewareFactoryFunc) {
	router.init()
	router.root.middlewares = append(router.root.middlewares, mws...)
}

func (router *Router) RouteInfo() RouteInfo {
	return nodeRoutes(router.root)
}

///////////////////////////////////////////////////////////////////////////////////////////////////

const allPathInfoMethod = "ALL"

func GetRouteInfo(h http.Handler) RouteInfo {
	if h == nil {
		return nil
	}
	if docs, ok := h.(RouteInformer); ok {
		return docs.RouteInfo()
	}
	var httpHandlerType = reflect.TypeOf(h)
	if doc, ok := riReg.Lookup(httpHandlerType); ok {
		return doc(h)
	}
	// by default a handler will receive all call
	return []PathInfo{{Method: allPathInfoMethod, Path: "/"}}
}

var riReg synckit.Map[reflect.Type, func(h http.Handler) RouteInfo]

func RegisterRouteInformer[T http.Handler](fn func(v T) RouteInfo) func() {
	var httpHandlerType = rf.TypeOf[T]()
	riReg.Set(httpHandlerType, func(h http.Handler) RouteInfo {
		return fn(h.(T))
	})
	return func() { riReg.Delete(httpHandlerType) }
}

type RouteInfo []PathInfo

type PathInfo struct {
	Method string `enum:"ALL,POST,GET,PUT,PATCH,DELETE,"`
	Path   string
	Desc   string
}

func (ri RouteInfo) String() string {
	var width int
	for _, s := range ri {
		if l := len(s.Method); l > width {
			width = l
		}
	}
	var rs []string
	for _, r := range ri {
		rs = append(rs, fmt.Sprintf("%-*s %s", width, r.Method, r.Path))
	}
	return strings.Join(rs, "\n")
}

func (ri RouteInfo) WithMountPoint(MountPoint string) RouteInfo {
	return slicekit.Map(ri, func(pi PathInfo) PathInfo {
		var lastChar rune
		for _, char := range pi.Path {
			lastChar = char
		}
		ogPathLen := len(pi.Path)
		pi.Path = pathkit.Join(MountPoint, pi.Path)
		if lastChar == '/' && 1 < ogPathLen {
			pi.Path += "/"
		}
		return pi
	})
}

type RouteInformer interface {
	RouteInfo() RouteInfo
}

func nodeRoutes(node *_Node) RouteInfo {
	var ri RouteInfo
	if node == nil {
		return ri
	}
	{ // root endpoints
		var entries []PathInfo
		for method := range node.methodsH {
			entries = append(entries, PathInfo{Method: method, Path: "/"})
		}
		sort.Slice(entries, func(i, j int) bool {
			a, b := entries[i], entries[j]
			return httpMethodPriority(a.Method) < httpMethodPriority(b.Method)
		})
		ri = append(ri, entries...)
	}
	{ // fixed endpoints
		subRoutes := mapkit.ToSlice(node.fixNodes)
		sort.Slice(subRoutes, func(i, j int) bool {
			a, b := subRoutes[i], subRoutes[j]
			return a.Key < b.Key
		})
		var entries []PathInfo
		for _, re := range subRoutes {
			srs := nodeRoutes(re.Value).WithMountPoint(re.Key)
			entries = append(entries, srs...)
		}
		ri = append(ri, entries...)
	}
	{ // dynamic paths
		var (
			// :var1,:var2,:var3
			dynvarpath = strings.Join(slicekit.Map(node.dynNodes.varnames,
				func(vn string) string { return ":" + vn }), ",")
			dynroutes = nodeRoutes(node.dynNodes.node).WithMountPoint(dynvarpath)
		)
		ri = append(ri, dynroutes...)
	}
	{ // default handler
		if node.defaultH != nil {
			ri = append(ri, GetRouteInfo(node.defaultH)...)
		}
	}
	{ // http.ServeMux
		if node.mux != nil {
			ri = append(ri, GetRouteInfo(node.mux)...)
		}
	}
	return ri
}

var _ = RegisterRouteInformer[*http.ServeMux](httpServeMuxRouteInfo)

func httpServeMuxRoutingNodeRouteInfo(v reflect.Value) RouteInfo {
	if rf.IsNil(v) {
		return nil
	}

	v = rf.BaseValue(v) // accept both routingNode and *routingNode
	var path string

	var patternToString = func(pattern reflect.Value) string {
		if rf.IsNil(pattern) {
			return "/"
		}
		pattern = rf.BaseValue(pattern)
		_, part, _ := rf.LookupField(pattern, "str")
		return part.String()
	}

	if _, patternPtr, ok := rf.LookupField(v, "pattern"); ok && !rf.IsNil(patternPtr) {
		path = patternToString(patternPtr)
	}

	var ri RouteInfo

	if _, handler, ok := rf.LookupField(v, "handler"); ok && !rf.IsNil(handler) {
		if handler.CanInterface() {
			if h, ok := handler.Interface().(http.Handler); ok {
				nri := GetRouteInfo(h).WithMountPoint(path)
				ri = append(ri, nri...)
			} else {
				ri = append(ri, PathInfo{
					Method: allPathInfoMethod,
					Path:   path,
				})
			}
		} else {
			ri = append(ri, PathInfo{
				Method: allPathInfoMethod,
				Path:   path,
			})
		}
	}

	if _, children, ok := rf.LookupField(v, "children"); ok && !rf.IsNil(children) {
		if _, s, ok := rf.LookupField(children, "s"); ok && !rf.IsNil(s) {
			for _, entry := range rf.OverSlice(s) {
				_, routingNodePtr, ok := rf.LookupField(entry, "value") /* *routingNode */
				if !ok {
					continue
				}

				ri = append(ri, httpServeMuxRoutingNodeRouteInfo(routingNodePtr)...)
			}
		}
		if _, m, ok := rf.LookupField(children, "m"); ok && !rf.IsEmpty(m) { // map[string, *routingNode]
			for _, routingNodePtr := range rf.OverMap(m) {
				ri = append(ri, httpServeMuxRoutingNodeRouteInfo(routingNodePtr)...)
			}
		}
	}

	if _, multiChild, ok := rf.LookupField(v, "multiChild"); ok && !rf.IsNil(multiChild) {
		ri = append(ri, httpServeMuxRoutingNodeRouteInfo(multiChild)...)
	}

	if _, emptyChild, ok := rf.LookupField(v, "emptyChild"); ok && !rf.IsNil(emptyChild) {
		ri = append(ri, httpServeMuxRoutingNodeRouteInfo(emptyChild)...)
	}

	return ri

}
func httpServeMuxRouteInfo(mux *http.ServeMux) RouteInfo {
	var rmux reflect.Value
	rmux = reflect.ValueOf(mux)
	rmux = rf.BaseValue(rmux)
	var ri RouteInfo
	if _, tree, ok := rf.LookupField(rmux, "tree"); ok {
		ri = append(ri, httpServeMuxRoutingNodeRouteInfo(tree)...)
	}
	return slicekit.Unique(ri)
}

func httpMethodPriority(method string) int {
	for i, comp := range httpMethodOrdering {
		if method == comp {
			return i + 1
		}
	}
	return math.MaxInt
}

var httpMethodOrdering = []string{
	http.MethodPost,   // Create
	http.MethodGet,    // Read
	http.MethodPut,    // Update
	http.MethodPatch,  //
	http.MethodDelete, // destroy

	http.MethodHead,
	http.MethodConnect,
	http.MethodOptions,
	http.MethodTrace,
}
