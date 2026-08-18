// MySQL Repository：
// DocumentRepository

// Redis Repository：
// DocumentCacheRepository

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorag/internal/model"

	"github.com/redis/go-redis/v9"
)

type DocumentCacheRepository struct {
	client *redis.Client
}

func NewDocumentCacheRepository(client *redis.Client) *DocumentCacheRepository {
	return &DocumentCacheRepository{client: client}
}

// documentCacheKey:统一生成Redis key
// Redis中的key只能以字符串存储，但是go项目中是uint，所以要进行转换
// 不能直接拿数字10当 Redis key 用，需要把它转换成一个字符串，并且最好加上前缀（比如 "doc:cache:"）来区分不同业务的数据。
func documentCacheKey(id uint) string {
	//Sprintf将内容作为一个字符串返回不打印
	//msg := fmt.Sprintf("....")
	return fmt.Sprintf("document:%d", id)
}

// Get 从 Redis 查询 Document
// 返回值：document found error
func (r *DocumentCacheRepository) Get(id uint) (*model.Document, bool, error) {
	//go-redis 库的所有网络操作方法（Get、Set、Ping 等）的函数签名都要求第一个参数是 context.Context
	//ctx的作用：
	//超时：如果 Redis 服务器响应很慢，你可以设置“最多等 200 毫秒，超时就放弃”。
	//取消：如果用户关闭了网页，你可以立刻取消还在进行的 Redis 查询，避免浪费资源
	ctx := context.Background()
	value, err := r.client.Get(
		ctx,
		documentCacheKey(id),
	).Result()

	//错误是 redis.Nil，说明 Redis 中这个键不存在
	//Cache miss不代表出错，仅代表这个key不在Redis中
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}

	//真正的Redis错误
	if err != nil {
		return nil, false, err
	}

	//redis里面数据存储是json格式没有结构体，要把它取出来后转成go程序能看懂的结构体。将json反序列化为结构体
	var document model.Document
	//json.Unmarshal 需要 []byte所以需要强转
	if err := json.Unmarshal([]byte(value), &document); err != nil {
		return nil, false, err
	}

	return &document, true, nil
}

// Set 把 Document 写入 Redis
// Document
// ↓
// JSON
// ↓
// Redis String
// ttl 控制缓存多久后自动过期。
// 传参：需要缓存的文档对象指针，生存时间
func (r *DocumentCacheRepository) Set(document *model.Document, ttl time.Duration) error {
	ctx := context.Background()

	//和Get相反，将结构体序列化为json
	data, err := json.Marshal(document)
	if err != nil {
		return err
	}

	//r.client.Set 是 go-redis 库提供的 Redis SET 命令封装。它接受四个参数：
	//ctx，key，value，ttl
	return r.client.Set(ctx, documentCacheKey(document.ID), data, ttl).Err()
}

// Delete 删除 Document Cache
func (r *DocumentCacheRepository) Delete(id uint) error {
	ctx := context.Background()

	return r.client.Del(ctx, documentCacheKey(id)).Err()
}
