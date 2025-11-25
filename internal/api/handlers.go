// author: weiqiang; date: 2022-03
package api

import (
	"github.com/gin-gonic/gin"
)

// Default Url /
func Default(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": "healthy",
	})
}
