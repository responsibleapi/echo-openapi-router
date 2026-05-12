package openapirouter

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	validator "github.com/responsibleapi/echo-middleware"
)

func TestToEchoPath(t *testing.T) {
	t.Parallel()

	got := ToEchoPath("/pets/{petId}/owners/{ownerId}")
	want := "/pets/:petId/owners/:ownerId"
	if got != want {
		t.Fatalf("ToEchoPath() = %q, want %q", got, want)
	}
}

func TestBuilderCreatesRoutesWithValidationSecurityAndHandlers(t *testing.T) {
	t.Parallel()

	builder := newTestBuilder(t)
	var order []string

	builder.RootHandler(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			order = append(order, "root")
			return next(c)
		}
	})
	err := builder.Security("api_key").APIKeyHandler(
		func(c *echo.Context, scheme *openapi3.SecurityScheme, _ []string) error {
			order = append(order, "security")
			if c.Request().Header.Get(scheme.Name) != "secret" {
				return echo.NewHTTPError(http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	builder.AddRoute("createPet", func(c *echo.Context) error {
		order = append(order, "handler")
		if c.Get(KeyOperation) == nil {
			t.Fatalf("operation metadata not set")
		}
		return c.NoContent(http.StatusCreated)
	}, func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			order = append(order, "route")
			return next(c)
		}
	})

	e, err := builder.CreateRouter()
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/pets",
		bytes.NewBufferString(`{"name":"fido"}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("X-Api-Key", "secret")
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if want := []string{"root", "security", "route", "handler"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %#v, want %#v", order, want)
	}
}

func TestGetRoutePanicsForUnknownOperationID(t *testing.T) {
	t.Parallel()

	builder := newTestBuilder(t)

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("GetRoute() did not panic")
		}
		message, ok := recovered.(string)
		if !ok {
			t.Fatalf("panic = %#v, want string", recovered)
		}
		if !strings.Contains(message, `GetRoute("missingPet")`) {
			t.Fatalf("panic = %q, want operationId in message", message)
		}
		if !strings.Contains(message, "available operationIds: createPet") {
			t.Fatalf("panic = %q, want available operationIds in message", message)
		}
	}()

	builder.GetRoute("missingPet")
}

func TestAddRoutePanicsForUnknownOperationID(t *testing.T) {
	t.Parallel()

	builder := newTestBuilder(t)

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("AddRoute() did not panic")
		}
		message, ok := recovered.(string)
		if !ok {
			t.Fatalf("panic = %#v, want string", recovered)
		}
		if !strings.Contains(message, `AddRoute("missingPet")`) {
			t.Fatalf("panic = %q, want operationId in message", message)
		}
		if !strings.Contains(message, "available operationIds: createPet") {
			t.Fatalf("panic = %q, want available operationIds in message", message)
		}
	}()

	builder.AddRoute("missingPet", func(c *echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})
}

func TestAddRoutePanicsForNilHandler(t *testing.T) {
	t.Parallel()

	builder := newTestBuilder(t)

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("AddRoute() did not panic")
		}
		message, ok := recovered.(string)
		if !ok {
			t.Fatalf("panic = %#v, want string", recovered)
		}
		if !strings.Contains(message, `AddRoute("createPet"): handler cannot be nil`) {
			t.Fatalf("panic = %q, want nil handler message", message)
		}
	}()

	builder.AddRoute("createPet", nil)
}

func TestRouteAddHandlerPanicsForNilHandler(t *testing.T) {
	t.Parallel()

	builder := newTestBuilder(t)
	route := builder.GetRoute("createPet")

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("AddHandler() did not panic")
		}
		message, ok := recovered.(string)
		if !ok {
			t.Fatalf("panic = %#v, want string", recovered)
		}
		for _, want := range []string{
			`AddHandler("createPet"): handler cannot be nil`,
			"POST /pets",
			"pass a non-nil echo.HandlerFunc",
			"leave the route without handlers to return 501 Not Implemented",
		} {
			if !strings.Contains(message, want) {
				t.Fatalf("panic = %q, want %q", message, want)
			}
		}
	}()

	route.AddHandler(nil)
}

func TestValidationRejectsInvalidRequest(t *testing.T) {
	t.Parallel()

	builder := newTestBuilder(t)
	routeMiddlewareCalled := false
	handlerCalled := false
	addTestSecurity(t, builder)
	builder.AddRoute("createPet", func(c *echo.Context) error {
		handlerCalled = true
		return c.NoContent(http.StatusCreated)
	}, func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			routeMiddlewareCalled = true
			return next(c)
		}
	})
	e, err := builder.CreateRouter()
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/pets",
		bytes.NewBufferString(`{"wrong":"field"}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if routeMiddlewareCalled {
		t.Fatalf("route middleware was called before request validation rejected the request")
	}
	if handlerCalled {
		t.Fatalf("handler was called after request validation rejected the request")
	}
}

func TestBuilderMountsRoutesAtRootWithRequestLogger(t *testing.T) {
	t.Parallel()

	builder := newTestBuilder(t)
	addTestSecurity(t, builder)
	builder.AddRoute("createPet", func(c *echo.Context) error {
		return c.NoContent(http.StatusCreated)
	})

	e := echo.New()
	var logged []middleware.RequestLoggerValues
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogMethod:    true,
		LogURIPath:   true,
		LogRoutePath: true,
		LogStatus:    true,
		LogValuesFunc: func(_ *echo.Context, values middleware.RequestLoggerValues) error {
			logged = append(logged, values)
			return nil
		},
	}))
	if err := builder.Mount(e); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/pets",
		bytes.NewBufferString(`{"name":"fido"}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(logged) != 1 {
		t.Fatalf("logged = %#v, want one request log", logged)
	}
	if got, want := logged[0].Method, http.MethodPost; got != want {
		t.Fatalf("logged method = %q, want %q", got, want)
	}
	if got, want := logged[0].URIPath, "/pets"; got != want {
		t.Fatalf("logged URI path = %q, want %q", got, want)
	}
	if got, want := logged[0].RoutePath, "/pets"; got != want {
		t.Fatalf("logged route path = %q, want %q", got, want)
	}
	if got, want := logged[0].Status, http.StatusCreated; got != want {
		t.Fatalf("logged status = %d, want %d", got, want)
	}
}

func TestBuilderMountsRoutesAtPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "valid request",
			body:       `{"name":"fido"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "invalid request",
			body:       `{"wrong":"field"}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			builder := newTestBuilder(t)
			addTestSecurity(t, builder)
			builder.AddRoute("createPet", func(c *echo.Context) error {
				return c.NoContent(http.StatusCreated)
			})

			e := echo.New()
			if err := builder.MountAt(e, "/api"); err != nil {
				t.Fatal(err)
			}

			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(
				context.Background(),
				http.MethodPost,
				"/api/pets",
				bytes.NewBufferString(tt.body),
			)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			e.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestMissingSecurityHandlerFailsCreateRouter(t *testing.T) {
	t.Parallel()

	builder := newTestBuilder(t)
	builder.AddRoute("createPet", func(c *echo.Context) error {
		return c.NoContent(http.StatusCreated)
	})

	if _, err := builder.CreateRouter(); err == nil {
		t.Fatalf("CreateRouter() err = nil, want missing security handler error")
	}
}

func TestUnhandledOperationReturns501(t *testing.T) {
	t.Parallel()

	builder := newTestBuilder(t)
	addTestSecurity(t, builder)
	e, err := builder.CreateRouter()
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/pets",
		bytes.NewBufferString(`{"name":"fido"}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestUnhandledOperationValidatesRequest(t *testing.T) {
	t.Parallel()

	builder := newTestBuilder(t)
	addTestSecurity(t, builder)
	e, err := builder.CreateRouter()
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/pets",
		bytes.NewBufferString(`{"wrong":"field"}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestFailureHandlerHandlesRouteError(t *testing.T) {
	t.Parallel()

	builder := newTestBuilder(t)
	addTestSecurity(t, builder)
	builder.GetRoute("createPet").
		AddHandler(func(_ *echo.Context) error {
			return echo.NewHTTPError(http.StatusTeapot, "broken")
		}).
		AddFailureHandler(func(c *echo.Context, err error) error {
			return c.String(http.StatusBadGateway, err.Error())
		})
	e, err := builder.CreateRouter()
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/pets",
		bytes.NewBufferString(`{"name":"fido"}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
}

func newTestBuilder(t *testing.T) *RouterBuilder {
	t.Helper()
	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromData([]byte(testSpec))
	if err != nil {
		t.Fatal(err)
	}
	builder, err := NewRouterBuilder(spec, validator.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return builder
}

func addTestSecurity(t *testing.T, builder *RouterBuilder) {
	t.Helper()
	if err := builder.Security("api_key").APIKeyHandler(
		func(*echo.Context, *openapi3.SecurityScheme, []string) error {
			return nil
		},
	); err != nil {
		t.Fatal(err)
	}
}

const testSpec = `
openapi: 3.0.3
info:
  title: test
  version: 1.0.0
paths:
  /pets:
    post:
      operationId: createPet
      security:
        - api_key: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [name]
              additionalProperties: false
              properties:
                name:
                  type: string
      responses:
        '201':
          description: created
components:
  securitySchemes:
    api_key:
      type: apiKey
      in: header
      name: X-API-Key
`
