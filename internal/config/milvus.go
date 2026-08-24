// 创建 Milvus 连接配置
package config

import (
	"context"
	"fmt"
	"os"

	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

// InitMilvus 创建 Milvus Client
// 与 MySQL 的 InitDB、Redis 的 InitRedis 类似
func InitMilvus(ctx context.Context) (*milvusclient.Client, error) {
	// 优先从环境变量获取地址
	address := os.Getenv("MILVUS_ADDRESS")

	if address == "" {
		address = "127.0.0.1:19530"
	}

	client, err := milvusclient.New(
		ctx,
		&milvusclient.ClientConfig{
			Address: address,
		},
	)

	if err != nil {
		return nil, fmt.Errorf("connect milvus %s failed: %w", address, err)
	}

	return client, nil
}
