package api

import (
	"net/http"
	"strconv"
	"yanshu-imgbed/database"
	"yanshu-imgbed/service"

	"github.com/gin-gonic/gin"
)

// LoginRequest 登录请求结构
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginHandler 处理用户登录
func LoginHandler(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token, err := service.Login(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token, "message": "登录成功"})
}

// GetUserInfo 获取当前登录用户信息
func GetUserInfoHandler(c *gin.Context) {
	userID := c.MustGet("userID").(uint)
	username := c.MustGet("username").(string)
	role := c.MustGet("userRole").(string)

	c.JSON(http.StatusOK, gin.H{
		"user_id":  userID,
		"username": username,
		"role":     role,
	})
}

// --- User Management (Admin Only) ---

// ListUsersHandler 列出所有用户
func ListUsersHandler(c *gin.Context) {
	var users []database.User
	database.DB.Select("id", "username", "role", "created_at", "updated_at").Find(&users)
	c.JSON(http.StatusOK, users)
}

// RegisterUserHandler 注册新用户 (管理员可以指定角色)
type RegisterUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role"` // 允许管理员指定角色
}

func RegisterUserHandler(c *gin.Context) {
	var req RegisterUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Role == "" {
		req.Role = "user" // 默认普通用户
	}

	user, err := service.RegisterUser(req.Username, req.Password, req.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "用户创建成功", "user_id": user.ID, "username": user.Username})
}

// ResetPasswordHandler 重置用户密码 (管理员)
type ResetPasswordRequest struct {
	NewPassword string `json:"new_password" binding:"required"`
}

func ResetPasswordHandler(c *gin.Context) {
	userID, _ := strconv.Atoi(c.Param("id"))
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := service.ResetUserPassword(uint(userID), req.NewPassword); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "密码重置成功"})
}

// DeleteUserHandler 删除用户 (管理员)
func DeleteUserHandler(c *gin.Context) {
	userID, _ := strconv.Atoi(c.Param("id"))
	if err := service.DeleteUser(uint(userID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "用户删除成功"})
}

// --- Self-Service Password Change ---

// ChangeMyPasswordHandler 修改自己的密码 (普通用户和管理员)
type ChangeMyPasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

func ChangeMyPasswordHandler(c *gin.Context) {
	userID := c.MustGet("userID").(uint)
	var req ChangeMyPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := service.ChangePassword(userID, req.OldPassword, req.NewPassword); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()}) // 旧密码错误也算Unauthorized
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "密码修改成功"})
}

// --- API Token Management ---

// ListAPITokensHandler 列出当前用户的API Token
func ListAPITokensHandler(c *gin.Context) {
	userID := c.MustGet("userID").(uint)
	tokens, err := service.GetUserAPITokens(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取API Token失败"})
		return
	}
	c.JSON(http.StatusOK, tokens)
}

// CreateAPITokenHandler 为当前用户创建API Token
type CreateAPITokenRequest struct {
	Name string `json:"name" binding:"required"`
}

func CreateAPITokenHandler(c *gin.Context) {
	userID := c.MustGet("userID").(uint)
	var req CreateAPITokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	token, err := service.CreateAPIToken(userID, req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建API Token失败"})
		return
	}
	c.JSON(http.StatusCreated, token)
}

// ToggleAPITokenStatusHandler 启用/禁用API Token
func ToggleAPITokenStatusHandler(c *gin.Context) {
	tokenID, _ := strconv.Atoi(c.Param("id"))
	userID := c.MustGet("userID").(uint) // 确保只能操作自己的Token

	var apiToken database.APIToken
	if err := database.DB.First(&apiToken, tokenID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "API Token not found"})
		return
	}
	if apiToken.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此API Token"})
		return
	}

	token, err := service.ToggleAPITokenStatus(uint(tokenID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, token)
}

// DeleteAPITokenHandler 删除API Token
func DeleteAPITokenHandler(c *gin.Context) {
	tokenID, _ := strconv.Atoi(c.Param("id"))
	userID := c.MustGet("userID").(uint) // 确保只能操作自己的Token

	var apiToken database.APIToken
	if err := database.DB.First(&apiToken, tokenID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "API Token not found"})
		return
	}
	if apiToken.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此API Token"})
		return
	}

	if err := service.DeleteAPIToken(uint(tokenID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "API Token删除成功"})
}
