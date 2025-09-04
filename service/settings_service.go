package service

import (
	"log"
	"strconv"
	"sync"
	"yanshu-imgbed/database"
)

// SettingsCache 用于在内存中缓存系统设置
type SettingsCache struct {
	RetryCount   int
	AccessPolicy string
	MaxUploadMB  int
}

var (
	// AppSettings 是全局可访问的设置缓存实例
	AppSettings *SettingsCache
	settingsMu  sync.RWMutex
)

// InitSettings 在程序启动时从数据库加载设置到内存
func InitSettings() {
	settingsMu.Lock()
	defer settingsMu.Unlock()

	AppSettings = &SettingsCache{
		RetryCount:   3, // 默认值
		AccessPolicy: "random",
		MaxUploadMB:  10,
	}

	if err := reloadSettings(); err != nil {
		log.Printf("Failed to initialize settings from database, using defaults: %v", err)
	} else {
		log.Println("Settings loaded into memory cache successfully.")
	}
}

// reloadSettings 是实际从数据库重新加载配置的内部函数
func reloadSettings() error {
	var settings []database.Setting
	if err := database.DB.Find(&settings).Error; err != nil {
		return err
	}

	settingsMap := make(map[string]string)
	for _, s := range settings {
		settingsMap[s.Key] = s.Value
	}

	if rcStr, ok := settingsMap["retry_count"]; ok {
		if rcInt, err := strconv.Atoi(rcStr); err == nil {
			AppSettings.RetryCount = rcInt
		}
	}
	if apStr, ok := settingsMap["access_policy"]; ok && (apStr == "random" || apStr == "priority") {
		AppSettings.AccessPolicy = apStr
	}
	if muStr, ok := settingsMap["max_upload_mb"]; ok {
		if muInt, err := strconv.Atoi(muStr); err == nil {
			AppSettings.MaxUploadMB = muInt
		}
	}
	// 在此可以加载其他设置

	return nil
}

// UpdateSettingsCache 用于在管理员更新设置后刷新内存缓存
func UpdateSettingsCache() error {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	log.Println("Updating settings cache from database...")
	return reloadSettings()
}

// GetRetryCount 从内存缓存中安全地获取重试次数
func GetRetryCount() int {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	if AppSettings == nil {
		return 3 // 如果缓存未初始化，返回一个安全的默认值
	}
	return AppSettings.RetryCount
}

// --- 5. 新增: GetAccessPolicy 从内存缓存中安全地获取访问策略 ---
func GetAccessPolicy() string {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	if AppSettings == nil {
		return "random" // 默认返回 "random"
	}
	return AppSettings.AccessPolicy
}

// --- 新增：GetMaxUploadMB 从内存缓存中安全地获取最大上传大小 ---
func GetMaxUploadMB() int {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	if AppSettings == nil {
		return 10 // 如果缓存未初始化，返回一个安全的默认值
	}
	return AppSettings.MaxUploadMB
}
