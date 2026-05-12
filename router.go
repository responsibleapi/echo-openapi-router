package openapirouter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/labstack/echo/v5"
	validator "github.com/responsibleapi/echo-middleware"
)

const (
	KeyOperation        = "openApiOperation"
	KeyValidatedRequest = "openApiValidatedRequest"
)

var pathParamRE = regexp.MustCompile(`\{([^{}]+)\}`)

type RouterBuilder struct {
	spec              *openapi3.T
	rootMiddlewares   []echo.MiddlewareFunc
	routes            map[string]*OpenAPIRoute
	orderedRoutes     []*OpenAPIRoute
	securityHandlers  map[string][]SecurityHandler
	validationOptions validator.Options
}

type routeRegistrar interface {
	AddRoute(route echo.Route) (echo.RouteInfo, error)
}

func NewRouterBuilder(spec *openapi3.T, options validator.Options) (*RouterBuilder, error) {
	if spec == nil {
		return nil, errors.New("openapi spec cannot be nil")
	}
	if spec.Paths == nil {
		return nil, errors.New("openapi spec paths cannot be nil")
	}
	if err := spec.Validate(context.Background()); err != nil {
		return nil, fmt.Errorf("invalid openapi spec: %w", err)
	}

	builder := &RouterBuilder{
		spec:              spec,
		routes:            make(map[string]*OpenAPIRoute),
		securityHandlers:  make(map[string][]SecurityHandler),
		validationOptions: options,
	}
	if err := builder.collectRoutes(); err != nil {
		return nil, err
	}
	return builder, nil
}

func LoadFromFile(path string, options validator.Options) (*RouterBuilder, error) {
	spec, err := openapi3.NewLoader().LoadFromFile(path)
	if err != nil {
		return nil, err
	}
	return NewRouterBuilder(spec, options)
}

func (builder *RouterBuilder) GetRoute(operationID string) *OpenAPIRoute {
	return builder.route(operationID, "GetRoute")
}

func (builder *RouterBuilder) AddRoute(
	operationID string,
	handler echo.HandlerFunc,
	middleware ...echo.MiddlewareFunc,
) *OpenAPIRoute {
	route := builder.route(operationID, "AddRoute")
	if handler == nil {
		panic(fmt.Sprintf("openapirouter: AddRoute(%q): handler cannot be nil", operationID))
	}
	route.Use(middleware...)
	route.AddHandler(handler)
	return route
}

func (builder *RouterBuilder) Routes() []*OpenAPIRoute {
	return slices.Clone(builder.orderedRoutes)
}

func (builder *RouterBuilder) RootHandler(middleware echo.MiddlewareFunc) *RouterBuilder {
	if middleware != nil {
		builder.rootMiddlewares = append(builder.rootMiddlewares, middleware)
	}
	return builder
}

func (builder *RouterBuilder) Security(name string) *Security {
	return &Security{builder: builder, name: name}
}

func (builder *RouterBuilder) CreateRouter() (*echo.Echo, error) {
	e := echo.New()
	if err := builder.Mount(e); err != nil {
		return nil, err
	}
	return e, nil
}

func (builder *RouterBuilder) Mount(e *echo.Echo) error {
	return builder.mount(e, "")
}

func (builder *RouterBuilder) MountAt(e *echo.Echo, prefix string) error {
	return builder.mount(e, prefix)
}

func (builder *RouterBuilder) mount(e *echo.Echo, prefix string) error {
	if e == nil {
		return errors.New("echo instance cannot be nil")
	}

	group := e.Group(prefix)
	for _, middleware := range builder.rootMiddlewares {
		group.Use(middleware)
	}
	group.Use(builder.validationMiddleware(prefix))
	return builder.addRoutes(group)
}

func (builder *RouterBuilder) addRoutes(registrar routeRegistrar) error {
	for _, route := range builder.orderedRoutes {
		echoRoute, err := builder.echoRoute(route)
		if err != nil {
			return err
		}
		if _, err := registrar.AddRoute(echoRoute); err != nil {
			return err
		}
	}
	return nil
}

