package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"yanshu-imgbed/database"
	"yanshu-imgbed/service"
	"yanshu-imgbed/storage"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ToggleImageRandomStatusHandler 切换图片随机状态的处理器
func ToggleImageRandomStatusHandler(c *gin.Context) {
	uuid := c.Param("uuid")
	image, err := service.ToggleImageRandomStatus(uuid)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Image not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update image status"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message":      "Random status updated successfully",
		"uuid":         image.UUID,
		"allow_random": image.AllowRandom,
	})
}

type BatchImageRequest struct {
	Action     string   `json:"action" binding:"required"`
	ImageUUIDs []string `json:"image_uuids" binding:"required"`
	BackendID  uint     `json:"backend_id"` // Optional, for backend-specific actions
}

func (h *APIHandlers) BatchImageHandler(c *gin.Context) {
	var req BatchImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := c.MustGet("userID").(uint)
	userRole := c.MustGet("userRole").(string)
	var taskID string
	var err error

	switch req.Action {
	case "delete":
		taskID, err = service.BatchDeleteImages(req.ImageUUIDs, userID, userRole, h.StorageManager)
	case "backfill":
		if req.BackendID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "backend_id is required for backfill action"})
			return
		}
		taskID, err = service.BatchBackfillToBackend(req.ImageUUIDs, req.BackendID, h.StorageManager)
	// --- 新增：处理批量设置随机图库状态 ---
	case "add_to_random":
		err = service.BatchSetRandomStatus(req.ImageUUIDs, true)
	case "remove_from_random":
		err = service.BatchSetRandomStatus(req.ImageUUIDs, false)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid action"})
		return
	}

	// --- 修改：为非任务型操作提供即时响应 ---
	if req.Action == "add_to_random" || req.Action == "remove_from_random" {
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Batch random status updated successfully"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Batch task started", "task_id": taskID})
}

func ListTasksHandler(c *gin.Context) {
	tasks := service.GetTasks()
	c.JSON(http.StatusOK, tasks)
}

// DeleteImageHandler 现在是 APIHandlers 的一个方法，可以访问 StorageManager
func (h *APIHandlers) DeleteImageHandler(c *gin.Context) {
	uuid := c.Param("uuid")
	userID := c.MustGet("userID").(uint)
	userRole := c.MustGet("userRole").(string)

	// 将 h.StorageManager 作为第四个参数传入
	if err := service.DeleteImage(uuid, userID, userRole, h.StorageManager); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Image deleted successfully"})
}

// --- 其他 Admin Handlers ---

// CreateBackendHandler ...
func (h *APIHandlers) CreateBackendHandler(c *gin.Context) {
	var backend database.Backend
	if err := c.ShouldBindJSON(&backend); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !json.Valid(backend.Config) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON in config field"})
		return
	}
	if err := database.DB.Create(&backend).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create backend"})
		return
	}
	go h.StorageManager.Refresh()
	c.JSON(http.StatusOK, backend)
}

func (h *APIHandlers) ToggleBackendFlagHandler(c *gin.Context) {
	idStr := c.Param("id")
	flag := c.Param("flag") // "upload" or "redirect"
	id, _ := strconv.Atoi(idStr)

	var backend database.Backend
	if err := database.DB.First(&backend, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Backend not found"})
		return
	}

	switch flag {
	case "upload":
		backend.AllowUpload = !backend.AllowUpload
	case "redirect":
		backend.AllowRedirect = !backend.AllowRedirect
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid flag specified"})
		return
	}

	// --- 关键修复：添加数据库保存操作的错误处理 ---
	if err := database.DB.Save(&backend).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update backend status"})
		return
	}
	// --- 修复结束 ---

	go h.StorageManager.Refresh()
	c.JSON(http.StatusOK, backend) // 返回更新后的后端对象，使前端可以验证
}

func SaveSettingsHandler(c *gin.Context) {
	var newSettings map[string]string
	if err := c.ShouldBindJSON(&newSettings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for key, value := range newSettings {
		database.DB.Model(&database.Setting{}).Where("key = ?", key).Update("value", value)
	}

	// --- 2. 新增：更新内存中的设置缓存 ---
	// 使用 goroutine 异步更新，避免阻塞当前请求
	go func() {
		if err := service.UpdateSettingsCache(); err != nil {
			log.Printf("Error updating settings cache: %v", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"message": "Settings saved successfully"})
}

// ListAllBackendsHandler (不需要 manager)
func ListAllBackendsHandler(c *gin.Context) {
	var backends []database.Backend
	database.DB.Order("priority asc").Find(&backends)
	c.JSON(http.StatusOK, backends)
}

// UpdateBackendHandler ...
func (h *APIHandlers) UpdateBackendHandler(c *gin.Context) {
	backendID, _ := strconv.Atoi(c.Param("id"))
	var req database.Backend
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !json.Valid(req.Config) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON in config field"})
		return
	}
	var backend database.Backend
	if err := database.DB.First(&backend, backendID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Backend not found"})
		return
	}
	backend.Name = req.Name
	backend.Type = req.Type
	backend.Config = req.Config
	backend.Priority = req.Priority
	if err := database.DB.Save(&backend).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update backend"})
		return
	}
	go h.StorageManager.Refresh()
	c.JSON(http.StatusOK, backend)
}

// DeleteBackendHandler ...
func (h *APIHandlers) DeleteBackendHandler(c *gin.Context) {
	backendID, _ := strconv.Atoi(c.Param("id"))
	var count int64
	database.DB.Model(&database.StorageLocation{}).Where("backend_id = ?", backendID).Count(&count)
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法删除：此后端仍有图片存储位置关联。"})
		return
	}
	if err := database.DB.Delete(&database.Backend{}, backendID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete backend"})
		return
	}
	go h.StorageManager.Refresh()
	c.JSON(http.StatusOK, gin.H{"message": "后端删除成功"})
}

// ValidateSmmsTokenHandler (不需要 manager)
func ValidateSmmsTokenHandler(c *gin.Context) {
	var req struct {
		BaseURL string `json:"baseURL" binding:"required"`
		Token   string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uploader := storage.NewSmmsUploader(req.BaseURL, req.Token)
	if err := uploader.CheckToken(); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": fmt.Sprintf("Token验证失败: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Token验证成功"})
}

// --- 新增：获取单张图片详细信息的处理器 ---
func GetImageDetailsHandler(c *gin.Context) {
	uuid := c.Param("uuid")
	var image database.Image
	err := database.DB.Preload("StorageLocations.Backend").Where("uuid = ?", uuid).First(&image).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Image not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve image details"})
		return
	}
	c.JSON(http.StatusOK, image)
}

// --- 新增：切换 StorageLocation IsActive 状态的处理器 ---
func ToggleStorageLocationStatusHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var loc database.StorageLocation
	if err := database.DB.First(&loc, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Storage location not found"})
		return
	}

	loc.IsActive = !loc.IsActive
	if err := database.DB.Save(&loc).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update status"})
		return
	}
	c.JSON(http.StatusOK, loc)
}
