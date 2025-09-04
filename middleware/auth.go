package middleware

import (
	"errors"
	"net/http"
	"strings"
	"yanshu-imgbed/config" // 确保引入了 config 包
	"yanshu-imgbed/database"
	"yanshu-imgbed/service"

	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AuthMiddleware JWT认证中间件
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := c.GetHeader("Authorization")
		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		parts := strings.Split(tokenString, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid Authorization header format"})
			c.Abort()
			return
		}
		tokenString = parts[1]

		claims := &service.Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			// 使用新的配置方式
			return []byte(config.Cfg.JWT.Secret), nil
		})

		if err != nil {
			if err == jwt.ErrSignatureInvalid {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token signature"})
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			}
			c.Abort()
			return
		}

		if !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("userRole", claims.Role)
		c.Next()
	}
}

// AdminAuthMiddleware 检查是否为管理员
func AdminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("userRole")
		if !exists || role.(string) != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden: Admin access required"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// APITokenAuthMiddleware 验证API Token
func APITokenAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenValue := c.GetHeader("X-API-TOKEN")
		if tokenValue == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "API Token required"})
			c.Abort()
			return
		}

		var apiToken database.APIToken
		if err := database.DB.Preload("User").Where("token = ? AND is_active = ?", tokenValue, true).First(&apiToken).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or inactive API Token"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error checking API Token"})
			}
			c.Abort()
			return
		}

		c.Set("userID", apiToken.UserID)
		c.Set("username", apiToken.User.Username)
		c.Set("userRole", apiToken.User.Role)
		c.Next()
	}
}

// CombinedAuthMiddleware 组合认证 (API Token 和 JWT)
func CombinedAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 尝试 API Token
		tokenValue := c.GetHeader("X-API-TOKEN")
		if tokenValue != "" {
			var apiToken database.APIToken
			err := database.DB.Preload("User").Where("token = ? AND is_active = ?", tokenValue, true).First(&apiToken).Error
			if err == nil {
				c.Set("userID", apiToken.UserID)
				c.Set("username", apiToken.User.Username)
				c.Set("userRole", apiToken.User.Role)
				c.Next()
				return
			}
		}

		// 2. 尝试 JWT Token
		tokenString := c.GetHeader("Authorization")
		if tokenString != "" {
			parts := strings.Split(tokenString, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenString = parts[1]
				claims := &service.Claims{}
				token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
					// 使用新的配置方式
					return []byte(config.Cfg.JWT.Secret), nil
				})

				if err == nil && token.Valid {
					c.Set("userID", claims.UserID)
					c.Set("username", claims.Username)
					c.Set("userRole", claims.Role)
					c.Next()
					return
				}
			}
		}

		// 两种认证都失败
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required (JWT or API Token)"})
		c.Abort()
	}
}
