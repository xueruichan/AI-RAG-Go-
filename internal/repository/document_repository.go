/*
repository：数据库
Create
Find
Delete
*/

package repository

import (
	"errors"

	"gorag/internal/model"

	"gorm.io/gorm"
)

type DocumentRepository struct {
	//gorm.DB 是 GORM 库定义的核心结构体，它内部封装了：
	/*
		数据库连接池（*sql.DB）。
		当前的 SQL 语句构建器状态。
		日志记录器。
		因此，DocumentRepository 持有这个指针，本质上就是持有了操作数据库所需的全部上下文和网络连接资源。
		r.db就是数据库
	*/
	db *gorm.DB
}

// new开头为构造函数的习惯
func NewDocumentRepository(db *gorm.DB) *DocumentRepository {
	return &DocumentRepository{
		db: db,
	}
}

// INSERT INTO documents ...插入
func (r *DocumentRepository) Create(document *model.Document) error {
	//终于将service收到的文件名在repository这一层写入了数据库MySQL，id由数据库自动生成并原路返回
	return r.db.Create(document).Error
}

func (r *DocumentRepository) FindAll() ([]model.Document, error) {
	var documents []model.Document
	//把数据库查询出来的很多记录填到documents里面，返回的documents是一个切片表示不止一条记录
	err := r.db.Order("id DESC").Find(&documents).Error
	return documents, err
}

func (r *DocumentRepository) FindByID(id uint) (*model.Document, error) {
	var document model.Document
	//GORM 的 First 用于取得单条记录，如果没有记录会产生 ErrRecordNotFound
	/*
		SELECT *
		FROM documents
		WHERE id = 1
		LIMIT 1;
	*/
	err := r.db.First(&document, id).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		//nil,nil:数据库没坏，只是这个 document 不存在
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &document, nil
}

// 为了module9实现RAG，新增批量查询的方法
// 拿到一批 chunk 后，需要根据每个 chunk 的 DocumentID 去查询 documents 表，获取文件名等信息。
// 如果对每个 DocumentID 都执行一次 SQL 查询，效率很低
func (r *DocumentRepository) FindByIDs(ids []uint) ([]model.Document, error) {
	// 空列表没有必要访问MySQL
	if len(ids) == 0 {
		return []model.Document{}, nil
	}

	//接收查询结果
	var documents []model.Document

	if err := r.db.Where("id IN ?", ids).Find(&documents).Error; err != nil {
		return nil, err
	}

	return documents, nil
}

func (r *DocumentRepository) DeleteByID(id uint) (bool, error) {
	//数据库按照id进行删除
	result := r.db.Delete(&model.Document{}, id)

	if result.Error != nil {
		return false, result.Error
	}

	return result.RowsAffected > 0, nil
}

// UpdateStatus 只更新某个 Document 的 status 字段。
func (r *DocumentRepository) UpdateStatus(id uint, status string) error {
	//Model 是 GORM 提供的一个方法，用于指定当前操作要作用的模型,即要操作的数据表
	//Where 方法用于添加 SQL 的 WHERE 条件
	//Update 是 GORM 中执行更新操作的方法，用于更新单个字段
	return r.db.Model(&model.Document{}).Where("id=?", id).Update("status", status).Error
}
