# echo-openapi-router

Echo v5 router builder for OpenAPI 3 contracts, ported from Vert.x Web's [`vertx-web-openapi-router`](https://vertx.io/docs/vertx-web-openapi-router/java/) behavior.

It builds Echo routes from OpenAPI operations, converts OpenAPI path parameters (`/pets/{id}`) to Echo path parameters (`/pets/:id`), mounts generated routes directly into Echo, and installs root middleware, request validation, security checks, route handlers, and route failure handlers. Request validation is delegated to `github.com/responsibleapi/echo-middleware` at `v1.0.3-responsibleapi.3`.

## Install

```sh
go get github.com/responsibleapi/echo-openapi-router
```

## Use

```go
import (
	"net/http"

	openapirouter "github.com/responsibleapi/echo-openapi-router"
	"github.com/labstack/echo/v5"
	validator "github.com/responsibleapi/echo-middleware"
)
```

```go
builder, err := openapirouter.LoadFromFile("openapi.yaml", validator.Options{})
if err != nil {
	return err
}

builder.AddRoute("getPet", func(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"id": c.PathValue("id")})
})

e := echo.New()
e.GET("/healthz", func(c *echo.Context) error {
	return c.NoContent(http.StatusNoContent)
})
if err := builder.Mount(e); err != nil {
	return err
}

return e.Start(":8080")
```

Use `builder.MountAt(e, "/api")` to mount generated routes under a path prefix. Use `builder.CreateRouter()` when you want a standalone `*echo.Echo`.

Security is configured per OpenAPI security scheme:

```go
builder.Security("api_key", func(c *echo.Context, scheme *openapi3.SecurityScheme, scopes []string) error {
	if c.Request().Header.Get(scheme.Name) == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "missing api key")
	}
	return nil
})
```

Operations without handlers are still mounted and return `501 Not Implemented`, matching the Vert.x module's fallback route behavior.
