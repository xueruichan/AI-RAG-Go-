// redis配置
package config

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// InitRedis 初始化 Redis Client。
// 当前开发环境：
// GoRAG
//   ↓
// 127.0.0.1:6379
//   ↓
// Redis
// DB = 0 表示使用 Redis 默认的第 0 个逻辑数据库

// *redis.Client：Redis 客户端对象，后续可以用它来执行 Redis 命令
// *gorm.DB是GO操作MySQL的入口，而*redis.Client是GO操作Redis的入口
func InitRedis() (*redis.Client, error) {
	//创建redis客户端
	//*redis.Options 结构体:包含连接 Redis 所需的配置信息
	client := redis.NewClient(
		&redis.Options{
			Addr:     "127.0.0.1:6379",
			Password: "",
			DB:       0,
		},
	)
	//NewClient 创建的是 Redis Client,redis.NewClient 只是创建了客户端对象，并没有真正建立网络连接
	//ping才能验证是否gorag->redis可以正常通信
	//context.Background() 是 Go 语言中用于传递上下文的标准方式，这里可以简单理解为提供一个空的上下文，用于控制请求的超时或取消
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}
	fmt.Println("Redis connected successfully")
	return client, nil
}
