package restapi

import (
	"log/slog"

	"github.com/danielmiessler/fabric/core"
	"github.com/gin-gonic/gin"
)

func Serve(registry *core.PluginRegistry, address string, apiKey string) (err error) {
	r := gin.New()

	// Middleware
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	if apiKey != "" {
		r.Use(APIKeyMiddleware(apiKey))
	} else {
		slog.Warn("Starting REST API server without API key authentication. This may pose security risks.")
	}

	// Register routes
	fabricDb := registry.Db

	// Register most handlers directly on the root engine 'r'
	// These will have paths like /patterns, /chat, etc.
	NewPatternsHandler(r, fabricDb.Patterns)
	NewContextsHandler(r, fabricDb.Contexts)
	NewSessionsHandler(r, fabricDb.Sessions)
	NewChatHandler(r, registry, fabricDb)
	NewConfigHandler(r, fabricDb)
	NewModelsHandler(r, registry.VendorManager)
	NewStrategiesHandler(r)

	// Create a specific group for /api and register YouTubeHandler there
	// This makes YouTube endpoints available at /api/youtube
	apiGroup := r.Group("/api")
	{
		NewYouTubeHandler(apiGroup, registry, fabricDb)
	}

	// Start server
	err = r.Run(address)
	if err != nil {
		return err
	}

	return
}
