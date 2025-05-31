package middleware

import "github.com/gin-gonic/gin"

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Header.Get("Authorization") != "Bearer token" {
			c.AbortWithStatus(401)
			return
		}
		c.Next()
	}
}