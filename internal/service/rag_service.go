//负责将“检索”和“生成”两个阶段串联起来
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
