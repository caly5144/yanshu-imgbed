package api

import (
	"yanshu-imgbed/database"
	"yanshu-imgbed/manager"
	"yanshu-imgbed/storage"
)

// APIHandlers 结构体持有所有 handler 的依赖
type APIHandlers struct {
	StorageManager *manager.StorageManager
}

// NewAPIHandlers 创建一个新的 APIHandlers 实例
func NewAPIHandlers(sm *manager.StorageManager) *APIHandlers {
	return &APIHandlers{
		StorageManager: sm,
	}
}

func (h *APIHandlers) getFullURL(loc database.StorageLocation) string {
	// 对于非本地存储，直接返回数据库中的URL
	if loc.StorageType != "local" {
		return loc.URL
	}

	// 如果是新数据（相对路径，如 /uploads/file.jpg），则动态拼接
	uploader, found := h.StorageManager.Get(loc.BackendID)
	if !found {
		// 如果找不到后端配置，返回一个提示性的相对路径
		return loc.URL
	}

	localUploader, ok := uploader.(*storage.LocalUploader)
	if !ok {
		return loc.URL
	}

	// 使用当前最新的 PublicURL 配置来拼接
	return localUploader.PublicURL + loc.URL
}
