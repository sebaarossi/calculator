package api

import (
	_ "embed"
	"net/http"
)

// The OpenAPI document is written by hand and embedded in the binary, so the
// docs ship with the service and need no code generation step.
//
//go:embed openapi.yaml
var openAPISpec []byte

// swaggerPage renders the spec with Swagger UI, loaded from a CDN.
//
//go:embed swagger.html
var swaggerPage []byte

func handleOpenAPISpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	_, _ = w.Write(openAPISpec)
}

func handleSwaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(swaggerPage)
}
