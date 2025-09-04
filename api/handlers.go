package api

import "yanshu-imgbed/manager"

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
