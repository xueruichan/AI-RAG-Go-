package worker

/*
 DocumentTask 表示一个待处理的任务
 为什么这里只保存 DocumentID？
 因为 documents 表中已经有：
	FileName
	FilePath
	Status

   Worker 拿到 DocumentID 后，
   可以通过 DocumentService.GetByID()
   再查询完整的 Document。

   因此 Task 本身保持非常轻量。
*/

type DocumentTask struct {
	DocumentID uint
}
