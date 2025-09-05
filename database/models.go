package database

import (
	"time"

	"gorm.io/datatypes"
)

// CustomModel 替换 gorm.Model
type CustomModel struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// User 用户模型
type User struct {
	CustomModel
	Username  string     `gorm:"type:varchar(50);uniqueIndex;not null"`
	Password  string     `gorm:"type:varchar(255);not null"`
	Role      string     `gorm:"type:varchar(20);default:'user'"`
	APITokens []APIToken `gorm:"foreignKey:UserID"`
}

// APIToken API Token 模型
type APIToken struct {
	CustomModel
	UserID    uint
	User      User
	Token     string `gorm:"type:varchar(255);uniqueIndex;not null"`
	Name      string `gorm:"type:varchar(100)"`
	IsActive  bool   `gorm:"default:true"`
	ExpiresAt *time.Time
}

// Image 主表
type Image struct {
	CustomModel
	UUID string `gorm:"type:varchar(36);uniqueIndex;not null"`
	// --- 已修改：移除独立唯一索引，改为与UserID的复合唯一索引 ---
	MD5              string `gorm:"type:varchar(32);index:idx_user_md5,unique"`
	OriginalFilename string `gorm:"type:varchar(255)"`
	FileSize         int64
	ContentType      string            `gorm:"type:varchar(50)"`
	Width            int               `gorm:"default:0"`
	Height           int               `gorm:"default:0"`
	StorageLocations []StorageLocation `gorm:"foreignKey:ImageID"`
	// --- 已修改：将 UserID 加入复合唯一索引 ---
	UserID      uint `gorm:"index:idx_user_md5,unique"`
	AllowRandom bool `gorm:"default:false;index"`
}

// StorageLocation 存储位置表
type StorageLocation struct {
	CustomModel
	ImageID          uint
	BackendID        uint
	Backend          Backend `gorm:"foreignKey:BackendID"`
	StorageType      string  `gorm:"type:varchar(50);not null"`
	URL              string  `gorm:"type:varchar(512);not null"`
	DeleteIdentifier string  `gorm:"type:varchar(255)"`
	IsActive         bool    `gorm:"default:true"`
	FailureCount     int     `gorm:"default:0"`
}

// Backend 存储后端配置表
type Backend struct {
	CustomModel
	Name          string         `gorm:"type:varchar(100);not null;unique"`
	Type          string         `gorm:"type:varchar(50);not null"`
	Config        datatypes.JSON `gorm:"type:json"`
	Priority      int            `gorm:"default:1"`
	AllowUpload   bool           `gorm:"default:true"`
	AllowRedirect bool           `gorm:"default:true"`
}

// Setting 系统设置表
type Setting struct {
	CustomModel
	Key   string `gorm:"type:varchar(100);uniqueIndex;not null"`
	Value string `gorm:"type:text"`
}
