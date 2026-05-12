package openapirouter

import (
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v5"
)

type FailureHandler func(c *echo.Context, err error) error

type OpenAPIRoute struct {
	method          string
	path            string
	operation       *openapi3.Operation
	handlers        []echo.HandlerFunc
	failureHandlers []FailureHandler
	doValidation    bool
	doSecurity      bool
}

func newOpenAPIRoute(method, path string, operation *openapi3.Operation) *OpenAPIRoute {
	return &OpenAPIRoute{
		method:       method,
		path:         path,
		operation:    operation,
		doValidation: true,
		doSecurity:   true,
	}
}

func (route *OpenAPIRoute) AddHandler(handler echo.HandlerFunc) *OpenAPIRoute {
	if handler != nil {
		route.handlers = append(route.handlers, handler)
	}
	return route
}

func (route *OpenAPIRoute) Handlers() []echo.HandlerFunc {
	return append([]echo.HandlerFunc(nil), route.handlers...)
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

func (route *OpenAPIRoute) DoValidation() bool {
	return route.doValidation
}

func (route *OpenAPIRoute) SetDoValidation(doValidation bool) *OpenAPIRoute {
	route.doValidation = doValidation
	return route
}

func (route *OpenAPIRoute) DoSecurity() bool {
	return route.doSecurity
}

func (route *OpenAPIRoute) SetDoSecurity(doSecurity bool) *OpenAPIRoute {
	route.doSecurity = doSecurity
	return route
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
