package router

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	commonauth "github.com/nusiss-capstone-project/identity-mservice/common/auth"
	"github.com/nusiss-capstone-project/payment-mservice/server/config"
	_ "github.com/nusiss-capstone-project/payment-mservice/server/docs"
	"github.com/nusiss-capstone-project/payment-mservice/server/http/api"
	"github.com/nusiss-capstone-project/payment-mservice/server/http/data"
	"github.com/nusiss-capstone-project/payment-mservice/server/log"
	swaggerFiles "github.com/swaggo/files"
	gs "github.com/swaggo/gin-swagger"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

const (
	serviceURIPrefix = "/payment-ms/v1"
)

func NewRouter() *gin.Engine {
	r := gin.New()
	r.Use(log.RecoveryMiddleware())
	r.Use(otelgin.Middleware(data.ServiceName))
	r.Use(log.HTTPResponseIDMiddleware())
	r.Use(corsMiddleware())

	basicGroup := r.Group(serviceURIPrefix)
	{
		// High-frequency / non-business routes: no HTTP access log.
		basicGroup.GET("/ping", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"message": "pong",
			})
		})
		basicGroup.GET("/swagger/*any", gs.WrapHandler(
			swaggerFiles.Handler,
			gs.URL("/payment-ms/v1/swagger/doc.json"),
		))
	}

	// Business routes: enable request access logging.
	apiGroup := basicGroup.Group("")
	apiGroup.Use(log.HTTPObservabilityMiddleware())
	{
		webGroup := apiGroup.Group("/web")
		webGroup.POST("/payment-methods", commonauth.RequireUser(), api.AddPaymentMethod)
		webGroup.GET("/payment-methods", commonauth.RequireUser(), api.ListPaymentMethods)
		webGroup.GET("/transactions/:payment_id", commonauth.RequireUser(), api.GetTransaction)
		apiGroup.POST("/stripe/webhook", api.StripeWebhook)
	}
	return r
}

func corsMiddleware() gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOrigins: allowedOrigins(),
		AllowMethods: []string{
			"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS",
		},
		AllowHeaders: []string{
			"Origin", "Content-Type", "Accept", "Authorization",
			commonauth.HeaderInternalUserID, commonauth.HeaderUserRole,
			log.RequestIDHeader, log.TraceIDHeader,
		},
		ExposeHeaders: []string{
			"Content-Length", commonauth.HeaderInternalUserID, commonauth.HeaderUserRole,
			log.RequestIDHeader, log.TraceIDHeader,
		},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})
}

func allowedOrigins() []string {
	if config.Config == nil || config.Config.SystemConfig == nil {
		return []string{}
	}
	return config.Config.SystemConfig.AllowedOrigins
}
