// 搜索服务（SearchService），它是 RAG 系统的查询入口
// 将用户的自然语言问题转换为向量，然后在向量数据库（Milvus）中找到最相似的文档块
// 最后根据这些块的 ID 从 MySQL 中取出原始文本，按相似度高低返回给客户端
package service

import (
	"context"
	"fmt"
	"strings"

	"gorag/internal/model"
	"gorag/internal/repository"
	"gorag/pkg/embedding"
)

const (
	//DefaultSearchTopK:用户没有指定 top_k 时，默认返回 5 条结果
	DefaultSearchTopK = 5
	//MaxSearchTopK：限制用户最多只能请求 20 条结果，防止一次查询过多数据
	MaxSearchTopK = 20
)

type SearchService struct {
	// 将查询文本转换为向量。query 也必须经过与文档 Chunk 相同的 Embedding 模型
	embeddingClient embedding.EmbeddingClient

	// 负责向 Milvus 查询 TopK相似向量
	vectorRepository *repository.VectorRepository

	//根据 Milvus 返回的 chunk_id 从 MySQL 取正文
	chunkRepository *repository.DocumentChunkRepository
}

func NewSearchService(
	embeddingClient embedding.EmbeddingClient,
	vectorRepository *repository.VectorRepository,
	chunkRepository *repository.DocumentChunkRepository,
) *SearchService {
	return &SearchService{
		embeddingClient:  embeddingClient,
		vectorRepository: vectorRepository,
		chunkRepository:  chunkRepository,
	}
}

func (s *SearchService) Search(
	ctx context.Context,
	query string,
	topK int,
) ([]model.SearchResult, error) {
	//如果query为空：
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	//用户不传topK范围或者topK超出范围:
	if topK == 0 {
		topK = DefaultSearchTopK
	}

	if topK < 1 || topK > MaxSearchTopK {
		return nil, fmt.Errorf("top_k must be between 1 and %d", MaxSearchTopK)
	}

	//1.Query 文本 → Embedding
	queryVector, err := s.embeddingClient.Embed(
		ctx,
		query,
	)
	if err != nil {
		return nil, fmt.Errorf("embed query failed: %w", err)
	}

	//2.在 Milvus 中执行向量搜索
	hits, err := s.vectorRepository.Search(
		ctx,
		queryVector,
		topK,
	)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	//如果 Milvus 返回的结果集为空（没有找到相似向量）
	if len(hits) == 0 {
		//HTTP JSON 返回 []，而不是 null
		return []model.SearchResult{}, nil
	}

	// 3. 提取 Milvus 返回的 chunk_id
	//创建一个 []uint 切片 chunkIDs，初始长度 0，容量为 len(hits)
	chunkIDs := make([]uint, 0, len(hits))

	//遍历 hits，对于每个 hit，如果 ChunkID > 0，则将其转换为 uint 并追加到 chunkIDs
	//这样得到所有有效的 chunk ID 列表，用于下一步 MySQL 查询
	for _, hit := range hits {
		if hit.ChunkID > 0 {
			chunkIDs = append(chunkIDs, uint(hit.ChunkID))
		}
	}

	//4. 一次性从 MySQL 查询正文
	//调用 chunkRepository.FindByIDs(chunkIDs)，传入 chunk ID 列表
	chunks, err := s.chunkRepository.FindByIDs(chunkIDs)

	if err != nil {
		return nil, fmt.Errorf("find chunks from mysql failed: %w", err)
	}

	//注：MySQL 的 IN 查询不保证返回结果的顺序与 IN 列表中 ID 的顺序一致
	//例如：SELECT * FROM document_chunks WHERE id IN (31, 82, 17);
	//传入的 ID 顺序是 [31, 82, 17]，但 MySQL 可能返回 [17, 31, 82]的结果，如果直接遍历MySQL返回的结果，会导致搜索结果乱序，最相似的内容可能排在后面

	//解决方案：虽然MySQL返回结果无序，但是向量数据库的返回结果hits是有序的
	// 把 MySQL 返回的 chunks 转换成一个 map，以 chunk.ID 为键，DocumentChunk 对象为值
	//重新遍历 hits（它是有序的），对于每个 hit，用 hit.ChunkID 去 chunkMap 中取对应的 chunk，然后组装成 SearchResult
	chunkMap := make(map[uint]model.DocumentChunk, len(chunks))

	for _, chunk := range chunks {
		chunkMap[chunk.ID] = chunk
	}

	//5. 再严格按照 Milvus 相似度排序组装最终结果
	//遍历 hits（此时 hits 已经按照相似度从高到低排列，由 Milvus 保证）
	/*
		对于每个 hit：
		跳过 ChunkID <= 0 的无效项（防御）。
		将ChunkID 转换为 uint。
		在 chunkMap 中查找该 ID 对应的 chunk。如果不存在（可能已被删除），跳过。
		将 chunk.ID、hit.Score、chunk.Content 组装成 model.SearchResult 并追加到 results
	*/
	results := make([]model.SearchResult, 0, len(hits))

	for _, hit := range hits {
		if hit.ChunkID <= 0 {
			continue
		}

		chunkID := uint(hit.ChunkID)

		chunk, exists := chunkMap[chunkID]
		if !exists {
			continue
		}

		results = append(
			results,
			model.SearchResult{
				ChunkID: chunk.ID,
				Score:   hit.Score,
				Content: chunk.Content,
			},
		)
	}

	return results, nil
}
