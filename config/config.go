package config

import (
	"log"

	"github.com/spf13/viper"
)

// AppConfig 保存了应用的所有配置
type AppConfig struct {
	Server   ServerConfig
	Database DatabaseConfig
	JWT      JWTConfig
}

// ServerConfig 服务器相关配置
type ServerConfig struct {
	Port string
	Mode string
}

// DatabaseConfig 数据库相关配置
type DatabaseConfig struct {
	DSN string // Data Source Name
}

// JWTConfig JWT 相关配置
type JWTConfig struct {
	Secret          string
	ExpirationHours int `mapstructure:"expiration_hours"`
}

// Cfg 是全局可访问的配置实例
var Cfg *AppConfig

// Init 初始化配置，从 config.yml 加载
func Init() error {
	// --- 新增：设置默认配置 ---
	viper.SetDefault("server.port", "3030")
	viper.SetDefault("server.mode", "release")
	viper.SetDefault("database.dsn", "data/image_bed.db")
	viper.SetDefault("jwt.secret", "your-super-secret-key-that-should-be-changed")
	viper.SetDefault("jwt.expiration_hours", 24)
	// --- 默认配置结束 ---

	viper.SetConfigName("config") // 配置文件名 (不带后缀)
	viper.SetConfigType("yml")    // 配置文件类型
	viper.AddConfigPath(".")      // 配置文件路径 (当前目录)

	// --- 修改：优雅地处理文件不存在的错误 ---
	if err := viper.ReadInConfig(); err != nil {
		// 如果错误是“配置文件未找到”，则忽略错误，因为我们将使用默认值
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Println("config.yml not found, using default settings.")
		} else {
			// 如果是其他错误（如文件格式错误），则返回错误
			return err
		}
	}

	Cfg = &AppConfig{}
	if err := viper.Unmarshal(Cfg); err != nil {
		return err
	}

	log.Println("Configuration loaded successfully")
	return nil
}
