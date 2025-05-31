package utils

import (
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"
)

func WriteJSON(c *gin.Context, status int, v interface{}) {
	c.JSON(status, v)
}

func WriteJSONRedis(c *gin.Context, status int, data []byte) {
	c.Data(status, "application/json", data)
}

func WriteError(c *gin.Context, status int, err string) {
	slog.Error("HTTP", "err:", err)
	c.AbortWithStatusJSON(status, gin.H{"err": err})
}

func GinParseJSON(c *gin.Context, v interface{}) error {
	if err := c.ShouldBindJSON(v); err != nil {
		return fmt.Errorf("error decoding JSON: %w", err)
	}
	return nil
}
