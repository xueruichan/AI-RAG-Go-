//RAGHandler 是 Gin 框架中的一个 HTTP 处理器，对应 RAG 问答接口（POST /ask）。它只负责 HTTP 层面的工作

package handler

import (
	"github.com/gin-gonic/gin"
	"gorag/internal/model"
	"gorag/internal/service"
	"net/http"
	"strings"
)

type RAGHandler struct {
	service *service.RAGService
}

func NewRAGHandler(
	ragService *service.RAGService,
) *RAGHandler {
	return &RAGHandler{
		service: ragService,
	}
}

// Ask:处理 POST /ask 请求,从 HTTP 请求中提取数据，调用业务逻辑，并返回响应
func (h *RAGHandler) Ask(
	c *gin.Context,
) {
	//接收query和topK
	var req model.RAGRequest

	//1.JSON解析
	//从请求体中读取 JSON 并反序列化到 req 结构体
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(
			http.StatusBadRequest,
			gin.H{
				"error": "invalid rag request",
			},
		)
		return
	}

	//2.Query参数检查
	if strings.TrimSpace(req.Query) == "" {
		c.JSON(
			http.StatusBadRequest,
			gin.H{
				"error": "query cannot be empty",
			},
		)
		return
	}

	//3.topK不在有效范围内
	if req.TopK < 0 || req.TopK > service.MaxSearchTopK {
		c.JSON(
			http.StatusBadRequest,
			gin.H{
				"error": "top_k must be 0(default) or between 1 and 20",
			},
		)
		return
	}

	//4.调用业务层
	response, err := h.service.Ask(
		c.Request.Context(),
		req.Query,
		req.TopK,
	)

	if err != nil {
		c.JSON(
			http.StatusInternalServerError,
			gin.H{
				"error": err.Error(),
			},
		)
		return
	}

	//5.返回
	c.JSON(
		http.StatusOK,
		response,
	)

}

// AskStream 处理 POST /ask/stream。
//
// 与 Ask 不同：
// Ask 返回一个完整 JSON；
// AskStream 返回持续到达的 SSE 事件。
func (h *RAGHandler) AskStream(c *gin.Context) {

	// 1. 解析请求体。
	// 复用现有的 RAGRequest，不需要新建请求结构体。
	var req model.RAGRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid rag request",
		})
		return
	}

	// 2. 检查 query。
	if strings.TrimSpace(req.Query) == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "query cannot be empty",
		})
		return
	}

	// 3. 检查 top_k。
	// 0 表示使用默认值；1 到 20 是有效范围。
	if req.TopK < 0 || req.TopK > service.MaxSearchTopK {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "top_k must be 0(default) or between 1 and 20",
		})
		return
	}

	// 4. 设置 SSE 响应头。
	//
	// text/event-stream：告诉客户端这是 SSE。
	// no-cache：避免缓存流式响应。
	// X-Accel-Buffering: no：提示 Nginx 不要缓冲响应。
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	c.Status(http.StatusOK)

	// 5. 封装 SSE 事件发送函数。
	//
	// event 是事件名，例如 delta。
	// data 是事件内容，Gin 会将 map 编码为 JSON。
	//
	// 每次 Flush 都尝试将已写入的数据推送给客户端，
	// 而不是等整个问答完成。
	sendEvent := func(event string, data any) error {

		// 客户端断开或请求取消后，不再继续发送。
		if err := c.Request.Context().Err(); err != nil {
			return err
		}

		c.SSEvent(event, data)

		// 将当前事件尽快刷新到 HTTP 响应。
		c.Writer.Flush()

		return c.Request.Context().Err()
	}

	// 6. 告知客户端：开始检索。
	if err := sendEvent(
		"status",
		gin.H{"status": "retrieving"},
	); err != nil {
		return
	}

	// 7. 调用 Service。
	//
	// onSources：将来源转换为 sources 事件。
	// onDelta：将模型生成的每一段文本转换为 delta 事件。
	err := h.service.AskStream(
		c.Request.Context(),
		req.Query,
		req.TopK,

		func(sources []model.RAGSource) error {
			return sendEvent(
				"sources",
				gin.H{"sources": sources},
			)
		},

		func(content string) error {
			return sendEvent(
				"delta",
				gin.H{"content": content},
			)
		},
	)

	// 8. 发生错误时，通过 SSE 发送错误事件。
	//
	// 注意：此时 HTTP 响应可能已经开始，
	// 不能再尝试使用 c.JSON 返回另一个 HTTP 响应。
	if err != nil {

		// 客户端已断开时，不再尝试发送错误。
		if c.Request.Context().Err() != nil {
			return
		}

		_ = sendEvent(
			"error",
			gin.H{"error": err.Error()},
		)
		return
	}

	// 9. 所有文本正常生成结束后，发送 done。
	_ = sendEvent(
		"done",
		gin.H{"message": "completed"},
	)
}
