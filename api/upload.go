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

func (h *APIHandlers) UploadHandler(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file is received"})
		return
	}

	maxUploadMB := service.GetMaxUploadMB()
	maxSizeBytes := int64(maxUploadMB) * 1024 * 1024 // 将 MB 转换为 bytes
	if file.Size > maxSizeBytes {
		errorMsg := fmt.Sprintf("File size exceeds the limit of %dMB", maxUploadMB)
		c.JSON(http.StatusBadRequest, gin.H{"error": errorMsg})
		return
	}

	userID := c.MustGet("userID").(uint)

	var targetBackendIDs []uint
	// 从表单数据中获取 'backends' 字段，它可能是以逗号分隔的ID字符串
	// 或者多次传入 'backends[]' 字段
	backendIDsParam := c.PostFormArray("backends") // 获取所有名为 'backends' 的表单字段
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

	// 修改 UploadImage 函数调用，传入 targetBackendIDs
	image, err := service.UploadImage(file, userID, targetBackendIDs, h.StorageManager)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 构造新的响应格式以匹配前端JS
	var locationsResponse []gin.H
	for _, loc := range image.StorageLocations {
		backendName := loc.StorageType // 默认值
		if loc.Backend.Name != "" {
			backendName = loc.Backend.Name // 如果成功预加载，则使用后端名称
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
			"locations": locationsResponse,                                  // 包含详细的存储位置列表
			"view_url":  fmt.Sprintf("%s/i/%s", c.Request.Host, image.UUID), // 增加一个主访问链接
		},
	})
}

// RedirectHandler 保持不变, 但路由会变
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

	// --- 2. 新增的核心逻辑 ---
	if location.StorageType == "local" {
		// 对于本地文件，我们直接提供服务，而不是重定向
		parsedURL, err := url.Parse(location.URL)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid local file URL"})
			return
		}
		// 从 URL 路径构建本地文件系统路径
		localPath := "." + parsedURL.Path

		// c.File() 会自动处理文件不存在的情况，并返回一个 404 响应。
		// 这就解决了“连接被拒绝”的问题！
		c.File(localPath)
	} else {
		// 对于远程存储，我们像以前一样进行重定向
		c.Redirect(http.StatusFound, location.URL)
	}
}