func (builder *RouterBuilder) echoRoute(route *OpenAPIRoute) (echo.Route, error) {
	middlewares := []echo.MiddlewareFunc{failureMiddleware(route.failureHandlers), metadataMiddleware(route.operation)}

	securityMiddleware, err := builder.securityMiddleware(route.operation)
	if err != nil {
		return echo.Route{}, err
	}
	if securityMiddleware != nil {
		middlewares = append(middlewares, securityMiddleware)
	}
	middlewares = append(middlewares, route.middlewares...)

	handler := notImplementedHandler
	if len(route.handlers) > 0 {
		handler = routeHandler(route.handlers)
	}

	return echo.Route{
		Method:      route.method,
		Path:        ToEchoPath(route.path),
		Name:        route.operation.OperationID,
		Handler:     handler,
		Middlewares: middlewares,
	}, nil
}

func (builder *RouterBuilder) route(operationID string, method string) *OpenAPIRoute {
	if builder == nil {
		panic(fmt.Sprintf("openapirouter: %s called on nil RouterBuilder", method))
	}
	if route := builder.routes[operationID]; route != nil {
		return route
	}
	panic(fmt.Sprintf(
		"openapirouter: %s(%q): operationId not found in OpenAPI spec; available operationIds: %s",
		method,
		operationID,
		builder.availableOperationIDs(),
	))
}

func (builder *RouterBuilder) availableOperationIDs() string {
	operationIDs := make([]string, 0, len(builder.orderedRoutes))
	for _, route := range builder.orderedRoutes {
		if route == nil || route.operation == nil {
			continue
		}
		operationIDs = append(operationIDs, route.operation.OperationID)
	}
	if len(operationIDs) == 0 {
		return "(none)"
	}
	return strings.Join(operationIDs, ", ")
}

func ToEchoPath(openAPIPath string) string {
	return pathParamRE.ReplaceAllString(openAPIPath, ":$1")
}

func notImplementedHandler(c *echo.Context) error {
	return c.NoContent(http.StatusNotImplemented)
}

func (builder *RouterBuilder) collectRoutes() error {
	for _, path := range builder.spec.Paths.InMatchingOrder() {
		pathItem := builder.spec.Paths.Value(path)
		if pathItem == nil {
			continue
		}
		for _, method := range supportedMethods {
			operation := pathItem.GetOperation(method)
			if operation == nil {
				continue
			}
			if operation.OperationID == "" {
				return fmt.Errorf("%s %s has empty operationId", method, path)
			}
			if _, exists := builder.routes[operation.OperationID]; exists {
				return fmt.Errorf("duplicate operationId %q", operation.OperationID)
			}
			route := newOpenAPIRoute(method, path, operation)
			builder.routes[operation.OperationID] = route
			builder.orderedRoutes = append(builder.orderedRoutes, route)
		}
	}
	return nil
}

func (builder *RouterBuilder) validationMiddleware(prefix string) echo.MiddlewareFunc {
	options := builder.validationOptions
	if prefix != "" {
		options.Prefix = prefix
	}
	if options.Options.AuthenticationFunc == nil {
		options.Options.AuthenticationFunc = openapi3filter.NoopAuthenticationFunc
	}
	return validator.OapiRequestValidatorWithOptions(builder.spec, &options)
}

func metadataMiddleware(operation *openapi3.Operation) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set(KeyOperation, operation)
			return next(c)
		}
	}
}

func routeHandler(handlers []echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		for _, handler := range handlers {
			if err := handler(c); err != nil {
				return err
			}
			if responseCommitted(c) {
				return nil
			}
		}
		return nil
	}
}

var supportedMethods = []string{
	http.MethodConnect,
	http.MethodDelete,
	http.MethodGet,
	http.MethodHead,
	http.MethodOptions,
	http.MethodPatch,
	http.MethodPost,
	http.MethodPut,
	http.MethodTrace,
}
