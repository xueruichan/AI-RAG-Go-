// 搜索功能的 HTTP 处理器
// 负责接收客户端发来的搜索请求，解析 JSON 请求体，验证参数，然后调用 SearchService 执行搜索，最后将结果以 JSON 格式返回给客户端
package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"gorag/internal/model"
	"gorag/internal/service"
)

type SearchHandler struct {
	service *service.SearchService
}

func NewSearchHandler(searchService *service.SearchService) *SearchHandler {
	return &SearchHandler{
		service: searchService,
	}
}

func (h *SearchHandler) Search(c *gin.Context) {
	var req model.SearchRequest
	//把 JSON：
	// {
	//     "query": "...",
	//     "top_k": 5
	// }  解析到 SearchRequest

	//c.ShouldBindJSON(&req)：Gin 提供的方法，从当前请求的 Body 中读取 JSON 数据，并尝试反序列化到 req 结构体
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid search request",
		})
		return
	}

	//检查 query 是否为空
	if strings.TrimSpace(req.Query) == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "query cannot be empty",
		})
		return
	}

	//检查 top_k 参数
	if req.TopK < 0 || req.TopK > service.MaxSearchTopK {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "top_k must be between 1 and 20",
		})
		return
	}

	//调用service层进行搜索
	results, err := h.service.Search(
		c.Request.Context(),
		req.Query,
		req.TopK,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, results)
}
