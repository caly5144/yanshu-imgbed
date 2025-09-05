package main

import (
	"embed"
	"fmt"
	"log"
	"yanshu-imgbed/config"
	"yanshu-imgbed/database"
	"yanshu-imgbed/manager"
	"yanshu-imgbed/router"
	"yanshu-imgbed/service"
)

//go:embed templates/*
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

func main() {
	// 1. 初始化配置
	if err := config.Init(); err != nil {
		log.Fatalf("Failed to initialize configuration: %v", err)
	}

	// 2. 初始化数据库 (传入配置)
	if err := database.Init(config.Cfg.Database.DSN); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// 3. --- 新增：初始化设置缓存 ---
	service.InitSettings()

	// 4. 初始化存储管理器
	storageManager, err := manager.NewStorageManager()
	if err != nil {
		log.Fatalf("Failed to initialize storage manager: %v", err)
	}

	// 5. 设置并运行路由 (注入管理器和嵌入的资源)
	r := router.SetupRouter(storageManager, templatesFS, staticFS)

	serverAddr := fmt.Sprintf(":%s", config.Cfg.Server.Port)
	log.Printf("Server is running on http://127.0.0.1%s", serverAddr)
	if err := r.Run(serverAddr); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}
