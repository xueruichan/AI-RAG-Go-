// 生成 .exe 程序---package main
// 给别人调用的库---package 文件夹名
package main

import (
	"context"
	"fmt"
	"gorag/pkg/embedding"
	"log"
	"os"
	"time"
)

func main() {
	//1.从环境变量读取 API Key
	apiKey := os.Getenv("DASHSCOPE_API_KEY")
	if apiKey == "" {
		log.Fatal("DASHSCOPE_API_KEY is not set")
	}

	//2.创建 Embedding Client
	client := embedding.NewDashScopeClient(apiKey)

	//3.设置超时时间:本次embedding 20s内完成
	ctx, cancel := context.WithTimeout(
		context.Background(),
		20*time.Second,
	)

	defer cancel()

	//4.调用embedding
	vector, err := client.EMbed(
		ctx,
		"Hello World",
	)
	if err != nil {
		log.Fatal("embedding failed:", err)
	}

	//5.确认获得向量
	fmt.Printf("Embedding dimension: %d\n", len(vector))
	//打印前5个值，如果向量长度不足 5，就打印实际长度
	previewSize := 5
	if len(vector) < previewSize {
		previewSize = len(vector)
	}
	//vector[:previewSize] 是切片操作，取前 previewSize 个元素
	fmt.Printf("First %d values: %v\n", previewSize, vector[:previewSize])

}
