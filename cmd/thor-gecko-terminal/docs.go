// API documentation handlers — serves OpenAPI spec and Swagger UI.

package main

import (
	_ "embed"
	"net/http"

	echo "github.com/labstack/echo/v4"
)

//go:embed openapi.yaml
var openAPISpec []byte

const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>THORChain DEX API</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
  <style>body { margin: 0; }</style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = function () {
      SwaggerUIBundle({
        url: '/openapi.yaml',
        dom_id: '#swagger-ui',
        deepLinking: true,
        docExpansion: 'list',
      });
    };
  </script>
</body>
</html>`

// OpenAPISpecHandler serves the embedded OpenAPI spec as YAML.
func OpenAPISpecHandler(c echo.Context) error {
	return c.Blob(http.StatusOK, "application/yaml", openAPISpec)
}

// SwaggerUIHandler serves a self-contained Swagger UI page that points at
// /openapi.yaml. UI assets are loaded from a public CDN.
func SwaggerUIHandler(c echo.Context) error {
	return c.HTML(http.StatusOK, swaggerUIHTML)
}
