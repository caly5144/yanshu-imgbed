package service

import (
	"errors"
	"fmt"
	"time"
	"yanshu-imgbed/config"
	"yanshu-imgbed/database"

	"github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Claims 定义JWT载荷
type Claims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.StandardClaims
}

// Login 处理用户登录，返回JWT Token
func Login(username, password string) (string, error) {
	var user database.User
	if err := database.DB.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errors.New("用户名或密码错误")
		}
		return "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return "", errors.New("用户名或密码错误")
	}

	// 使用配置生成JWT Token
	expirationTime := time.Now().Add(time.Duration(config.Cfg.JWT.ExpirationHours) * time.Hour) // 使用配置的过期时间
	claims := &Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expirationTime.Unix(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(config.Cfg.JWT.Secret)) // 使用配置的密钥
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// RegisterUser 注册新用户
func RegisterUser(username, password string, role string) (*database.User, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := database.User{
		Username: username,
		Password: string(hashedPassword),
		Role:     role,
	}

	if err := database.DB.Create(&user).Error; err != nil {
		return nil, fmt.Errorf("注册用户失败: %w", err)
	}
	return &user, nil
}

// ChangePassword 修改用户密码
func ChangePassword(userID uint, oldPassword, newPassword string) error {
	var user database.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return errors.New("用户不存在")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(oldPassword)); err != nil {
		return errors.New("旧密码不正确")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.Password = string(hashedPassword)
	return database.DB.Save(&user).Error
}

// ResetUserPassword 重置用户密码 (管理员权限)
func ResetUserPassword(userID uint, newPassword string) error {
	var user database.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return errors.New("用户不存在")
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.Password = string(hashedPassword)
	return database.DB.Save(&user).Error
}

// DeleteUser 删除用户 (管理员权限)
func DeleteUser(userID uint) error {
	// TODO: 删除用户时，还需要处理该用户上传的图片和API Token
	// 可以选择：
	// 1. 将图片和Token转交给一个默认用户
	// 2. 彻底删除所有关联数据
	// 简单起见，这里先只删除用户和其API Token
	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&database.APIToken{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&database.User{}, userID).Error; err != nil {
			return err
		}
		return nil
	})
}

// CreateAPIToken 为用户创建API Token
func CreateAPIToken(userID uint, name string) (*database.APIToken, error) {
	tokenValue := uuid.New().String() // 生成随机Token值
	apiToken := database.APIToken{
		UserID:   userID,
		Token:    tokenValue,
		Name:     name,
		IsActive: true,
	}
	if err := database.DB.Create(&apiToken).Error; err != nil {
		return nil, err
	}
	return &apiToken, nil
}

// ToggleAPITokenStatus 启用/禁用API Token
func ToggleAPITokenStatus(tokenID uint) (*database.APIToken, error) {
	var apiToken database.APIToken
	if err := database.DB.First(&apiToken, tokenID).Error; err != nil {
		return nil, errors.New("API Token not found")
	}
	apiToken.IsActive = !apiToken.IsActive
	if err := database.DB.Save(&apiToken).Error; err != nil {
		return nil, err
	}
	return &apiToken, nil
}

// DeleteAPIToken 删除API Token
func DeleteAPIToken(tokenID uint) error {
	return database.DB.Delete(&database.APIToken{}, tokenID).Error
}

// GetUserAPITokens 获取用户的API Token列表
func GetUserAPITokens(userID uint) ([]database.APIToken, error) {
	var tokens []database.APIToken
	err := database.DB.Where("user_id = ?", userID).Find(&tokens).Error
	return tokens, err
}
