package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetVideoRouter(router *gin.Engine) {
	// Video proxy: accepts either session auth (dashboard) or token auth (API clients)
	videoProxyRouter := router.Group("/v1")
	videoProxyRouter.Use(middleware.RouteTag("relay"))
	videoProxyRouter.Use(middleware.TokenOrUserAuth())
	{
		videoProxyRouter.GET("/videos/:task_id/content", controller.VideoProxy)
	}

	videoV1Router := router.Group("/v1")
	videoV1Router.Use(middleware.RouteTag("relay"))
	videoV1Router.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		videoV1Router.POST("/video/generations", controller.RelayTask)
		videoV1Router.GET("/video/generations/:task_id", controller.RelayTaskFetch)
		videoV1Router.POST("/videos/:video_id/remix", controller.RelayTask)
	}
	// openai compatible API video routes
	// docs: https://platform.openai.com/docs/api-reference/videos/create
	{
		videoV1Router.POST("/videos", controller.RelayTask)
		videoV1Router.GET("/videos/:task_id", controller.RelayTaskFetch)
	}

	klingV1Router := router.Group("/kling/v1")
	klingV1Router.Use(middleware.RouteTag("relay"))
	klingV1Router.Use(middleware.KlingRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
	{
		klingV1Router.POST("/videos/text2video", controller.RelayTask)
		klingV1Router.POST("/videos/image2video", controller.RelayTask)
		klingV1Router.GET("/videos/text2video/:task_id", controller.RelayTaskFetch)
		klingV1Router.GET("/videos/image2video/:task_id", controller.RelayTaskFetch)
	}

	// Jimeng official API routes - direct mapping to official API format
	jimengOfficialGroup := router.Group("jimeng")
	jimengOfficialGroup.Use(middleware.RouteTag("relay"))
	jimengOfficialGroup.Use(middleware.JimengRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
	{
		// Maps to: /?Action=CVSync2AsyncSubmitTask&Version=2022-08-31 and /?Action=CVSync2AsyncGetResult&Version=2022-08-31
		jimengOfficialGroup.POST("/", controller.RelayTask)
	}

	// Doubao official API routes
	doubaoOfficialGroup := router.Group("/api/v3")
	doubaoOfficialGroup.Use(middleware.RouteTag("relay"))
	doubaoOfficialGroup.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		doubaoOfficialGroup.POST("/contents/generations/tasks", controller.RelayTask)
		doubaoOfficialGroup.GET("/contents/generations/tasks/:task_id", controller.RelayTaskFetch)
		// doubaoOfficialGroup.GET("/contents/generations/tasks", controller.BatchQueryTasks)
	}

	// Ali DashScope official video API routes
	aliOfficialGroup := router.Group("/api/v1")
	aliOfficialGroup.Use(middleware.RouteTag("relay"))
	aliOfficialGroup.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		aliOfficialGroup.POST("/services/aigc/video-generation/video-synthesis", controller.RelayTask)
		aliOfficialGroup.GET("/tasks/:task_id", controller.RelayTaskFetch)
	}

	// Vidu official API routes
	viduOfficialGroup := router.Group("/ent/v2")
	viduOfficialGroup.Use(middleware.RouteTag("relay"))
	viduOfficialGroup.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		viduOfficialGroup.POST("/text2video", controller.RelayTask)
		viduOfficialGroup.POST("/img2video", controller.RelayTask)
		viduOfficialGroup.POST("/start-end2video", controller.RelayTask)
		viduOfficialGroup.POST("/reference2video", controller.RelayTask)
		viduOfficialGroup.GET("/tasks/:task_id/creations", controller.RelayTaskFetch)
	}
}
