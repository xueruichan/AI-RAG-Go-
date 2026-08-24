//定义业务中需要使用的数据结构
package model

//SearchRequest：接收搜索请求
//当客户端调用 POST /search 接口时，会发送一个 JSON 请求体:
/*
{
    "query": "goroutine是什么？",
    "top_k": 5
}
*/
type SearchRequest struct{
	Query string `json:"query" binding:"required"`
	TopK  int    `json:"top_k,omitempty"`
}

//SearchResult：返回搜索结果
// 业务层最终返回给客户端 的结果
type SearchResult struct{
	ChunkID uint   `json:"chunk_id"`
	Score float32  `json:"score"`
	Content string `json:"content"`
}