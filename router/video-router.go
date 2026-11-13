package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetVideoRouter(router *gin.Engine) {
	videoSharedRouter := router.Group("/v1")
	videoSharedRouter.Use(middleware.RouteTag("relay"))
	videoSharedRouter.Use(middleware.TokenAuth())
	videoSharedRouter.Use(middleware.SystemPerformanceCheck())
	videoSharedRouter.POST(
		"/video/generations",
		middleware.AdtyRequireCurrentSiteTerms(),
		middleware.PinTaskPluginEndpoint(),
		middleware.TaskPluginEndpointOnly(middleware.ModelRequestRateLimit()),
		middleware.AdtyUserLimit(), middleware.AdtyKeyRateLimit(), // 二开：任务 7.4
		middleware.PrepareTaskPluginEndpoint(),
		middleware.Distribute(),
		func(c *gin.Context) {
			controller.RelayTaskPluginEndpoint(c, controller.RelayTask)
		},
	)

	videoV1Router := router.Group("/v1")
	videoV1Router.Use(middleware.RouteTag("relay"))
	videoV1Router.Use(middleware.TokenAuth())
	{
		videoV1Router.GET("/video/generations/:task_id", middleware.Distribute(), controller.RelayTaskFetch)
		videoV1Router.POST(
			"/videos/:video_id/remix",
			middleware.AdtyRequireCurrentSiteTerms(),
			middleware.SystemPerformanceCheck(),
			middleware.PinTaskPluginEndpoint(),
			middleware.TaskPluginEndpointOnly(middleware.ModelRequestRateLimit()),
			middleware.AdtyUserLimit(),
			middleware.AdtyKeyRateLimit(),
			middleware.PrepareTaskPluginEndpoint(),
			middleware.Distribute(),
			controller.RelayTask,
		)
	}
}
