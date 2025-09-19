package router

import (
	"embed"
	"html/template"
	"io/fs"
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

func SetupRouter(storageManager *manager.StorageManager, templatesFS embed.FS, staticFS embed.FS) *gin.Engine {

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

	// Load templates and static files from embedded FS
	templ := template.Must(template.ParseFS(templatesFS, "templates/*.html"))
	r.SetHTMLTemplate(templ)
	subStaticFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("Failed to create sub filesystem for static files: %v", err)
	}
	r.StaticFS("/static", http.FS(subStaticFS))

	r.Static("/uploads", "./uploads")

	// Page routes
	r.GET("/login", func(c *gin.Context) { c.HTML(http.StatusOK, "login.html", nil) })
	r.GET("/", func(c *gin.Context) {
		var backends []database.Backend
		database.DB.Where("allow_upload = ?", true).Order("priority asc").Find(&backends)
		maxUploadMB := service.GetMaxUploadMB()
		c.HTML(http.StatusOK, "index.html", gin.H{
			"Backends":    backends,
			"MaxUploadMB": maxUploadMB,
		})
	})
	r.GET("/admin", func(c *gin.Context) { c.HTML(http.StatusOK, "admin.html", nil) })
	r.GET("/admin/images/:uuid", func(c *gin.Context) { c.HTML(http.StatusOK, "image_details.html", nil) })

	// Public routes
	authGroup := r.Group("/auth")
	{
		authGroup.POST("/login", api.LoginHandler)
	}
	r.GET("/i/:uuid", api.ServeImageHandler)
	r.GET("/api/random", api.GetRandomImageRedirectHandler) // Random image API

	// API routes requiring JWT Token (user and admin)
	protectedApiGroup := r.Group("/api", middleware.AuthMiddleware())
	{
		protectedApiGroup.POST("/upload/web", apiHandlers.UploadHandler)
		protectedApiGroup.POST("/images/batch", apiHandlers.BatchUserImageHandler) // NEW: User batch endpoint

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

	// API route for API token uploads
	r.POST("/api/upload/api", middleware.APITokenAuthMiddleware(), apiHandlers.UploadHandler)

	// Admin-only API routes
	adminApiGroup := r.Group("/api/admin", middleware.AuthMiddleware(), middleware.AdminAuthMiddleware())
	{
		adminApiGroup.GET("/backends/all", api.ListAllBackendsHandler)
		adminApiGroup.POST("/backends", apiHandlers.CreateBackendHandler)
		adminApiGroup.PUT("/backends/:id", apiHandlers.UpdateBackendHandler)
		adminApiGroup.DELETE("/backends/:id", apiHandlers.DeleteBackendHandler)
		adminApiGroup.POST("/backends/:id/toggle/:flag", apiHandlers.ToggleBackendFlagHandler)
		adminApiGroup.POST("/backends/smms/validate-token", api.ValidateSmmsTokenHandler)

		adminApiGroup.POST("/settings", api.SaveSettingsHandler)

		adminApiGroup.GET("/users", api.ListUsersHandler)
		adminApiGroup.POST("/users", api.RegisterUserHandler)
		adminApiGroup.POST("/users/:id/reset-password", api.ResetPasswordHandler)
		adminApiGroup.DELETE("/users/:id", api.DeleteUserHandler)

		adminApiGroup.POST("/images/batch", apiHandlers.BatchAdminImageHandler) // Renamed from BatchImageHandler
		adminApiGroup.POST("/images/:uuid/toggle-random", api.ToggleImageRandomStatusHandler)
		adminApiGroup.GET("/tasks", api.ListTasksHandler)
		adminApiGroup.GET("/images/:uuid", apiHandlers.GetImageDetailsHandler)
		adminApiGroup.POST("/storagelocations/:id/toggle", api.ToggleStorageLocationStatusHandler)
	}

	return r
}
