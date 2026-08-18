package model

type DocumentChunk struct {
	ID uint `gorm:"primaryKey" json:"id"`
	//属于哪一个PDF
	DocumentID uint `gorm:"not null;index" json:"document_id"`
	//mysql中使用long text
	Content    string `gorm:"type:longtext;not null" json:"content"`
	ChunkIndex int    `gorm:"not null" json:"chunk_index"`
}
