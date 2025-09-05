package database

import (
	"time"

	"gorm.io/datatypes"
)

// CustomModel 替换 gorm.Model，不包含 DeletedAt
type CustomModel struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// User 用户模型
type User struct {
	CustomModel
	Username  string     `gorm:"type:varchar(50);uniqueIndex;not null"`
	Password  string     `gorm:"type:varchar(255);not null"`      // 存储哈希后的密码
	Role      string     `gorm:"type:varchar(20);default:'user'"` // "admin", "user"
	APITokens []APIToken `gorm:"foreignKey:UserID"`               // 用户拥有的API Token
}

// APIToken API Token 模型
type APIToken struct {
	CustomModel
	UserID    uint
	User      User
	Token     string     `gorm:"type:varchar(255);uniqueIndex;not null"` // 实际的Token值
	Name      string     `gorm:"type:varchar(100)"`                      // Token的名称，方便用户管理
	IsActive  bool       `gorm:"default:true"`                           // 是否启用
	ExpiresAt *time.Time // Token过期时间，可选
}

// Image 主表
type Image struct {
	CustomModel
	UUID             string `gorm:"type:varchar(36);uniqueIndex;not null"`
	MD5              string `gorm:"type:varchar(32);uniqueIndex;not null"`
	OriginalFilename string `gorm:"type:varchar(255)"`
	FileSize         int64
	ContentType      string            `gorm:"type:varchar(50)"`
	Width            int               `gorm:"default:0"`
	Height           int               `gorm:"default:0"`
	AllowRandom      bool              `gorm:"default:false;index"`
	StorageLocations []StorageLocation `gorm:"foreignKey:ImageID"`
	UserID           uint              `gorm:"index"`
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
