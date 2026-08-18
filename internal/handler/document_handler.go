/*
handler:HTTP
读取请求
解析参数
返回 JSON
*/

/*
HTTP接口层
接收 Gin 框架传过来的 HTTP 请求（比如 POST /api/documents）。
从请求中提取参数（比如 JSON 里的文件名、URL 里的 ID）。
调用 service 层的业务方法。
把 service 层返回的结果转换成 HTTP 响应（JSON 或错误码）返回给客户端
Handler
   ↓
Service
*/

package handler

import (
	"fmt"
	"net/http"
	"os"
	//Go 官方的 path/filepath 就是专门进行当前操作系统文件路径处理的包
	"gorag/internal/service"
	"gorag/internal/worker"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Module 3 先把单文件最大大小限制为 10 MB
const maxUploadSize int64 = 10 * 1024 * 1024

type DocumentHandler struct {
	//文档业务层
	service *service.DocumentService
	//后台文档处理
	workerPool *worker.Pool
}

func NewDocumentHandler(documentService *service.DocumentService, workerPool *worker.Pool) *DocumentHandler {
	return &DocumentHandler{
		service:    documentService,
		workerPool: workerPool,
	}
}

// 上传pdf并进行切片
func (h *DocumentHandler) Create(c *gin.Context) {
	//1.获取文件
	//c.FormFile：这是 Gin 提供的一个方法，专门用来解析 HTTP 请求体中的 multipart/form-data 格式（也就是文件上传专用的 MIME 类型）
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "file is required",
		})
		return
	}
	//2.拒绝空文件
	if file.Size == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "empty file is not allowed",
		})
		return
	}
	//3.限制文件大小
	//file.Size单位是byte,如果文件超过 10 MB,不保存也不写入数据库
	if file.Size > maxUploadSize {
		//413:由于请求的实体过大，服务器无法处理，因此拒绝请求
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"error": "file is too large,max size is 10MB",
		})
		return
	}
	//4.处理客户端文件名
	//不能直接信任，要进行处理：../../abc/test.pdf处理为test.pdf
	//filepath.Base:接受一个字符串，把它当成文件路径处理，然后只返回路径中的最后一个元素（文件名），剥离掉所有父目录部分
	safeName := filepath.Base(file.Filename)

	//5.只允许PDF
	//filepath.Ext(safeName) —— 提取扩展名  从最后一个点号 . 截取到末尾
	//strings.ToLower(...) —— 强制转小写
	ext := strings.ToLower(filepath.Ext(safeName))

	if ext != ".pdf" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "only PDF files are allowed",
		})
		return
	}

	//6.生成服务器内部文件名
	//不能直接生成 uploads/test.pdf，因为第二个人上传test.pdf就会被覆盖

	//生成存储文件名:“当前时间的纳秒数” 和 “处理后的原始文件名”，用下划线 _ 拼成
	//sprintf:根据指定的格式，把变量拼装成一个完整的字符串，然后把这个字符串返回（不会打印到终端）
	storedName := fmt.Sprintf(
		"%d_%s",
		time.Now().UnixNano(),
		safeName,
	)
	//生成完整的物理路径
	//调用 filepath.Join 函数，把文件夹名称 "uploads" 和刚刚生成的 storedName，用操作系统正确的路径分隔符拼接到一起。
	//filepath.Join：它是操作系统感知的,Linux/macOS :"/";Windows:"\"，不同操作系统分隔符不同
	storedPath := filepath.Join(
		"uploads",
		storedName,
	)

	//7.把文件写进linux文件系统
	//到这里才是真正的保存文件
	//c.SaveUploadedFile 是 Gin 提供保存 FormFile 拿到的文件对象
	//相当于封装了c++中的fopen、fread、fwrite、fclose这一系列系统调用
	if err := c.SaveUploadedFile(file, storedPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to save uploaded file",
		})
		return
	}

	//8.创建数据库记录,交给service业务层
	document, err := h.service.Create(
		safeName,
		storedPath,
	)
	//此时文件已经在系统中保存成功但是mysql创建失败
	//必须删除系统中的文件进行回滚，否则就会导致数据不一致的后果：
	//uploads/xxx.pdf 存在但是数据库中没有记录
	if err != nil {
		//os.Remove 是 Go 标准库提供的系统调用封装,删除指定路径的文件并返回一个error
		_ = os.Remove(storedPath)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to create document record",
		})
		return
	}

	// Document不再在 HTTP Handler中同步处理，由后台worker执行:
	//(1)更新文件状态
	if err := h.service.UpdateStatus(
		document.ID,
		"PROCESSING",
	); err != nil {
		//如果PROCESSING更新失败,回滚刚才创建的 Document 和 PDF
		_, _ = h.service.Delete(document.ID)
		_ = os.Remove(storedPath)

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to update document status",
		})
		return
	}

	//(2)创建后台任务
	//Task 中只记录 DocumentID:
	//
	task := worker.DocumentTask{
		DocumentID: document.ID,
	}

	//(3)把Task放入Worker Pool，这一行并不是执行PDF解析，而是将任务放入channel
	h.workerPool.Submit(task)

	//(4)HTTP返回
	c.JSON(http.StatusAccepted, gin.H{
		"document_id": document.ID,
		"status":      "PROCESSING",
	})
}

func (h *DocumentHandler) List(c *gin.Context) {
	documents, err := h.service.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to list documents",
		})
		return
	}
	c.JSON(http.StatusOK, documents)
}

func (h *DocumentHandler) GetByID(c *gin.Context) {
	//strconv.ParseUint将字符串转换成uint,因为数据库里面是uint
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid document id",
		})
		return
	}
	//注：关于err可以一直重复是因为短声明只用确保左边有一个新变量即可，存在的变量可以复用
	document, err := h.service.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get document",
		})
		return
	}
	if document == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "document not found",
		})
		return
	}

	c.JSON(http.StatusOK, document)
}

func (h *DocumentHandler) Delete(c *gin.Context) {
	//1.URL参数解析与格式校验，判断id的格式是否正确。例：不能属于字符串等等来作为id
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid document id",
		})
		return
	}
	//2.先查后删：在数据库中查询到这条id的物理文件路径并保存
	//删除之前要查document数据库，得到文件目录，否则如果直接删除数据库会导致找不到对应的系统中的PDF也就进行不了删除了
	document, err := h.service.GetByID(uint(id))
	//系统故障，返回（只要是err都是系统级错误，另一个返回参数才是具体的错误）
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get document",
		})
		return
	}

	//获取时：数据库中没有查询到这条id，返回
	if document == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "document not found",
		})
		return
	}

	//3.数据库删除
	deleted, err := h.service.Delete(uint(id))
	//系统故障，返回
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to delete document",
		})
		return
	}

	//删除时：数据库中没有这条id（单线程下与没有查询到id的错误是冗余的，但是多线程情况下可能会被抢删，所以删除前还要判断）
	if deleted == false {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "document not found",
		})
		return
	}

	//4.物理删除系统磁盘上的pdf文件
	if document.FilePath != "" {
		err := os.Remove(document.FilePath)

		//如果文件不存在不需要认为delete失败
		//IsNotExist：错误类型断言，判断这个err是否属于"文件不存在"这类错误
		if err != nil && !os.IsNotExist(err) {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "document deleted,but failed to remove file",
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "document deleted",
	})
}
