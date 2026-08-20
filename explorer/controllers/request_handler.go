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

func (rh RequestHandler) GetHttpMethod() string        { return rh.httpMethod }
func (rh RequestHandler) GetPath() string              { return rh.path }
func (rh RequestHandler) GetHandler() http.HandlerFunc { return rh.handler }
