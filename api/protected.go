package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
	"yanshu-imgbed/database"
	"yanshu-imgbed/service"

	"github.com/gin-gonic/gin"
)

// ListImagesHandler 列出图片，会根据用户角色和ID进行过滤
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

// ListBackendsHandler 列出所有活跃的存储后端 (供前端显示复选框用)
// 理论上不需要认证，但在我们的系统里，只有认证用户才能使用上传页。
// 所以放在 protectedApiGroup 中比较合理。
func ListBackendsHandler(c *gin.Context) {
	var backends []database.Backend
	// 允许所有认证用户查看活跃的后端列表
	database.DB.Where("allow_upload = ?", true).Order("priority asc").Find(&backends)
	c.JSON(http.StatusOK, backends)
}

// --- GetStatsHandler 也移动到这里，因为它提供的是概览数据，所有用户都能看，但需要认证 ---
func GetStatsHandler(c *gin.Context) {
	userID := c.MustGet("userID").(uint)
	userRole := c.MustGet("userRole").(string)

	var totalImages, totalBackends, todayUploads int64
	var totalSize int64

	// --- NEW: 为 Count 和 Select 创建独立的查询链 ---

	// 查询总图片数
	queryTotalImages := database.DB.Model(&database.Image{})
	if userRole != "admin" {
		queryTotalImages = queryTotalImages.Where("user_id = ?", userID)
	}
	queryTotalImages.Count(&totalImages)

	fmt.Println(userID, totalImages)
	// 查询总占用空间
	queryTotalSize := database.DB.Model(&database.Image{})
	if userRole != "admin" {
		queryTotalSize = queryTotalSize.Where("user_id = ?", userID)
	}
	queryTotalSize.Select("IFNULL(sum(file_size), 0)").Row().Scan(&totalSize)

	// --- END NEW ---

	// 查询总后端数 (保持不变)
	database.DB.Model(&database.Backend{}).Where("1 = ?", 1).Count(&totalBackends)

	// 查询今日上传数 (保持不变，但为了代码一致性，也建议独立查询链)
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

// ListRecentImagesHandler 也移动到这里
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

// GetSettingsHandler 也移动到这里，因为普通用户也可能需要查看某些公开设置
func GetSettingsHandler(c *gin.Context) {
	var settings []database.Setting
	database.DB.Find(&settings)
	settingsMap := make(map[string]string)
	for _, s := range settings {
		settingsMap[s.Key] = s.Value
	}
	c.JSON(http.StatusOK, settingsMap)
}
