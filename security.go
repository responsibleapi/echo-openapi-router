package openapirouter

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v5"
)

type SecurityHandler func(c *echo.Context, scheme *openapi3.SecurityScheme, scopes []string) error

func (builder *RouterBuilder) securityScheme(name string) (*openapi3.SecurityScheme, error) {
	if builder.spec.Components == nil || builder.spec.Components.SecuritySchemes == nil {
		return nil, fmt.Errorf("missing security scheme %q", name)
	}
	ref := builder.spec.Components.SecuritySchemes[name]
	if ref == nil || ref.Value == nil {
		return nil, fmt.Errorf("missing security scheme %q", name)
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
