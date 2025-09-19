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

// BatchAdminImageRequest defines the structure for admin-level batch operations.
type BatchAdminImageRequest struct {
	Action     string   `json:"action" binding:"required"`
	ImageUUIDs []string `json:"image_uuids" binding:"required"`
	BackendID  uint     `json:"backend_id"` // Optional, for backfill
}

// BatchAdminImageHandler handles batch operations initiated by admins.
func (h *APIHandlers) BatchAdminImageHandler(c *gin.Context) {
	var req BatchAdminImageRequest
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
	case "add_to_random":
		err = service.BatchSetRandomStatus(req.ImageUUIDs, true)
	case "remove_from_random":
		err = service.BatchSetRandomStatus(req.ImageUUIDs, false)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid action"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if taskID != "" {
		c.JSON(http.StatusOK, gin.H{"message": "Batch task started", "task_id": taskID})
	} else {
		c.JSON(http.StatusOK, gin.H{"message": "Batch operation completed successfully"})
	}
}

func ListTasksHandler(c *gin.Context) {
	tasks := service.GetTasks()
	c.JSON(http.StatusOK, tasks)
}

// DeleteImageHandler is a method of APIHandlers to access the StorageManager
func (h *APIHandlers) DeleteImageHandler(c *gin.Context) {
	uuid := c.Param("uuid")
	userID := c.MustGet("userID").(uint)
	userRole := c.MustGet("userRole").(string)

	if err := service.DeleteImage(uuid, userID, userRole, h.StorageManager); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Image deleted successfully"})
}

// ToggleImageRandomStatusHandler toggles the random status for an image.
func ToggleImageRandomStatusHandler(c *gin.Context) {
	uuid := c.Param("uuid")
	image, err := service.ToggleImageRandomStatus(uuid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, image)
}

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

	if err := database.DB.Save(&backend).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update backend status"})
		return
	}

	go h.StorageManager.Refresh()
	c.JSON(http.StatusOK, backend)
}

func SaveSettingsHandler(c *gin.Context) {
	var newSettings map[string]string
	if err := c.ShouldBindJSON(&newSettings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for key, value := range newSettings {
		// Use transaction for multiple updates? For now, this is fine.
		setting := database.Setting{Key: key, Value: value}
		if err := database.DB.Where("key = ?", key).First(&database.Setting{}).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				database.DB.Create(&setting)
			}
		} else {
			database.DB.Model(&database.Setting{}).Where("key = ?", key).Update("value", value)
		}
	}

	go func() {
		if err := service.UpdateSettingsCache(); err != nil {
			log.Printf("Error updating settings cache: %v", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"message": "Settings saved successfully"})
}

// ListAllBackendsHandler (no manager needed)
func ListAllBackendsHandler(c *gin.Context) {
	var backends []database.Backend
	database.DB.Order("priority asc").Find(&backends)
	c.JSON(http.StatusOK, backends)
}

func (h *APIHandlers) UpdateBackendHandler(c *gin.Context) {
	backendID, _ := strconv.Atoi(c.Param("id"))

	var existingBackend database.Backend
	if err := database.DB.First(&existingBackend, backendID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Backend not found"})
		return
	}

	var req database.Backend
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !json.Valid(req.Config) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON in config field"})
		return
	}

	// 如果是本地存储，则强制保留原始的 storagePath
	if existingBackend.Type == "local" {
		var existingConfig map[string]interface{}
		if err := json.Unmarshal(existingBackend.Config, &existingConfig); err == nil {
			var newConfig map[string]interface{}
			if err := json.Unmarshal(req.Config, &newConfig); err == nil {
				// 强制使用旧的 storagePath
				if oldPath, ok := existingConfig["storagePath"]; ok {
					newConfig["storagePath"] = oldPath
				}
				// 将更新后的配置重新序列化
				updatedConfigJSON, _ := json.Marshal(newConfig)
				req.Config = updatedConfigJSON
			}
		}
	}

	// 更新字段
	existingBackend.Name = req.Name
	// 不允许修改类型
	// existingBackend.Type = req.Type
	existingBackend.Config = req.Config
	existingBackend.Priority = req.Priority

	if err := database.DB.Save(&existingBackend).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update backend"})
		return
	}
	go h.StorageManager.Refresh()
	c.JSON(http.StatusOK, existingBackend)
}

// DeleteBackendHandler ...
func (h *APIHandlers) DeleteBackendHandler(c *gin.Context) {
	backendID, _ := strconv.Atoi(c.Param("id"))
	var count int64
	database.DB.Model(&database.StorageLocation{}).Where("backend_id = ?", backendID).Count(&count)
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete backend: still associated with stored images."})
		return
	}
	if err := database.DB.Delete(&database.Backend{}, backendID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete backend"})
		return
	}
	go h.StorageManager.Refresh()
	c.JSON(http.StatusOK, gin.H{"message": "Backend deleted successfully"})
}

// ValidateSmmsTokenHandler (no manager needed)
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
		c.JSON(http.StatusOK, gin.H{"success": false, "message": fmt.Sprintf("Token validation failed: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Token validation successful"})
}

// GetImageDetailsHandler gets details for a single image.
func (h *APIHandlers) GetImageDetailsHandler(c *gin.Context) {
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
	type StorageLocationResponse struct {
		database.StorageLocation
		URL string `json:"URL"` // 覆盖原始URL字段
	}

	type ImageDetailResponse struct {
		database.Image
		StorageLocations []StorageLocationResponse `json:"StorageLocations"`
	}

	response := ImageDetailResponse{Image: image}
	for _, loc := range image.StorageLocations {
		response.StorageLocations = append(response.StorageLocations, StorageLocationResponse{
			StorageLocation: loc,
			URL:             h.getFullURL(loc), // 使用辅助方法
		})
	}

	c.JSON(http.StatusOK, response)
}

// ToggleStorageLocationStatusHandler toggles the IsActive status of a StorageLocation.
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
