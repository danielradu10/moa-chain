package controllers

import "net/http"

// RequestHandler pairs an HTTP method and path with its handler function.
type RequestHandler struct {
	httpMethod string
	path       string
	handler    http.HandlerFunc
}

func newRequestHandler(httpMethod, path string, handler http.HandlerFunc) RequestHandler {
	return RequestHandler{httpMethod: httpMethod, path: path, handler: handler}
}

// GetHttpMethod returns the HTTP method this handler is registered for.
func (rh RequestHandler) GetHttpMethod() string { return rh.httpMethod }

// GetPath returns the URL path pattern this handler is registered for.
func (rh RequestHandler) GetPath() string { return rh.path }

// GetHandler returns the handler function for this route.
func (rh RequestHandler) GetHandler() http.HandlerFunc { return rh.handler }
