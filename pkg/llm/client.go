// llm 封装大语言模型客户端
// 专门用来调用 DeepSeek 的 Chat Completions API，完成“给定系统提示 + 用户提示，返回模型生成的回答”的功能。它是对 HTTP 细节的封装
// Service 层只依赖 Client 接口，不直接依赖 DeepSeek 的 HTTP 细节
package llm

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

const (
	deepSeekChatURL = "https://api.deepseek.com/chat/completions"
	defaultModel    = "deepseek-v4-flash"
)

// SearchService 不应该知道 HTTP 请求怎么发送；
// RAGService 也不应该知道 DeepSeek JSON 长什么样
// 它们只需要知道：
// 给你 System Prompt + User Prompt
//System Prompt（系统提示词）:设定模型的行为、角色、规则,通常由后端写死，用来约束模型不要乱编、要忠于检索到的文档内容。
//User Prompt（用户提示词）:提出具体问题或任务
// 你给我一个最终字符串答案。

type Client interface {
	Chat(
		ctx context.Context,
		systemPrompt string,
		userPrompt string,
	) (string, error)
}

// DeepSeekClient 是 Client 接口的一种具体实现
type DeepSeekClient struct {
	apiKey string
	model  string
	//发送 HTTP 请求
	httpClient *http.Client
}

func NewDeepSeekClient(apiKey string) *DeepSeekClient {
	return &DeepSeekClient{
		apiKey: apiKey,
		model:  defaultModel,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// chatMessage 对应 DeepSeek 请求中的一条消息:
//
//	{
//	    "role": "system",
//	    "content": "..."
//	}
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// thinkingConfig 控制 DeepSeek 是否开启 Thinking
type thinkingConfig struct {
	Type string `json:"type"`
}

// chatRequest 对应发给 DeepSeek 的整个 JSON
// 结构体标签（struct tag）。它主要用于 JSON 序列化和反序列化 时,结构体字段名和 JSON 键名之间的对应关系
// Go 会把字段名（如 Model）作为 JSON 的键名
type chatRequest struct {
	Model       string         `json:"model"`
	Messages    []chatMessage  `json:"messages"`
	Thinking    thinkingConfig `json:"thinking"`
	Temperature float64        `json:"temperature"`
	MaxTokens   int            `json:"max_tokens"`
	Stream      bool           `json:"stream"`
}

// chatResponse 只定义我们真正需要读取的字段。
//
// DeepSeek 实际返回还有：
// id
// usage
// finish_reason
// ...
type chatResponse struct {
	//匿名结构体
	Choice []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Chat 真正调用 DeepSeek
func (c *DeepSeekClient) Chat(
	ctx context.Context,
	systemPrompt string,
	userPrompt string,
) (string, error) {
	// 1. API Key 防御性检查
	if strings.TrimSpace(c.apiKey) == "" {
		return "", fmt.Errorf("deepseek api key is empty")
	}

	// User Prompt 不允许为空
	if strings.TrimSpace(userPrompt) == "" {
		return "", fmt.Errorf("user prompt cannot be empty")
	}

	// 2. 构造 Go 请求结构体,包含了调用 DeepSeek 聊天接口所需的全部信息
	requestBody := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			//系统提示和用户提示
			{
				Role:    "system",
				Content: systemPrompt,
			},
			{
				Role:    "user",
				Content: userPrompt,
			},
		},

		//RAG场景下：关闭深度思考模式
		Thinking: thinkingConfig{
			Type: "disabled",
		},

		//设置采样温度，值越小输出越确定、稳定、保守
		Temperature: 0.2,

		//设置模型最多生成的 token 数量:1024 个 token 大约能生成 500-700 个中文字符
		MaxTokens: 1024,

		//设置是否使用流式响应
		//false 表示不使用流式，即 API 会一次性返回完整结果，而不是分块推送
		Stream: false,
	}

	// 3. Go struct -> JSON
	//将一个 Go 值（之前构造的 chatRequest 结构体 requestBody）转换为 JSON 格式的字节切片
	body, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("marshal deepseek request failed: %w", err)
	}

	// 4. 创建 HTTP POST 请求
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		deepSeekChatURL,
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("create deepseek request failed: %w", err)
	}

	// 5. 设置 Header
	//设置请求头（Header），用来告诉服务器如何理解这个请求
	req.Header.Set(
		"Authorization",
		"Bearer "+c.apiKey,
	)

	req.Header.Set(
		"Content-Type",
		"application/json",
	)

	// 6. 真正访问 DeepSeek,发送请求,并接收服务器的响应
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send deepseek request failed: %w", err)
	}
	//resp.Body 是响应体，类型为 io.ReadCloser。关闭它释放底层网络连接资源，避免资源泄漏
	defer resp.Body.Close()

	//7.检查状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 最多读取8KB错误内容，避免异常服务返回巨大Body
		errorBody, _ := io.ReadAll(
			io.LimitReader(
				resp.Body,
				8*1024,
			),
		)
		return "", fmt.Errorf("deepseek api returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(errorBody)))
	}

	// 8. JSON -> Go struct
	//当状态码表示成功时，将响应体中的 JSON 解析到 chatResponse 结构体中，方便后续提取答案
	var result chatResponse

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode deepseek response failed: %w", err)
	}

	// 9. 防止返回空 choices
	//不能继续取 [0]，否则会 panic
	if len(result.Choice) == 0 {
		return "", fmt.Errorf("deepseek api returned no choices")
	}

	//提取最终答案
	answer := strings.TrimSpace(result.Choice[0].Message.Content)

	if answer == "" {
		return "", fmt.Errorf("deepseek api returned empty answer")
	}

	return answer, nil
}
