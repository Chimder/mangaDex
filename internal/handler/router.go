package handler

import (
	_ "mangadex/docs"

	"mangadex/internal/handler/middleware"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func Init() *gin.Engine {
	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"https://*", "http://*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposeHeaders:    []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	{
		r.GET("/swagger", ginSwagger.WrapHandler(swaggerfiles.Handler))
		r.GET("/healthz", func(c *gin.Context) {
			c.String(200, "ok")
		})
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))
	}

	api := r.Group("/api/v1")
	auth := api.Group("/auth")
	{
		auth.GET("/steam", func(ctx *gin.Context) {})
		auth.GET("/steam/callback", func(ctx *gin.Context) {})
	}

	user := api.Group("/user").Use(middleware.AuthMiddleware())
	{
		user.GET("/inventory", func(ctx *gin.Context) {})
		user.POST("/inventory/sync", func(ctx *gin.Context) {})
	}

	return r
}
