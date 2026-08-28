// 搜索服务（SearchService）。
//
// Module 8：
// Query -> Embedding -> Milvus -> MySQL Chunk
//
// Module 9：
// 在原有结果基础上继续补充：
// DocumentID
// FileName
// ChunkIndex
//
// 为后面的RAG Sources做准备。
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
	// 用户不传top_k时默认5。
	DefaultSearchTopK = 5

	// 防止一次查询过多Chunk。
	MaxSearchTopK = 20
)

type SearchService struct {

	// Query -> 1024维向量。
	embeddingClient embedding.EmbeddingClient

	// Milvus TopK搜索。
	vectorRepository *repository.VectorRepository

	// 根据chunk_id获取正文。
	chunkRepository *repository.DocumentChunkRepository

	// Module 9新增：
	// 根据DocumentID获取PDF文件名。
	documentRepository *repository.DocumentRepository
}

// SearchService构造函数。
func NewSearchService(
	embeddingClient embedding.EmbeddingClient,
	vectorRepository *repository.VectorRepository,
	chunkRepository *repository.DocumentChunkRepository,

	// Module 9新增。
	documentRepository *repository.DocumentRepository,

) *SearchService {

	return &SearchService{
		embeddingClient: embeddingClient,

		vectorRepository: vectorRepository,

		chunkRepository: chunkRepository,

		documentRepository: documentRepository,
	}
}

func (s *SearchService) Search(
	ctx context.Context,
	query string,
	topK int,
) ([]model.SearchResult, error) {

	// ============================
	// 0. 参数检查
	// ============================

	query = strings.TrimSpace(query)

	if query == "" {
		return nil, fmt.Errorf(
			"query cannot be empty",
		)
	}

	// 用户没有传：
	//
	// "top_k"
	//
	// Go int零值就是0。
	if topK == 0 {
		topK = DefaultSearchTopK
	}

	if topK < 1 || topK > MaxSearchTopK {
		return nil, fmt.Errorf(
			"top_k must be between 1 and %d",
			MaxSearchTopK,
		)
	}

	// ============================
	// 1. Query -> Embedding
	// ============================

	queryVector, err := s.embeddingClient.Embed(
		ctx,
		query,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"embed query failed: %w",
			err,
		)
	}

	// ============================
	// 2. Milvus TopK
	// ============================

	hits, err := s.vectorRepository.Search(
		ctx,
		queryVector,
		topK,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"vector search failed: %w",
			err,
		)
	}

	if len(hits) == 0 {
		return []model.SearchResult{}, nil
	}

	// ============================
	// 3. 提取Chunk ID
	// ============================

	chunkIDs := make(
		[]uint,
		0,
		len(hits),
	)

	for _, hit := range hits {

		if hit.ChunkID > 0 {

			chunkIDs = append(
				chunkIDs,
				uint(hit.ChunkID),
			)
		}
	}

	// ============================
	// 4. MySQL查询Chunk
	// ============================

	chunks, err := s.chunkRepository.FindByIDs(
		chunkIDs,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"find chunks from mysql failed: %w",
			err,
		)
	}

	// MySQL IN查询不能保证顺序。
	//
	// 因此先：
	//
	// chunk.ID -> chunk
	//
	chunkMap := make(
		map[uint]model.DocumentChunk,
		len(chunks),
	)

	for _, chunk := range chunks {
		chunkMap[chunk.ID] = chunk
	}

	// ============================
	// 5. Module 9：
	// 收集Document ID
	// ============================

	// 一个TopK结果可能是：
	//
	// chunk1 -> document3
	// chunk2 -> document3
	// chunk3 -> document7
	//
	// document3出现了两次。
	//
	// 因此要去重。

	documentIDSet := make(
		map[uint]struct{},
	)

	documentIDs := make(
		[]uint,
		0,
	)

	for _, chunk := range chunks {

		// 已经记录过这个document_id。
		if _, exists := documentIDSet[chunk.DocumentID]; exists {
			continue
		}

		// 标记已经出现。
		documentIDSet[chunk.DocumentID] = struct{}{}

		documentIDs = append(
			documentIDs,
			chunk.DocumentID,
		)
	}

	// ============================
	// 6. 一次查询PDF元数据
	// ============================

	documents, err := s.documentRepository.FindByIDs(
		documentIDs,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"find documents from mysql failed: %w",
			err,
		)
	}

	// 同样转换：
	//
	// DocumentID -> Document
	documentMap := make(
		map[uint]model.Document,
		len(documents),
	)

	for _, document := range documents {
		documentMap[document.ID] = document
	}

	// ============================
	// 7. 按Milvus顺序组装结果
	// ============================

	results := make(
		[]model.SearchResult,
		0,
		len(hits),
	)

	// 注意：
	//
	// 一定继续遍历 hits，
	// 不能直接遍历MySQL chunks。
	//
	// 因为Milvus hits才是按照
	// 相似度排序的。

	for _, hit := range hits {

		if hit.ChunkID <= 0 {
			continue
		}

		// 根据Milvus的chunk_id
		// 找MySQL Chunk。
		chunk, exists := chunkMap[uint(hit.ChunkID)]

		if !exists {
			continue
		}

		// 根据Chunk中的DocumentID
		// 找PDF Document。
		document, exists := documentMap[chunk.DocumentID]

		if !exists {

			// 极端情况下文档恰好被并发删除，
			// 就不要返回失去来源的Chunk。
			continue
		}

		// 最终结果。
		results = append(
			results,
			model.SearchResult{

				ChunkID: chunk.ID,

				DocumentID: chunk.DocumentID,

				FileName: document.FileName,

				ChunkIndex: chunk.ChunkIndex,

				Score: hit.Score,

				Content: chunk.Content,
			},
		)
	}

	return results, nil
}
