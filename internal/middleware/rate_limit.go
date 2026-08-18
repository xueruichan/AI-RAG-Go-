// RateLimit 创建一个简单固定窗口限流 Middleware。
//HTTP进入业务前的公共检查，例如限流
// 例如：
// limit  = 60
// window = 1 minute
// 表示：
// 每个 IP
// 60 requests / minute
package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// 接收 Redis 客户端、限制次数、时间窗口，返回一个 Gin 中间件
func RateLimit(
	redisClient *redis.Client,
	limit int64,
	window time.Duration,
) gin.HandlerFunc {
	//c *gin.Context：当前HTTP请求在GIN中的上下文对象
	return func(c *gin.Context) {
		//1.获取客户端ip
		ip := c.ClientIP()

		//为每个ip生成一个Redis key进行计数
		key := fmt.Sprintf("rate_limit:%s", ip)
		ctx := c.Request.Context()

		//2.该ip每访问一次计数就加一并返回
		//这个操作是原子性的，即使多个请求同时到达，也不会出现计数错误
		//执行 Redis 的 INCR key 命令：将 key 的值加 1，并返回加 1 后的值。
		count, err := redisClient.Incr(
			ctx,
			key,
		).Result()

		if err != nil {
			c.Next()
			return
		}

		//3.第一次请求时设置窗口过期时间
		//执行 Redis 的 EXPIRE key seconds 命令：给 key 设置一个过期时间（TTL），时间单位是秒
		//当过期时间到达后，Redis 会自动删除这个 key
		if count == 1 {
			if err := redisClient.Expire(
				ctx,
				key,
				window,
			).Err(); err != nil {
				_ = redisClient.Del(
					ctx,
					key,
				).Err()

				c.Next()
				return
			}
		}

		//4.超过限制,拒绝请求
		if count > limit {
			//停止当前GIN请求，无法进入后续的handler，service层
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "too many requests",
			})

			return
		}

		//5.没超过限制,直接放行
		c.Next()
	}
}
