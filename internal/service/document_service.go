/*
service:业务逻辑

用户上传文档
↓
检查文件是否合法
↓
创建记录
↓
触发解析任务
↓
生成 Embedding
*/

// Service 本身不存数据，它只持有 Repository 的引用：Repository 提供的基础 CRUD（增删改查）能力，包装成面向业务场景的方法
package service

import (
	"context"
	"fmt"
	"gorag/internal/model"
	"gorag/internal/rag"
	"gorag/internal/repository"
	"gorag/pkg/embedding"
	"time"
)

// service能够持有的数据仓库
type DocumentService struct {
	repository       *repository.DocumentRepository
	chunkRepository  *repository.DocumentChunkRepository
	cacheRepository  *repository.DocumentCacheRepository
	embeddingClient  embedding.EmbeddingClient    //embedding.EmbeddingClient 接口，用于将文本转换为向量
	vectorRepository *repository.VectorRepository //类型是 *repository.VectorRepository，用于操作 Milvus 向量数据库，包括插入、搜索、删除向量。
}

// 注：构造函数不写接收者
func NewDocumentService(
	repository *repository.DocumentRepository,
	chunkRepository *repository.DocumentChunkRepository,
	cacheRepository *repository.DocumentCacheRepository,
	embeddingClient embedding.EmbeddingClient,
	vectorRepository *repository.VectorRepository,
) *DocumentService {
	return &DocumentService{
		repository:       repository,
		chunkRepository:  chunkRepository,
		cacheRepository:  cacheRepository,
		embeddingClient:  embeddingClient,
		vectorRepository: vectorRepository,
	}
}

// 收到了handler层的文件名，和文件路径
func (s *DocumentService) Create(fileName string, filePath string) (*model.Document, error) {
	//model.Document是model层创建的结构体，调用之后创建了对象，并将文件名填进去，并绑定status，此时FilePath还是空的
	document := &model.Document{
		FileName: fileName,
		FilePath: filePath,
		Status:   "UPLOADED",
	}

	//错误判断通过后又调用了repository的create方法
	err := s.repository.Create(document)
	if err != nil {
		return nil, err
	}
	return document, nil
}

func (s *DocumentService) List() ([]model.Document, error) {
	return s.repository.FindAll()
}

// 引入Redis后查找逻辑发生变化
func (s *DocumentService) GetByID(id uint) (*model.Document, error) {
	//1.先查Redis
	cachedDocument, found, err := s.cacheRepository.Get(id)

	if err == nil && found {
		fmt.Printf("cache hit: document:%d\n", id)
		return cachedDocument, nil
	}

	//Redis 出错不能影响 MySQL 主业务
	if err != nil {
		fmt.Printf("redis cache get failed: %v, fallback to mysql\n", err)
	} else {
		fmt.Printf("cache miss: document:%d\n", id)
	}

	// 2. Redis Miss → MySQL
	document, err := s.repository.FindByID(id)

	if err != nil {
		return nil, err
	}
	if document == nil {
		return nil, nil
	}

	// 3. MySQL 查到了 → 回填 Redis,TTL=5min
	if err := s.cacheRepository.Set(document, 5*time.Minute); err != nil {
		//redis写入失败，但是MySQL查询到的数据是正确的，不能让整个请求失败
		fmt.Printf("redis cache set failed: %v\n", err)
	}

	return document, nil
}

// error:就像int一样可以直接用，是一种类型
// 同理引入Redis后也要删除Redis中的缓存
func (s *DocumentService) Delete(id uint) (bool, error) {
	//0.删除缓存
	if err := s.cacheRepository.Delete(id); err != nil {
		fmt.Printf("delete document cache failed: %v\n", err)
	}

	//1.删除文档时同时删除 Milvus 向量,否则会出现数据不一致：能搜到 chunk_id，但是 MySQL 查不到正文

	//带有 10 秒超时的上下文，用于控制删除 Milvus 向量的操作时间，避免长时间阻塞
	vectorCtx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	err := s.vectorRepository.DeleteByDocumentID(
		vectorCtx,
		id,
	)
	//通知上下文可以释放相关资源
	cancel()

	if err != nil {
		return false, fmt.Errorf("delete document vectors failed: %w", err)
	}

	//2.先删除文档的chunk
	if err := s.chunkRepository.DeleteByDocumentID(id); err != nil {
		return false, err
	}

	//3.删除文档
	return s.repository.DeleteByID(id)
}

