// 定义了 RAG 接口的数据结构
package model

/*
整体上，它们是 RAG 系统对外接口的数据契约：

客户端发送一个查询问题，可能带上 top_k 参数。

服务端处理后，返回一个最终答案（answer）以及支撑这个答案的多个来源（sources），每个来源包含来源编号、文档 ID、文件名、chunk ID、chunk 序号和相似度分数。
*/

// RAGRequest 是 POST /ask 的请求体
type RAGRequest struct {
	Query string `json:"query" binding:"required"`
	TopK  int    `json:"top_k,omitempty"`
}

// RAGSource 表示最终答案的一条真实来源
// Source 是后端根据数据库查询结果生成的,而不是让 LLM 自己生成
type RAGSource struct {
	//对应 Prompt中的资料
	Reference string `json:"reference"`
	//文档ID
	DocumentID uint `json:"document_id"`
	//原始PDF文件名
	FileName string `json:"filename"`
	// MySQL Chunk主键
	ChunkID uint `json:"chunk_id"`
	// 这篇PDF里的第几个Chunk
	ChunkIndex int `json:"chunk_index"`
	// Milvus相似度
	Score float32 `json:"score"`
}

// RAGResponse 是 POST /ask 最终返回的数据
type RAGResponse struct {
	Answer  string      `json:"answer"`
	Sources []RAGSource `json:"sources"`
}
