package config

import (
	"context"
	"fmt"
	"os"

	"github.com/redis/go-redis/v9"
)

// InitRedis 初始化 Redis 客户端。
func InitRedis() (*redis.Client, error) {
	// 本地开发默认连接 127.0.0.1:6379。
	// Docker 部署时使用 REDIS_ADDR=redis:6379。
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}

	// 本地 Redis 默认无密码。
	// 部署时通过 REDIS_PASSWORD 设置密码。
	password := os.Getenv("REDIS_PASSWORD")

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})

	// 通过 Ping 检查连接是否正常。
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect to Redis at %s failed: %w", addr, err)
	}

	fmt.Println("Redis connected successfully")
	return client, nil
}