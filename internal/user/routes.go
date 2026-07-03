package user

import "github.com/gin-gonic/gin"

func RegisterRoutes(rg *gin.RouterGroup, h *Handler) {
	rg.POST("/users", h.Create)
	rg.GET("/users", h.List)
	rg.GET("/users/:id", h.Get)
	rg.PUT("/users/:id", h.Update)
	rg.DELETE("/users/:id", h.Delete)

	rg.POST("/login", h.Login)

	protected := rg.Group("/")
	protected.Use(AuthRequired(h.JWTSecret))
	protected.GET("/me", h.Me)
}
