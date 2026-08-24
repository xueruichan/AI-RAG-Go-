package repository

import (
	"gorag/internal/model"
	"gorm.io/gorm"
)

type DocumentChunkRepository struct {
	db *gorm.DB
}

func NewDocumentChunkRepository(db *gorm.DB) *DocumentChunkRepository {
	return &DocumentChunkRepository{
		db: db,
	}
}

// CreateBatch 一次保存一篇文档的所有 Chunk
func (r *DocumentChunkRepository) CreateBatch(chunks []model.DocumentChunk) error {
	if len(chunks) == 0 {
		return nil
	}

	//gorm对数据库的批量插入操作
	//将 chunks 切片里的所有记录一次性插入到数据库中
	//r.db提供了各种操作数据库的方法
	//指针传递： GORM 内部需要修改传入的数据
	return r.db.Create(&chunks).Error
}

func (r *DocumentChunkRepository) DeleteByDocumentID(documentID uint) error {
	//Where同样是GORM 提供的方法，用来给数据库操作添加条件。其中?是占位符
	//Delete 是 GORM 中执行删除操作的方法，需要传入一个模型对象的指针（通常是空指针），告诉GORM操作哪张表
	//{}:创建一个该结构体的实例，并且所有字段都使用零值初始化
	return r.db.Where("document_id=?", documentID).Delete(&model.DocumentChunk{}).Error
}


//搜索 Milvus 后只会得到chunk_id而正文在MySQL中还要进行查询
//FindByIDs 根据一组 Chunk ID 批量查询正文
// Milvus 返回：
// [31, 82, 17]
//
// 然后一次 SQL 查询：
// WHERE id IN (31, 82, 17)

//输入:[]uint 类型的切片，包含多个 chunk ID  输出：一个 []model.DocumentChunk 切片，包含这些 ID 对应的完整 chunk 记录
func (r *DocumentChunkRepository) FindByIDs(ids []uint) ([]model.DocumentChunk,error){
	//如果传入的 ID 切片为空，没必要查询数据库，直接返回一个空切片和 nil 错误
	if len(ids)==0{
		return []model.DocumentChunk{},nil
	}

	//用于接收查询结果
	var chunks []model.DocumentChunk

	if err:=r.db.Where("id IN ?",ids).Find(&chunks).Error; err!=nil{
		return nil,err
	}

	return chunks,nil
}