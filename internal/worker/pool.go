package worker

import (
	"fmt"
	"gorag/internal/service"
	"sync"
)

// 定义工作池用于调度
type Pool struct {
	/*
		taskQueue 是后台任务队列
		Handler 是生产者:把 DocumentTask 放进来
		Worker 是消费者：从这里不断取 DocumentTask
	*/
	taskQueue chan DocumentTask
	//设置worker数量
	workerCount int
	//Worker 真正处理 Document 时需要调用业务层
	documentService *service.DocumentService

	//wg 用来等待所有 Worker goroutine 退出,实现优雅关闭
	/*
		Add(n int)	将计数器增加 n，表示需要等待 n 个 goroutine
		Done()	将计数器减 1，通常配合 defer 使用，表示一个 goroutine 已完成
		Wait()	阻塞当前 goroutine，直到计数器变为 0
	*/
	wg sync.WaitGroup
}

func NewPool(workerCount int, queueSize int, documentService *service.DocumentService) *Pool {
	return &Pool{
		//通过make来创建channel并作为阻塞队列传给结构体中的taskQueue
		taskQueue:       make(chan DocumentTask, queueSize),
		workerCount:     workerCount,
		documentService: documentService,
	}
}

// Start 启动所有 Worker。
//
// 假设 workerCount = 4：
//
// main goroutine
//      │
//      ├── go Worker1
//      ├── go Worker2
//      ├── go Worker3
//      └── go Worker4

func (p *Pool) Start() {
	for workerID := 1; workerID <= p.workerCount; workerID++ {
		//启动worker
		p.wg.Add(1)
		//必须使用 go。否则第一个 Worker 会一直阻塞等待 Task，后面的 Worker 和 Gin Server 都无法继续启动
		go p.runWorker(workerID)
	}
}

func (p *Pool) Submit(task DocumentTask) {
	p.taskQueue <- task
}

// runWorker 表示一个长期运行的 Worker
func (p *Pool) runWorker(workerID int) {
	//Done() 会将 WaitGroup 的计数器减 1，表示“这个 worker 已经完成，不再需要等待”
	defer p.wg.Done()

	fmt.Printf("[worker %d] started\n", workerID)
	//range:自动接收channel的数据并处理关闭
	//退出条件：当 channel 被关闭并且 channel 中所有已缓冲的任务都被取完后，range 循环会自动结束。
	//有task，执行；没有task，阻塞等待，不占用CPU空转
	for task := range p.taskQueue {
		p.processTask(workerID, task)
	}

	fmt.Printf("[worker %d] stopped\n", workerID)

}

// 真正处理task的业务核心：
// 接收一个任务（包含文档 ID），通过 DocumentService 完成文档的查询、处理（提取文本、分块、存储），并根据结果将文档状态更新为 SUCCESS 或 FAILED
func (p *Pool) processTask(workerID int, task DocumentTask) {
	fmt.Printf("[worker %d] processing document %d\n", workerID, task.DocumentID)

	//1. 根据 DocumentID 查询完整 Document
	document, err := p.documentService.GetByID(task.DocumentID)
	if err != nil {
		//数据库查询错误把文件标记为failed
		_ = p.documentService.UpdateStatus(task.DocumentID, "FAILED")
		fmt.Printf("[worker %d] get document %d failed: %v\n", workerID, task.DocumentID, err)
		return
	}

	//文件被删除了
	if document == nil {
		fmt.Printf("[worker %d] document %d not found\n", workerID, task.DocumentID)

		return
	}

	/*
		//2.直接调用service的业务：
		 PDF
		  ↓
		 ExtractText
		  ↓
		 SplitText
		  ↓
		 CreateBatch
	*/
	chunks, err := p.documentService.ProcessDocument(document)

	if err != nil {
		// PDF 解析、切分或者写 Chunk 失败：状态设置为failed
		_ = p.documentService.UpdateStatus(task.DocumentID, "FAILED")

		fmt.Printf("[worker %d] document %d failed: %v\n", workerID, task.DocumentID, err)

		return
	}
	//3.文档处理成功:
	//状态更新
	if err := p.documentService.UpdateStatus(task.DocumentID, "SUCCESS"); err != nil {
		fmt.Printf("[worker %d] update document %d status failed: %v\n", workerID, task.DocumentID, err)

		return
	}

	fmt.Printf("[worker %d] document %d success, chunks=%d\n", workerID, task.DocumentID, len(chunks))
}

/*
Stop 停止 Worker Pool。

 close(taskQueue)
      ↓
 不再发送新任务
      ↓
 Worker 把队列剩余任务消费完
      ↓
 for range 结束
      ↓
 Worker 退出
      ↓
 wg.Wait() 返回
*/

func (p *Pool) Stop() {
	close(p.taskQueue)
	//Wait() 方法会阻塞当前 goroutine，直到 WaitGroup 的计数器变为 0
	p.wg.Wait()
}
