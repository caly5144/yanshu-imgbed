package api

import (
	"net/http"
	"strconv"
	"time"
	"yanshu-imgbed/database"
	"yanshu-imgbed/service"

	"github.com/gin-gonic/gin"
)

// BatchUserImageRequest defines the structure for user-level batch operations.
type BatchUserImageRequest struct {
	Action     string   `json:"action" binding:"required"`
	ImageUUIDs []string `json:"image_uuids" binding:"required"`
	BackendID  uint     `json:"backend_id"` // For backfill
}

// BatchUserImageHandler handles batch operations initiated by non-admin users.
func (h *APIHandlers) BatchUserImageHandler(c *gin.Context) {
	var req BatchUserImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := c.MustGet("userID").(uint)

	switch req.Action {
	case "backfill":
		if req.BackendID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "backend_id is required for backfill action"})
			return
		}
		taskID, err := service.BatchBackfillImagesForUser(req.ImageUUIDs, req.BackendID, userID, h.StorageManager)
		if err != nil {
			// This could be a permission error or other internal error.
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Batch backfill task started for your images", "task_id": taskID})
	// Add other user-level batch actions here in the future if needed.
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid action for user"})
		return
	}
}

// ListImagesHandler lists images, filtered by user role.
func ListImagesHandler(c *gin.Context) {
	userID := c.MustGet("userID").(uint)
	userRole := c.MustGet("userRole").(string)

	keyword := c.Query("keyword")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))

	response, err := service.ListImages(userID, userRole, keyword, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list images"})
		return
	}
	c.JSON(http.StatusOK, response)
}

// ListBackendsHandler lists all active storage backends for the upload form.
func ListBackendsHandler(c *gin.Context) {
	var backends []database.Backend
	database.DB.Where("allow_upload = ?", true).Order("priority asc").Find(&backends)
	c.JSON(http.StatusOK, backends)
}

// GetStatsHandler provides overview statistics, filtered by user role.
func GetStatsHandler(c *gin.Context) {
	userID := c.MustGet("userID").(uint)
	userRole := c.MustGet("userRole").(string)

	var totalImages, totalBackends, todayUploads int64
	var totalSize int64

	queryTotalImages := database.DB.Model(&database.Image{})
	if userRole != "admin" {
		queryTotalImages = queryTotalImages.Where("user_id = ?", userID)
	}
	queryTotalImages.Count(&totalImages)

	queryTotalSize := database.DB.Model(&database.Image{})
	if userRole != "admin" {
		queryTotalSize = queryTotalSize.Where("user_id = ?", userID)
	}
	queryTotalSize.Select("IFNULL(sum(file_size), 0)").Row().Scan(&totalSize)

	// Total backends is a global stat
	database.DB.Model(&database.Backend{}).Count(&totalBackends)

	today := time.Now().Format("2006-01-02")
	queryTodayUploads := database.DB.Model(&database.Image{})
	if userRole != "admin" {
		queryTodayUploads = queryTodayUploads.Where("user_id = ?", userID)
	}
	queryTodayUploads.Where("DATE(created_at) = ?", today).Count(&todayUploads)

	c.JSON(http.StatusOK, gin.H{
		"totalImages":   totalImages,
		"totalSize":     totalSize,
		"totalBackends": totalBackends,
		"todayUploads":  todayUploads,
	})
}

// ListRecentImagesHandler lists recent images, filtered by user role.
func ListRecentImagesHandler(c *gin.Context) {
	userID := c.MustGet("userID").(uint)
	userRole := c.MustGet("userRole").(string)

	var recentImages []database.Image
	query := database.DB.Order("created_at desc").Limit(8)
	if userRole != "admin" {
		query = query.Where("user_id = ?", userID)
	}
	query.Find(&recentImages)
	c.JSON(http.StatusOK, recentImages)
}

// GetSettingsHandler gets public settings.
func GetSettingsHandler(c *gin.Context) {
	var settings []database.Setting
	database.DB.Find(&settings)
	settingsMap := make(map[string]string)
	for _, s := range settings {
		settingsMap[s.Key] = s.Value
	}
	c.JSON(http.StatusOK, settingsMap)
}
