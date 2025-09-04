package router

import (
	"log"
	"net/http"
	"yanshu-imgbed/api"
	"yanshu-imgbed/config"
	"yanshu-imgbed/database"
	"yanshu-imgbed/manager"
	"yanshu-imgbed/middleware"
	"yanshu-imgbed/service"

	"github.com/gin-gonic/gin"
)

func SetupRouter(storageManager *manager.StorageManager) *gin.Engine {

	if config.Cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
		log.Println("Running in release mode")
	} else {
		gin.SetMode(gin.DebugMode)
		log.Println("Running in debug mode")
	}
	r := gin.Default()

	r.SetTrustedProxies([]string{"127.0.0.1", "::1"})
	apiHandlers := api.NewAPIHandlers(storageManager)

	r.LoadHTMLGlob("templates/*")
	r.Static("/static", "./static")
	r.Static("/uploads", "./uploads")

	// --- 页面路由 ---
	// 页面路由应该是公开的，让前端JS来处理认证状态
	r.GET("/login", func(c *gin.Context) {
		c.HTML(http.StatusOK, "login.html", nil)
	})
	r.GET("/", func(c *gin.Context) {
		var backends []database.Backend
		// 直接在这里查询数据
		database.DB.Where("allow_upload = ?", true).Order("priority asc").Find(&backends)
		maxUploadMB := service.GetMaxUploadMB()

		// 将查询到的数据传递给模板
		c.HTML(http.StatusOK, "index.html", gin.H{
			"Backends":    backends,
			"MaxUploadMB": maxUploadMB,
		})
	})

	r.GET("/admin", func(c *gin.Context) {
		c.HTML(http.StatusOK, "admin.html", nil)
	})
	r.GET("/admin/images/:uuid", func(c *gin.Context) {
		c.HTML(http.StatusOK, "image_details.html", nil)
	})

	// --- 认证相关公共路由 (不需要登录即可访问) ---
	authGroup := r.Group("/auth")
	{
		authGroup.POST("/login", api.LoginHandler)
	}

	// --- 图片访问路由 (公开) ---
	r.GET("/i/:uuid", api.ServeImageHandler)

	// --- API 路由 (这里才是认证保护的重点) ---
	r.POST("/api/upload/web", middleware.AuthMiddleware(), apiHandlers.UploadHandler)
	r.POST("/api/upload/api", middleware.APITokenAuthMiddleware(), apiHandlers.UploadHandler)

	// 需要JWT Token的API (用户自己的操作)
	protectedApiGroup := r.Group("/api", middleware.AuthMiddleware())
	{
		protectedApiGroup.GET("/user/info", api.GetUserInfoHandler)
		protectedApiGroup.POST("/user/change-password", api.ChangeMyPasswordHandler)
		protectedApiGroup.GET("/user/tokens", api.ListAPITokensHandler)
		protectedApiGroup.POST("/user/tokens", api.CreateAPITokenHandler)
		protectedApiGroup.POST("/user/tokens/:id/toggle", api.ToggleAPITokenStatusHandler)
		protectedApiGroup.DELETE("/user/tokens/:id", api.DeleteAPITokenHandler)
		protectedApiGroup.GET("/stats", api.GetStatsHandler)
		protectedApiGroup.GET("/images/recent", api.ListRecentImagesHandler)
		protectedApiGroup.GET("/images", api.ListImagesHandler)
		protectedApiGroup.DELETE("/images/:uuid", apiHandlers.DeleteImageHandler)
		protectedApiGroup.GET("/backends", api.ListBackendsHandler)
		protectedApiGroup.GET("/settings", api.GetSettingsHandler)
	}

	// 后台管理 API (需要JWT Token + 管理员权限)
	adminApiGroup := r.Group("/api/admin", middleware.AuthMiddleware(), middleware.AdminAuthMiddleware())
	{
		adminApiGroup.GET("/backends/all", api.ListAllBackendsHandler)
		adminApiGroup.POST("/backends", apiHandlers.CreateBackendHandler)
		adminApiGroup.PUT("/backends/:id", apiHandlers.UpdateBackendHandler)
		adminApiGroup.DELETE("/backends/:id", apiHandlers.DeleteBackendHandler)
		adminApiGroup.POST("/backends/:id/toggle/:flag", apiHandlers.ToggleBackendFlagHandler) // <<<--- 新增此行
		adminApiGroup.POST("/backends/smms/validate-token", api.ValidateSmmsTokenHandler)
		adminApiGroup.POST("/settings", api.SaveSettingsHandler)
		adminApiGroup.GET("/users", api.ListUsersHandler)
		adminApiGroup.POST("/users", api.RegisterUserHandler)
		adminApiGroup.POST("/users/:id/reset-password", api.ResetPasswordHandler)
		adminApiGroup.DELETE("/users/:id", api.DeleteUserHandler)
		adminApiGroup.POST("/images/batch", apiHandlers.BatchImageHandler) // NEW
		adminApiGroup.GET("/tasks", api.ListTasksHandler)                  // NEW
		// --- 新增：获取图片详情和切换状态的API ---
		adminApiGroup.GET("/images/:uuid", api.GetImageDetailsHandler)
		adminApiGroup.POST("/storagelocations/:id/toggle", api.ToggleStorageLocationStatusHandler)
	}

	return r
}
