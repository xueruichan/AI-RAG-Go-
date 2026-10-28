package config

import (
	"fmt"
	"os"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// getEnvOrDefault 从环境变量读取配置。
// 如果环境变量没有设置，就使用 fallback 默认值。
// 这样既能兼容你原来的本地开发环境，
// 也能在 Docker 中通过环境变量指定数据库地址。
func getEnvOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value != "" {
		return value
	}

	return fallback
}

// InitDB 初始化 MySQL 连接。
func InitDB() (*gorm.DB, error) {
	// 本地开发时使用默认值；
	// Docker Compose 部署时会传入容器内部的服务地址。
	host := getEnvOrDefault("DB_HOST", "127.0.0.1")
	port := getEnvOrDefault("DB_PORT", "3306")
	user := getEnvOrDefault("DB_USER", "gorag")
	password := getEnvOrDefault("DB_PASSWORD", "gorag123456")
	database := getEnvOrDefault("DB_NAME", "gorag")

	// 拼接 MySQL DSN。
	// host 在 Docker 部署时会是 mysql，而不是 127.0.0.1。
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		user,
		password,
		host,
		port,
		database,
	)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		// 不打印密码，避免敏感信息出现在日志中。
		return nil, fmt.Errorf(
			"connect to MySQL at %s:%s failed: %w",
			host,
			port,
			err,
		)
	}

	fmt.Println("MySQL connected successfully")
	return db, nil
}