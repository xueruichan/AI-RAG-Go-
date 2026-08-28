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
