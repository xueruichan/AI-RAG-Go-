// 负责将“检索”和“生成”两个阶段串联起来
package service

import (
	"context"
	"fmt"
	"time"

	"gorag/internal/model"

	// internal/rag包名本来就是rag。
	//
	// 这里起别名ragcore，
	// 只是为了代码阅读起来更明确：
	//
	// ragcore.BuildUserPrompt
	ragcore "gorag/internal/rag"

	"gorag/pkg/llm"
)

type RAGService struct {
	// Module8已经完成的Retrieval
	SearchService *SearchService
	// Module9新增的Generation
	llmClient llm.Client
}

func NewRAGService(
	SearchService *SearchService,
	llmClient llm.Client,
) *RAGService {
	return &RAGService{
		SearchService: SearchService,
		llmClient:     llmClient,
	}
}

// RAG 的核心方法，接收用户问题 query 和期望返回数量 topK
func (s *RAGService) Ask(
	ctx context.Context,
	query string,
	topK int,
) (*model.RAGResponse, error) {
	//1. Retrieval,调用module8的接口SearchService.Search()
	results, err := s.SearchService.Search(
		ctx,
		query,
		topK,
	)

	if err != nil {
		return nil, fmt.Errorf("retrieve knowledge failed: %w", err)
	}

	//2.搜索不到任何相关文档chunk，不调用API Key，否则容易产生幻觉
	if len(results) == 0 {
		return &model.RAGResponse{
			Answer:  "根据当前知识库资料无法确定。",
			Sources: []model.RAGSource{},
		}, nil
	}

	//3.Context -> Prompt
	userPrompt := ragcore.BuildUserPrompt(
		query,
		results,
	)

	//4. Generation  生成回答
	//设置超时时间60s
	llmCtx, cancel := context.WithTimeout(
		ctx,
		60*time.Second,
	)
	defer cancel()

	//5.调用deepseek
	answer, err := s.llmClient.Chat(
		llmCtx,
		//系统提示符
		ragcore.SystemPrompt,
		//用户提示符(context+question)
		userPrompt,
	)

	if err != nil {
		return nil, fmt.Errorf("generate answer failed: %w", err)
	}

	//6.构造来源列表

	sources := make([]model.RAGSource, 0, len(results))

	//遍历 results（检索结果），为每个结果生成一个 RAGSource
	for i, result := range results {
		sources = append(
			sources,
			model.RAGSource{
				//和prompt.go中对应
				Reference:  fmt.Sprintf("资料%d", i+1),
				DocumentID: result.DocumentID,
				FileName:   result.FileName,
				ChunkID:    result.ChunkID,
				ChunkIndex: result.ChunkIndex,
				Score:      result.Score,
			},
		)
	}

	//7.最终结果
	return &model.RAGResponse{
		Answer:  answer,
		Sources: sources,
	}, nil
}

// AskStream 实现 RAG 的流式问答。
//
// onSources：检索结束后，将真实文档来源交给 HTTP 层。
// onDelta：每生成一段文本，就将该段文本交给 HTTP 层。
//
// 这里不直接操作 HTTP，也不依赖 Gin。
// Service 只负责检索和调用模型，保持分层结构。
func (s *RAGService) AskStream(
	ctx context.Context,
	query string,
	topK int,
	onSources func([]model.RAGSource) error,
	onDelta func(string) error,
) error {

	// 1. 检查回调函数，避免调用 nil 函数发生 panic。
	if onSources == nil {
		return fmt.Errorf("onSources callback is nil")
	}

	if onDelta == nil {
		return fmt.Errorf("onDelta callback is nil")
	}

	// 2. 检查当前 LLM 客户端是否支持流式生成。
	//
	// llmClient 的静态类型仍然是 llm.Client，
	// 但实际创建的是 *DeepSeekClient。
	// Go 的类型断言用于检查实际对象是否实现了
	// llm.StreamingClient 接口。
	streamClient, ok := s.llmClient.(llm.StreamingClient)
	if !ok {
		return fmt.Errorf("LLM client does not support streaming")
	}

	// 3. Retrieval：复用原有的 SearchService。
	// 这里会完成 Query Embedding、Milvus 搜索和 MySQL 回查。
	results, err := s.SearchService.Search(
		ctx,
		query,
		topK,
	)

	if err != nil {
		return fmt.Errorf("retrieve knowledge failed: %w", err)
	}

	// 4. 根据检索结果构造真实来源。
	// 来源来自数据库和检索结果，而不是让大模型自行编造。
	sources := make([]model.RAGSource, 0, len(results))

	for i, result := range results {
		sources = append(
			sources,
			model.RAGSource{
				Reference:  fmt.Sprintf("资料%d", i+1),
				DocumentID: result.DocumentID,
				FileName:   result.FileName,
				ChunkID:    result.ChunkID,
				ChunkIndex: result.ChunkIndex,
				Score:      result.Score,
			},
		)
	}

	// 5. 将来源发送给上层。
	// HTTP 层收到后，会通过 SSE 的 sources 事件发给客户端。
	if err := onSources(sources); err != nil {
		return fmt.Errorf("send sources failed: %w", err)
	}

	// 6. 如果没有检索到任何资料，就不调用大模型。
	// 直接把现有项目使用的兜底答案作为一段文本发送。
	if len(results) == 0 {
		return onDelta("根据当前知识库资料无法确定。")
	}

	// 7. 复用原来的 Prompt 构建逻辑。
	// 不需要重新设计 RAG 的提示词。
	userPrompt := ragcore.BuildUserPrompt(
		query,
		results,
	)

	// 8. 为模型生成设置 60 秒超时。
	// 请求被取消时，模型调用也会随之终止。
	llmCtx, cancel := context.WithTimeout(
		ctx,
		60*time.Second,
	)
	defer cancel()

	// 9. 真正执行流式生成。
	// 每收到一段文本，StreamChat 就调用 onDelta。
	err = streamClient.StreamChat(
		llmCtx,
		ragcore.SystemPrompt,
		userPrompt,
		onDelta,
	)

	if err != nil {
		return fmt.Errorf("generate streaming answer failed: %w", err)
	}

	return nil
}
