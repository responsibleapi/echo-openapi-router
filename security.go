package openapirouter

import (
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v5"
)

type SecurityHandler func(c *echo.Context, scheme *openapi3.SecurityScheme, scopes []string) error

type Security struct {
	builder *RouterBuilder
	name    string
}

func (security *Security) Handler(handler SecurityHandler) error {
	if handler == nil {
		return errors.New("security handler cannot be nil")
	}
	if _, err := security.scheme(); err != nil {
		return err
	}
	security.builder.securityHandlers[security.name] = append(security.builder.securityHandlers[security.name], handler)
	return nil
}

func (security *Security) APIKeyHandler(handler SecurityHandler) error {
	scheme, err := security.scheme()
	if err != nil {
		return err
	}
	if scheme.Type != "apiKey" {
		return fmt.Errorf("invalid type for apiKey security scheme %q: %s", security.name, scheme.Type)
	}
	if scheme.Name == "" {
		return fmt.Errorf("apiKey security scheme %q has empty name", security.name)
	}
	switch scheme.In {
	case "header", "query", "cookie":
	default:
		return fmt.Errorf("apiKey security scheme %q has invalid in value %q", security.name, scheme.In)
	}
	return security.Handler(handler)
}

func (security *Security) HTTPHandler(schemeName string, handler SecurityHandler) error {
	scheme, err := security.scheme()
	if err != nil {
		return err
	}
	if scheme.Type != "http" {
		return fmt.Errorf("invalid type for http security scheme %q: %s", security.name, scheme.Type)
	}
	if scheme.Scheme != schemeName {
		return fmt.Errorf("invalid scheme for http security scheme %q: %s", security.name, scheme.Scheme)
	}
	return security.Handler(handler)
}

func (security *Security) OAuth2Handler(handler SecurityHandler) error {
	scheme, err := security.scheme()
	if err != nil {
		return err
	}
	if scheme.Type != "oauth2" {
		return fmt.Errorf("invalid type for oauth2 security scheme %q: %s", security.name, scheme.Type)
	}
	return security.Handler(handler)
}

func (security *Security) OpenIDConnectHandler(handler SecurityHandler) error {
	scheme, err := security.scheme()
	if err != nil {
		return err
	}
	if scheme.Type != "openIdConnect" {
		return fmt.Errorf("invalid type for openIdConnect security scheme %q: %s", security.name, scheme.Type)
	}
	if scheme.OpenIdConnectUrl == "" {
		return fmt.Errorf("openIdConnect security scheme %q has empty openIdConnectUrl", security.name)
	}
	return security.Handler(handler)
}

func (security *Security) scheme() (*openapi3.SecurityScheme, error) {
	if security.builder.spec.Components == nil || security.builder.spec.Components.SecuritySchemes == nil {
		return nil, fmt.Errorf("missing security scheme %q", security.name)
	}
	ref := security.builder.spec.Components.SecuritySchemes[security.name]
	if ref == nil || ref.Value == nil {
		return nil, fmt.Errorf("missing security scheme %q", security.name)
	}
	return ref.Value, nil
}

func (builder *RouterBuilder) securityMiddleware(operation *openapi3.Operation) (echo.MiddlewareFunc, error) {
	requirements := builder.effectiveSecurityRequirements(operation)
	if len(requirements) == 0 {
		return nil, nil
	}
	optional := false
	for _, requirement := range requirements {
		if len(requirement) == 0 {
			optional = true
			continue
		}
		for name := range requirement {
			if len(builder.securityHandlers[name]) == 0 {
				return nil, fmt.Errorf("missing security handler for %q", name)
			}
		}
	}
	if optional {
		return nil, nil
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			var firstErr error
			for _, requirement := range requirements {
				if builder.satisfiesSecurityRequirement(c, requirement, &firstErr) {
					return next(c)
				}
			}
			if firstErr != nil {
				return firstErr
			}
			return echo.NewHTTPError(http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
		}
	}, nil
}

func (builder *RouterBuilder) effectiveSecurityRequirements(
	operation *openapi3.Operation,
) openapi3.SecurityRequirements {
	if operation.Security != nil {
		return *operation.Security
	}
	return builder.spec.Security
}

func (builder *RouterBuilder) satisfiesSecurityRequirement(
	c *echo.Context,
	requirement openapi3.SecurityRequirement,
	firstErr *error,
) bool {
	names := make([]string, 0, len(requirement))
	for name := range requirement {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		ref := builder.spec.Components.SecuritySchemes[name]
		for _, handler := range builder.securityHandlers[name] {
			if err := handler(c, ref.Value, requirement[name]); err != nil {
				if *firstErr == nil {
					*firstErr = err
				}
				return false
			}
		}
	}
	return true
}
