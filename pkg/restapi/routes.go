package restapi

import (
	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/pkg/restapi/internal"
	"net/http"
	"sync"
)

func NewRouter(configure ...func(*Router)) *Router {
	router := &Router{}
	for _, c := range configure {
		c(router)
	}
	return router
}

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
	route *route
	mutex sync.RWMutex
}

type route struct {
	Handler http.Handler
	Routes  map[string]*route
}

func (r *route) GetRoutes() map[string]*route {
	if r.Routes == nil {
		r.Routes = make(map[string]*route)
	}
	return r.Routes
}

func (r *route) Ensure(part string) *route {
	if _, ok := r.GetRoutes()[part]; !ok {
		r.Routes[part] = &route{}
	}
	return r.Routes[part]
}

func (r *route) Lookup(part string) (*route, bool) {
	if r == nil {
		return nil, false
	}
	if r.Routes == nil {
		return nil, false
	}
	sr, ok := r.Routes[part]
	return sr, ok
}

func (router *Router) ServeHTTP(responseWriter http.ResponseWriter, request *http.Request) {
	router.mutex.RLock()
	defer router.mutex.RUnlock()
	if router.route == nil {
		defaultErrorHandler.HandleError(responseWriter, request, ErrPathNotFound)
		return
	}
	request, rc := internal.WithRoutingCountex(request)
	var (
		mount   []string
		route   = router.route
		handler = route.Handler
	)
	for _, part := range pathkit.Split(rc.Path) {
		var ok bool
		route, ok = route.Lookup(part)
		if !ok {
			break
		}
		mount = append(mount, part)
		handler = route.Handler
	}
	if handler == nil {
		defaultErrorHandler.HandleError(responseWriter, request, ErrPathNotFound)
		return
	}
	withMountPoint(rc, pathkit.Join(mount...))
	handler.ServeHTTP(responseWriter, request)
}

func (router *Router) Mount(path Path, handler http.Handler) {
	router.mutex.Lock()
	defer router.mutex.Unlock()

	if router.route == nil {
		router.route = &route{}
	}

	var ro *route
	ro = router.route
	for _, part := range pathkit.Split(path) {
		ro = ro.Ensure(part)
	}

	ro.Handler = handler
}

type (
	Path   = string
	Routes map[Path]http.Handler
)

func (router *Router) MountRoutes(routes Routes) {
	for path, handler := range routes {
		router.Mount(path, handler)
	}
}