// 文档处理业务
func (s *DocumentService) ProcessDocument(document *model.Document) ([]string, error) {
	//1.从服务器的磁盘上的pdf提取文本
	text, err := rag.ExtractText(document.FilePath)
	if err != nil {
		return nil, fmt.Errorf("extract document text failed:%w", err)
	}

	//2.把提取的text切分为chunk
	// Chunk Size = 500
	// Overlap    = 50
	chunks := rag.SplitText(text, 500, 50)

	//3.提取不到文本认为处理失败
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no text extracted from document")
	}

	//在原有“提取文本 → 分块 → 存 MySQL”的基础上，增加了 Embedding 生成和 Milvus 写入。
	//module8:每个 Chunk 生成对应的 Embedding
	//例如:chunks[0]  <-> vectors[0]   chunks[1]  <-> vectors[1]

	//创建一个二维切片 vectors，长度等于 chunks 数量,用来保存每个 chunk 对应的 1024 维向量的二维切片
	vectors := make([][]float32, len(chunks))
	for i, content := range chunks {
		embedCtx, cancel := context.WithTimeout(
			context.Background(),
			30*time.Second,
		)

		vector, err := s.embeddingClient.Embed(
			embedCtx,
			content,
		)

		cancel()

		if err != nil {
			return nil, fmt.Errorf("embed chunk %d failed: %w", i, err)
		}
		vectors[i] = vector
	}

	//4.把切分好的chunk全部存到数据库中的表里面
	chunkModels := make(
		[]model.DocumentChunk,
		0,
		len(chunks),
	)

	for index, content := range chunks {
		chunkModels = append(
			chunkModels,
			model.DocumentChunk{
				DocumentID: document.ID,
				Content:    content,
				ChunkIndex: index,
			},
		)
	}

	if err := s.chunkRepository.CreateBatch(chunkModels); err != nil {
		return nil, fmt.Errorf("save document chunks failed: %w", err)
	}

	//从 MySQL 写入后的 Chunk 中获取chunk ID 和 document ID
	//Milvus 不自己生成 chunk_id
	//Milvus设计：chunk_id，document_id，embedding

	//创建 []int64 切片，长度等于 chunk 数量，用于存储每个 chunk 的 ID（从 MySQL 回填的 ID）
	chunkIDs := make([]int64, len(chunkModels))
	//创建 []int64 切片，长度相同，用于存储每个 chunk 所属的文档 ID
	documentIDs := make([]int64, len(chunkModels))

	for i := range chunkModels {
		if chunkModels[i].ID == 0 {
			return nil, fmt.Errorf("chunk %d has no id after mysql insert", i)
		}

		chunkIDs[i] = int64(chunkModels[i].ID)
		documentIDs[i] = int64(document.ID)
	}

	//写入 Milvus
	// 三组数据严格一一对应：
	// chunkIDs[i]
	// documentIDs[i]
	// vectors[i]

	//为 Milvus 插入操作创建 30 秒超时上下文
	insertCtx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)

	err = s.vectorRepository.InsertChunks(
		insertCtx,
		chunkIDs,
		documentIDs,
		vectors,
	)

	cancel()

	// 此时 MySQL Chunk 已经写成功,但是 Milvus 失败,同理要保证数据一致性不能直接return，否则MySQL有chunk，Milvus没有向量
	if err != nil {
		//调用将MySQL和Milvus回滚的函数
		s.rollbackProcessedChunks(
			document.ID,
		)

		return nil, fmt.Errorf("save vectors to milvus failed: %w", err)
	}

	return chunks, nil
}

func (s *DocumentService) rollbackProcessedChunks(documentID uint) {
	//1.删除已经写入MySQL的chunk
	if err := s.chunkRepository.DeleteByDocumentID(documentID); err != nil {
		fmt.Printf("rollback mysql chunks failed, document=%d: %v\n", documentID, err)
	}

	//2.删除Milvus 中可能已经写入的向量
	//即使 InsertChunks 返回错误，也不能完全假设 Milvus 一条都没写进去
	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)

	defer cancel()

	if err := s.vectorRepository.DeleteByDocumentID(ctx, documentID); err != nil {
		fmt.Printf("rollback milvus vectors failed, document=%d: %v\n", documentID, err)
	}

}

// UpdateStatus 更新 Document 的业务状态。
// 引入Redis之后，只要修改了文档状态就要删除Redis中对应的，否则会导致脏数据,因为MySQL中状态更新了Redis中就是旧的了
func (s *DocumentService) UpdateStatus(id uint, status string) error {
	if err := s.repository.UpdateStatus(id, status); err != nil {
		return err
	}

	//删除Redis里面的旧文档
	if err := s.cacheRepository.Delete(id); err != nil {
		fmt.Printf("delete document cache failed: %v\n", err)
	}
	return nil
}
