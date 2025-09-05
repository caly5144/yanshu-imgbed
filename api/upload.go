package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"yanshu-imgbed/service"

	"github.com/gin-gonic/gin"
)

// GetRandomImageRedirectHandler handles requests for a random image.
func GetRandomImageRedirectHandler(c *gin.Context) {
	uuid, err := service.GetRandomImageUUID()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Construct the redirect URL and perform a 302 redirect.
	// This reuses the existing /i/:uuid logic.
	redirectURL := fmt.Sprintf("/i/%s", uuid)
	c.Redirect(http.StatusFound, redirectURL)
}

func (h *APIHandlers) UploadHandler(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file is received"})
		return
	}

	maxUploadMB := service.GetMaxUploadMB()
	maxSizeBytes := int64(maxUploadMB) * 1024 * 1024
	if file.Size > maxSizeBytes {
		errorMsg := fmt.Sprintf("File size exceeds the limit of %dMB", maxUploadMB)
		c.JSON(http.StatusBadRequest, gin.H{"error": errorMsg})
		return
	}

	userID := c.MustGet("userID").(uint)

	var targetBackendIDs []uint
	backendIDsParam := c.PostFormArray("backends")
	if len(backendIDsParam) > 0 {
		for _, idStr := range backendIDsParam {
			id, parseErr := strconv.ParseUint(idStr, 10, 32)
			if parseErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid backend ID: %s", idStr)})
				return
			}
			targetBackendIDs = append(targetBackendIDs, uint(id))
		}
	}

	image, err := service.UploadImage(file, userID, targetBackendIDs, h.StorageManager)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var locationsResponse []gin.H
	for _, loc := range image.StorageLocations {
		backendName := loc.StorageType
		if loc.Backend.Name != "" {
			backendName = loc.Backend.Name
		}
		locationsResponse = append(locationsResponse, gin.H{
			"id":           loc.ID,
			"backend_name": backendName,
			"url":          loc.URL,
			"is_active":    loc.IsActive,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"hash":      image.UUID,
			"filename":  image.OriginalFilename,
			"size":      image.FileSize,
			"locations": locationsResponse,
			"view_url":  fmt.Sprintf("%s/i/%s", c.Request.Host, image.UUID),
		},
	})
}

func ServeImageHandler(c *gin.Context) {
	uuid := c.Param("uuid")
	location, err := service.GetHealthyStorageLocation(uuid)

	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		}
		return
	}

	if location.StorageType == "local" {
		parsedURL, err := url.Parse(location.URL)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid local file URL"})
			return
		}
		localPath := "." + parsedURL.Path
		c.File(localPath)
	} else {
		c.Redirect(http.StatusFound, location.URL)
	}
}
