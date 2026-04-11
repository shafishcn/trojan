package control

import (
	"embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed ui/*
var uiFS embed.FS

func registerUIRoutes(router *gin.Engine) {
	router.GET("/", func(c *gin.Context) {
		indexHTML, err := uiFS.ReadFile("ui/index.html")
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	})
}
