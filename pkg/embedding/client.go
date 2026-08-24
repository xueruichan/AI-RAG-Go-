// Embedding 客户端：负责“文本 → 向量”的转换
package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// const 常量名 = 值或const 常量名 数据类型= 值,但是不能用短声明或者var
// 阿里云百炼的API
const(
	dashScopeEmbeddingURL = "https://dashscope.aliyuncs.com/compatible-mode/v1/embeddings"
	constDefaultDimension=1024
)
//分离内部使用的常量和对外暴露的接口
const DefaultDimension=constDefaultDimension



// EmbeddingClient 定义 Embedding 能力
//接口里写的是“方法签名”，不是定义或调用:只要拥有这个方法就能实现接口
/*
type 接口名 interface {
    方法名(参数列表) 返回值列表
    ...
}
*/
type EmbeddingClient interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// OpenAIClient 是EmbeddingClient 的一种具体实现
type DashScopeClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewDashScopeClient(apiKey string) *DashScopeClient {
	return &DashScopeClient{
		apiKey: apiKey,
		model:  "text-embedding-v4",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// embeddingRequest 表示发送给Embedding API的JSON。
// 使用结构体来表示 JSON 数据，并通过标签来指定结构体字段与 JSON 键之间的映射关系
type embeddingRequest struct {
	Input          string `json:"input"`
	Model          string `json:"model"`
	EncodingFormat string `json:"encoding_format"`
	// 强制要求返回 1024 维，
	// 避免模型配置变化以后和 Milvus Schema 不一致
	Dimension      int    `json:"dimensions"`
}

// embeddingResponse：表示 API 返回的 JSON 结构
type embeddingResponse struct {
	//Data是切片，里面存放结构体
	//[]struct{}是匿名结构体，作用类似于匿名函数
	//OpenAI 的 Embedding API 设计成支持批量处理。可以在一次请求中传入多个文本，API 会为每个文本生成一个向量，然后放在一个数组里返回
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed 把一段文本转换为向量
func (c *DashScopeClient) Embed(
	ctx context.Context,
	text string,
) ([]float32, error) {
	//TrimSpace去除文本空格
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("embedding text cannot be empty")
	}

	//1.构造GO的请求结构体
	requestBody := embeddingRequest{
		Input:          text,
		Model:          c.model,
		EncodingFormat: "float",
		Dimension: DefaultDimension,
	}

	//2. Go struct -> JSON
	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request failed: %w", err)
	}

	//3.创建 HTTP POST Request
	//在内存里创建一个 HTTP 请求对象
	req, err := http.NewRequestWithContext(
		//控制：取消，超时，生命周期
		ctx,
		//post
		http.MethodPost,
		dashScopeEmbeddingURL,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("create embedding request failed: %w", err)
	}

	// 4. 添加 HTTP Header
	req.Header.Set(
		"Authorization",
		"Bearer "+c.apiKey,
	)

	req.Header.Set(
		"Content-Type",
		"application/json",
	)
	/*
		NewRequestWithContext
	        │
	        │ 创建
	        ▼
	   http.Request
	        │
	        │ 传给
	        ▼
	     Client.Do
	        │
	        │ 真正发送
	        ▼
	      网络
	*/
	//5.发出HTTP请求
	//发送 HTTP Request，并返回 HTTP Response：
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	//6.检查HTTP状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errorBody, _ := io.ReadAll(resp.Body)

		return nil, fmt.Errorf("embedding api returned status %d: %s", resp.StatusCode, string(errorBody))
	}

	//7.JSON Response -> Go struct
	var result embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embedding response failed: %w", err)
	}

	//8. 检查 API 是否真的返回向量
	if len(result.Data) == 0 {
		return nil, fmt.Errorf(
			"embedding api returned empty data",
		)
	}
	
	vector:=result.Data[0].Embedding
	if len(vector)==0{
		return nil,fmt.Errorf("embedding api returned empty vector")
	}

	//9.维度错误（不是1024）
	if len(vector)!=DefaultDimension{
		return nil,fmt.Errorf("embedding dimension mismatch: expected=%d actual=%d",DefaultDimension,len(vector))
	}

	return vector,nil
}
