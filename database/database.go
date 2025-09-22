package database

import (
	"log"
	"os"
	"path/filepath"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Init(dsn string) error {
	dbDir := filepath.Dir(dsn)
	if dbDir != "" {
		// os.MkdirAll会创建路径中的所有目录，如果目录已存在则什么也不做
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			log.Printf("Failed to create data directory '%s': %v", dbDir, err)
			return err
		}
	}
	var err error
	DB, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return err
	}

	err = DB.AutoMigrate(&Image{}, &StorageLocation{}, &Backend{}, &Setting{}, &User{}, &APIToken{})
	if err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	initDefaultData()

	return nil
}

func initDefaultData() {
	// 检查是否已有本地后端
	var count int64
	DB.Model(&Backend{}).Where("type = ?", "local").Count(&count)
	if count == 0 {
		log.Println("Initializing default local backend...")
		localBackend := Backend{
			Name:          "本地存储",
			Type:          "local",
			Config:        datatypes.JSON(`{"storagePath": "uploads", "publicUrl": "http://127.0.0.1:3030"}`),
			Priority:      1,
			AllowUpload:   true,
			AllowRedirect: true,
		}
		DB.Create(&localBackend)
	}

	// 检查是否已有设置
	DB.Model(&Setting{}).Count(&count)
	if count == 0 {
		log.Println("Initializing default settings...")
		settings := []Setting{
			{Key: "access_policy", Value: "random"},
			{Key: "retry_count", Value: "0"},
			{Key: "max_upload_mb", Value: "10"},
		}
		DB.Create(&settings)
	}

	// --- 新增：初始化默认管理员用户 ---
	var userCount int64
	DB.Model(&User{}).Count(&userCount)
	if userCount == 0 {
		log.Println("Initializing default admin user...")
		// 默认密码，生产环境应该要求用户首次登录修改或随机生成
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		adminUser := User{
			Username: "admin",
			Password: string(hashedPassword),
			Role:     "admin",
		}
		DB.Create(&adminUser)
	}

}
