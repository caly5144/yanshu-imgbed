package manager

import (
	"encoding/json"
	"log"
	"sync"
	"yanshu-imgbed/database"
	"yanshu-imgbed/storage"
)

// StorageManager 负责管理所有存储后端 Uploader 实例
type StorageManager struct {
	uploaders map[uint]storage.Uploader // key 是 backend.ID
	mu        sync.RWMutex
}

// NewStorageManager 创建并初始化一个新的 StorageManager
func NewStorageManager() (*StorageManager, error) {
	sm := &StorageManager{
		uploaders: make(map[uint]storage.Uploader),
	}
	if err := sm.Refresh(); err != nil {
		return nil, err
	}
	return sm, nil
}

// Get 根据后端 ID 获取一个 Uploader 实例
func (sm *StorageManager) Get(backendID uint) (storage.Uploader, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	uploader, found := sm.uploaders[backendID]
	return uploader, found
}

// GetAllActive 返回所有活跃的 Uploader 实例
func (sm *StorageManager) GetAllActive() []storage.Uploader {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	activeUploaders := make([]storage.Uploader, 0)
	var activeBackends []database.Backend
	database.DB.Where("allow_upload = ?", true).Find(&activeBackends)

	for _, backend := range activeBackends {
		if uploader, ok := sm.uploaders[backend.ID]; ok {
			activeUploaders = append(activeUploaders, uploader)
		}
	}
	return activeUploaders
}

// Refresh 重新从数据库加载所有后端配置并更新 Uploader 实例
func (sm *StorageManager) Refresh() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var backends []database.Backend
	if err := database.DB.Find(&backends).Error; err != nil {
		return err
	}

	newUploaders := make(map[uint]storage.Uploader)
	for _, backend := range backends {
		var uploader storage.Uploader
		var configMap map[string]string

		if err := json.Unmarshal(backend.Config, &configMap); err != nil {
			log.Printf("Error parsing config for backend %s (ID: %d): %v. Skipping.", backend.Name, backend.ID, err)
			continue
		}

		switch backend.Type {
		case "local":
			uploader = storage.NewLocalUploader(configMap["storagePath"], configMap["publicUrl"])
		case "sm.ms":
			uploader = storage.NewSmmsUploader(configMap["baseURL"], configMap["token"])
		case "oss":
			var err error
			uploader, err = storage.NewOssUploader(configMap)
			if err != nil {
				log.Printf("Error initializing OSS backend %s (ID: %d): %v. Skipping.", backend.Name, backend.ID, err)
				continue
			}
		// 在此添加其他存储类型的初始化逻辑
		default:
			log.Printf("Unsupported backend type: %s for backend %s (ID: %d). Skipping.", backend.Type, backend.Name, backend.ID)
			continue
		}
		newUploaders[backend.ID] = uploader
	}

	sm.uploaders = newUploaders
	log.Printf("Storage manager refreshed. Loaded %d uploader(s).", len(sm.uploaders))
	return nil
}
