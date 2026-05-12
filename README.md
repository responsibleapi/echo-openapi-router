# echo-openapi-router

Echo v5 router builder for OpenAPI 3 contracts, ported from Vert.x Web's `vertx-web-openapi-router` behavior.

It builds Echo routes from OpenAPI operations, converts OpenAPI path parameters (`/pets/{id}`) to Echo path parameters (`/pets/:id`), installs root middleware first, then security checks, request validation, route handlers, and route failure handlers. Request validation is delegated to `github.com/responsibleapi/echo-middleware` at `v1.0.3-responsibleapi.3`.

```go
builder, err := openapirouter.LoadFromFile("openapi.yaml")
if err != nil {
	return err
}

builder.GetRoute("getPet").AddHandler(func(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"id": c.PathValue("id")})
})

e, err := builder.CreateRouter()
if err != nil {
	return err
}
return e.Start(":8080")
```

Security is configured per OpenAPI security scheme:

```go
err := builder.Security("api_key").APIKeyHandler(func(c *echo.Context, scheme *openapi3.SecurityScheme, scopes []string) error {
	if c.Request().Header.Get(scheme.Name) == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "missing api key")
	}
	return nil
})
```

Operations without handlers are still mounted and return `501 Not Implemented`, matching the Vert.x module's fallback route behavior.
