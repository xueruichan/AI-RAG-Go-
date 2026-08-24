//VectorRepository 封装与Milvus向量数据库的数据访问操作
/*
返回的搜索结果是 VectorHit 结构体，仅包含 ChunkID 和 Score
后续 Service 层再根据 ChunkID 去 MySQL 查询实际文本内容，实现了存储层与向量检索层的分离。
*/
package repository
import (
	"context"
	"fmt"

	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

// 加上 v1 的原因：
// 以后如果我们修改 Embedding 模型或者向量维度，
// 可以创建 gorag_document_chunks_v2，避免和旧 schema 冲突
const MilvusCollectionName="gorag_document_chunks_v1"
//常量组，一次定义多个常量
const(
	fieldChunkID="chunk_id"
	fieldDocumentID="document_id"
	fieldEmbedding="embedding"
)

//VectorHit 是 Milvus 搜索返回给 Service 层的最小结果
//Milvus Repository 不负责去 MySQL 查询正文,只返回chunk_id + 相似度 score
// 和 DocumentRepository 的思想一样：
//
// Service
//    ↓
// VectorRepository
//    ↓
// Milvus

//VectorHit：表示一次搜索命中的结果并返回
//Milvus 仓库层 返回给 Service 的中间结果
type VectorHit struct{
	ChunkID int64
	Score float32
}

//VectorRepository：Milvus 数据访问层的主体，持有所有与 Milvus 交互所需的配置和连接，并提供一系列方法，执行具体的数据操作
type VectorRepository struct{
	client *milvusclient.Client
	collectionName string
	dimension int
}

func NewVectorRepository(client *milvusclient.Client,dimension int) *VectorRepository{
	return &VectorRepository{
		client: client,
		collectionName: MilvusCollectionName,
		dimension: dimension,
	}
}

//EnsureCollection:确保 Milvus 中用于存储文档块向量的 Collection已经存在，并且已经加载到内存，可以正常执行插入和搜索操作。
func (r *VectorRepository) EnsureCollection(ctx context.Context) error{
	if r.dimension<=0{
		return fmt.Errorf("invalid embedding dimension: %d",r.dimension)
	}

	/*
	关于为什么collection需要检查存在性：
	Milvus 的 Collection 必须先创建、再建索引、最后加载到内存才能用于搜索
	为了在应用启动时自动保证这个准备过程完成，并适配不同环境（如首次运行或后续重启）
	就需要先检查 Collection 是否存在，再决定是创建还是直接加载。
	而MySQL已经通过GORM封装好了，但是milvus没有
	*/
	//1. 检查 Collection 是否已经存在。
	//HasCollection 是 Milvus Go 客户端提供的一个方法，用于查询某个 Collection 是否存在
	exists,err:=r.client.HasCollection(ctx,milvusclient.NewHasCollectionOption(r.collectionName))
	if err!=nil{
		return fmt.Errorf("check milvus collection failed: %w",err)
	}

	//2. 第一次运行项目时自动创建 Collection
	//exists=false
	//定义 Milvus Collection的结构
	/*
	定义了三个字段：

	chunk_id：主键，Int64 类型，需手动指定 ID。

	document_id：Int64 类型，关联文档。

	embedding：浮点向量，维度由 r.dimension 指定
	*/
	if !exists{
		schema := entity.NewSchema().
			WithDynamicFieldEnabled(false).
			WithField(
				entity.NewField().
					WithName(fieldChunkID).
					WithDataType(entity.FieldTypeInt64).
					WithIsPrimaryKey(true).
					WithIsAutoID(false),
			).
			WithField(
				entity.NewField().
					WithName(fieldDocumentID).
					WithDataType(entity.FieldTypeInt64),
			).
			WithField(
				entity.NewField().
					WithName(fieldEmbedding).
					WithDataType(entity.FieldTypeFloatVector).
					WithDim(int64(r.dimension)),
			)
		    
			//添加索引，向量数据库中有大量数据时必须用索引来加速搜索
			indexOption:=milvusclient.NewCreateIndexOption(
				//在 embedding 字段上建一个索引，用自动索引算法，相似度用余弦来计算
				r.collectionName,
				fieldEmbedding,
				index.NewAutoIndex(entity.COSINE),
			)

			//创建collection
			err=r.client.CreateCollection(
				ctx,
				milvusclient.NewCreateCollectionOption(
					//指定集合名字
					r.collectionName,
					//指定集合结构
					schema,
					//把上一步创建的索引选项附加到集合创建请求中,在创建表的同时，Milvus 就会对 embedding 字段建立索引
				).WithIndexOptions(indexOption).
				  //设置一致性级别为“强一致性”
				  WithConsistencyLevel(entity.ClStrong),
			)
			if err!=nil{
				return fmt.Errorf("create milvus collection failed: %w",err)
			}
	}

	//等待 Milvus 把集合（Collection）真正加载到内存中，加载完成后才能进行搜索
	//向量搜索的核心操作是计算查询向量与数据库中大量向量的距离（如余弦相似度）。
	// 如果这些向量还在磁盘上，每次搜索都要从磁盘读入、计算，速度极慢
	//r.client.LoadCollection：调用 Milvus 客户端，要求加载名为 r.collectionName 的集合
	loadTask,err:=r.client.LoadCollection(
		ctx,
		//milvusclient.NewLoadCollectionOption(r.collectionName)：创建一个“加载选项”，告诉 Milvus 要加载哪个集合。
		milvusclient.NewLoadCollectionOption(
			r.collectionName,
		),
	)
	if err!=nil{
		return fmt.Errorf("load milvus collection failed: %w",err)
	}

	//loadTask.Await(ctx)：等待刚才发起的加载任务真正完成
	if err:=loadTask.Await(ctx); err!=nil{
		return fmt.Errorf("wait milvus collection load failed: %w",err)
	}
	return nil
}


//批量写入文档块（chunk）的向量数据到 Milvus
//收三个切片：chunkIDs（每个块的 ID）、documentIDs（块所属文档的 ID）、vectors（每个块对应的向量），然后将它们作为三个列一次性写入 Milvus 集合中
func (r *VectorRepository) InsertChunks(
	ctx context.Context,
	//存放每个 chunk 的主键 ID
	chunkIDs []int64,
	//存放每个 chunk 所属文档的 ID
	documentIDs []int64,
	//二维切片：外层表示不同的chunk，内层表示该chunk的多个维度（例如1024等）
	vectors [][]float32,
) error{
	if len(chunkIDs)==0{
		return nil
	}

	//三列行数必须一致，都表示有多少个chunk
	if len(chunkIDs)!=len(documentIDs)||len(chunkIDs)!=len(vectors){
		return fmt.Errorf(
			"milvus insert column length mismatch: chunk_ids=%d document_ids=%d vectors=%d",
			len(chunkIDs),
			len(documentIDs),
			len(vectors),
		)
	}
	//每一个向量都必须严格等于 Collection 定义的维度
	for i,vector:=range vectors{
		if len(vector)!=r.dimension{
			return fmt.Errorf(
				"vector %d dimension mismatch: expected=%d actual=%d",
				i,
				r.dimension,
				len(vector),
			)
		}
	}
	
	//调用 Milvus 客户端的 Insert 方法执行插入
	_,err:=r.client.Insert(
		ctx,
		//NewColumnBasedInsertOption(r.collectionName)：指定要插入的集合名
		milvusclient.NewColumnBasedInsertOption(
			r.collectionName,
		).
			//后面3个都为添加对应的列，并指定字段名和数据
			//添加一个 Int64 列，字段名是 chunk_id，数据是 chunkIDs。
			WithInt64Column(
				fieldChunkID,
				chunkIDs,
			).
			WithInt64Column(
				fieldDocumentID,
				documentIDs,
			).
			//字段名是 embedding，维度为 r.dimension，数据是 vectors
			WithFloatVectorColumn(
				fieldEmbedding,
				r.dimension,
				vectors,
			),
	)

	if err!=nil{
		return fmt.Errorf("insert vectors into milvus failed: %w",err)
	}

	return nil

}


// Search 在 Milvus 中执行 TopK 向量搜索:即找出与目标向量vector最接近的k个向量
//返回两个值：[]VectorHit（命中结果列表）和 error
func (r *VectorRepository) Search(
	ctx context.Context,
	vector []float32,
	topk int,
) ([]VectorHit,error) {
	if len(vector)!=r.dimension{
		return nil,fmt.Errorf(
			"query vector dimension mismatch: expected=%d actual=%d",
			r.dimension,
			len(vector),
		)
	}

	if topk<=0{
		return nil,fmt.Errorf("topK must be greater than zero")
	}

	//调用 Milvus 客户端的 Search 方法执行向量搜索
	resultSets,err:=r.client.Search(
		ctx,
		//NewSearchOption(r.collectionName, topK, []entity.Vector{...})：指定集合名、返回数量 topK、以及要查询的向量列表(这里只传一个向量，所以用切片包一个)
		milvusclient.NewSearchOption(
			r.collectionName,
			topk,
			[]entity.Vector{
				entity.FloatVector(vector),
			},
		).
			//WithANNSField(fieldEmbedding)：指定在 embedding 字段上执行近似最近邻搜索
			WithANNSField(fieldEmbedding).
			WithConsistencyLevel(entity.ClStrong),
	)
	if err!=nil{
		return nil,fmt.Errorf("search milvus failed: %w",err)
	}

	//如果搜索请求出错，返回错误
	//一次只传入了一个 Query Vector,所以正常情况下只有一个 ResultSet
	//结果集为空则返回空切片
	if len(resultSets)==0{
		return []VectorHit{},nil
	}

	resultSet:=resultSets[0]
	if resultSet.Err!=nil{
		return nil,fmt.Errorf("milvus result contains error: %w",resultSet.Err)
	}

	//获取结果数量。如果为 0，说明没有找到相似向量，返回空切片
	//resultSet是接收resultSets切片的第一个元素，而它本身是一个结构体里面有很多字段和方法，其中就包括Len()方法，这个方法能够统计出本次获取的相似向量的个数
	resultCount:=resultSet.Len()
	if resultCount==0{
		return []VectorHit{},nil
	}

	//检查结果中的主键 ID 列表是否存在。如果不存在，返回错误
	if resultSet.IDs==nil{
		return nil,fmt.Errorf("milvus search returned no primary keys")
	}

	//检查分数列表的长度是否足够。正常情况下，分数数量应该与结果数量一致
	if len(resultSet.Scores)<resultCount{
		return nil,fmt.Errorf("milvus scores count mismatch")
	}

	//将 Milvus 的原始结果转换成我们自定义的、业务友好的结构体切片返还给Service层
	//创建一个 VectorHit 切片，长度 0，容量为 resultCount，用来存放最终结果
	hits:=make([]VectorHit,0,resultCount)

	for i:=0;i<resultCount;i++{
		//将 chunkID 和对应的分数 resultSet.Scores[i] 封装成 VectorHit，追加到 hits 中
		chunkID,err:=resultSet.IDs.GetAsInt64(i)
		if err!=nil{
			return nil,fmt.Errorf("read milvus chunk id failed: %w",err)
		}

		hits=append(hits, VectorHit{
			ChunkID: chunkID,
			Score: resultSet.Scores[i],
		})
	}

	return hits,nil
}


//删除某个文档在 Milvus 中对应的所有向量
//和Redis删除同理：
//当用户删除文档时，不仅要删除 MySQL 中的 chunk 记录，也要删除 Milvus 中这些 chunk 的向量，避免留下“幽灵向量”

func (r *VectorRepository) DeleteByDocumentID(
	ctx context.Context,
	documentID uint,
) error{
	if documentID==0{
		return nil
	}

	//构造一个删除过滤表达式，类似于 SQL 的 WHERE 子句
	//fieldDocumentID 是常量 "document_id"，所以生成的表达式类似 document_id == 15
	expr:=fmt.Sprintf("%s == %d",fieldDocumentID,documentID)

	//Delete 返回两个值，第一个是删除结果（这里忽略），第二个是错误 err
	_,err:=r.client.Delete(
		ctx,
		milvusclient.NewDeleteOption(
			r.collectionName,
			//WithExpr(expr)：指定过滤表达式，只删除满足条件的实体
		).WithExpr(expr),
	)
	if err!=nil{
		return fmt.Errorf("delete document vectors from milvus failed: %w",err)
	}

	return nil
}
