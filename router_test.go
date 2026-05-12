package openapirouter

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v5"
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
	builder.GetRoute("createPet").AddHandler(func(c *echo.Context) error {
		order = append(order, "handler")
		if c.Get(KeyOperation) == nil {
			t.Fatalf("operation metadata not set")
		}
		return c.NoContent(http.StatusCreated)
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
	if want := []string{"root", "security", "handler"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %#v, want %#v", order, want)
	}
}

func TestValidationRejectsInvalidRequest(t *testing.T) {
	t.Parallel()

	builder := newTestBuilder(t)
	builder.GetRoute("createPet").SetDoSecurity(false).AddHandler(func(c *echo.Context) error {
		return c.NoContent(http.StatusCreated)
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
}

func TestValidationCanBeDisabled(t *testing.T) {
	t.Parallel()

	builder := newTestBuilder(t)
	builder.GetRoute("createPet").
		SetDoSecurity(false).
		SetDoValidation(false).
		AddHandler(func(c *echo.Context) error {
			return c.NoContent(http.StatusCreated)
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

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestMissingSecurityHandlerFailsCreateRouter(t *testing.T) {
	t.Parallel()

	builder := newTestBuilder(t)
	builder.GetRoute("createPet").AddHandler(func(c *echo.Context) error {
		return c.NoContent(http.StatusCreated)
	})

	if _, err := builder.CreateRouter(); err == nil {
		t.Fatalf("CreateRouter() err = nil, want missing security handler error")
	}
}

func TestUnhandledOperationReturns501(t *testing.T) {
	t.Parallel()

	builder := newTestBuilder(t)
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

func TestFailureHandlerHandlesRouteError(t *testing.T) {
	t.Parallel()

	builder := newTestBuilder(t)
	builder.GetRoute("createPet").
		SetDoSecurity(false).
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
	builder, err := NewRouterBuilder(spec)
	if err != nil {
		t.Fatal(err)
	}
	return builder
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
