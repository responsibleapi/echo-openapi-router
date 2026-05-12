package openapirouter

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v5"
)

type FailureHandler func(c *echo.Context, err error) error

type OpenAPIRoute struct {
	method          string
	path            string
	operation       *openapi3.Operation
	handlers        []echo.HandlerFunc
	middlewares     []echo.MiddlewareFunc
	failureHandlers []FailureHandler
}

func newOpenAPIRoute(method string, path string, operation *openapi3.Operation) *OpenAPIRoute {
	return &OpenAPIRoute{
		method:    method,
		path:      path,
		operation: operation,
	}
}

func (route *OpenAPIRoute) AddHandler(handler echo.HandlerFunc, middleware ...echo.MiddlewareFunc) *OpenAPIRoute {
	if route == nil {
		panic(
			"openapirouter: OpenAPIRoute.AddHandler called on nil route; " +
				"get a route from RouterBuilder.GetRoute or RouterBuilder.AddRoute before adding handlers",
		)
	}
	if handler == nil {
		operationID := "(unknown)"
		if route.operation != nil && route.operation.OperationID != "" {
			operationID = route.operation.OperationID
		}
		panic(fmt.Sprintf(
			"openapirouter: AddHandler(%q): handler cannot be nil for %s %s; "+
				"pass a non-nil echo.HandlerFunc, or leave the route without handlers to return 501 Not Implemented",
			operationID,
			route.method,
			route.path,
		))
	}
	route.Use(middleware...)
	route.handlers = append(route.handlers, handler)
	return route
}

func (route *OpenAPIRoute) Handlers() []echo.HandlerFunc {
	return append([]echo.HandlerFunc(nil), route.handlers...)
}

func (route *OpenAPIRoute) Use(middleware ...echo.MiddlewareFunc) *OpenAPIRoute {
	for _, m := range middleware {
		if m != nil {
			route.middlewares = append(route.middlewares, m)
		}
	}
	return route
}

func (route *OpenAPIRoute) Middlewares() []echo.MiddlewareFunc {
	return append([]echo.MiddlewareFunc(nil), route.middlewares...)
}

func (route *OpenAPIRoute) AddFailureHandler(handler FailureHandler) *OpenAPIRoute {
	if handler != nil {
		route.failureHandlers = append(route.failureHandlers, handler)
	}
	return route
}

func (route *OpenAPIRoute) FailureHandlers() []FailureHandler {
	return append([]FailureHandler(nil), route.failureHandlers...)
}

func (route *OpenAPIRoute) Operation() *openapi3.Operation {
	return route.operation
}

func (route *OpenAPIRoute) Method() string {
	return route.method
}

func (route *OpenAPIRoute) Path() string {
	return route.path
}

func failureMiddleware(handlers []FailureHandler) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			err := next(c)
			if err == nil || len(handlers) == 0 {
				return err
			}
			for _, handler := range handlers {
				if handlerErr := handler(c, err); handlerErr != nil {
					err = handlerErr
				}
				if responseCommitted(c) {
					return nil
				}
			}
			return err
		}
	}
}

func responseCommitted(c *echo.Context) bool {
	response, ok := c.Response().(*echo.Response)
	return ok && response.Committed
}
