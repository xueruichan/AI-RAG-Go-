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
