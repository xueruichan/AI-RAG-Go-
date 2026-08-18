/*
model：数据长什么样
例：Document有哪些字段

document.go:要创建的数据表以及字段

由db.AutoMigrate(&model.Document{})自动创建
*/

package model

import(
	"time"
)

type Document struct {
	//标签：1.gorm:"primaryKey：告诉gorm主键；2.json:id：告诉JSON，字段ID对应JSON里面的id
	ID        uint      `gorm:"primaryKey" json:"id"`
	FileName  string    `gorm:"size:255;not null" json:"filename"`
	FilePath  string    `gorm:"size:500" json:"filepath"`
	Status    string    `gorm:"size:32;not null;default:pending;index" json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}