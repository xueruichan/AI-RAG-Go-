//main：在程序启动时把数据库、Repository、Service、Handler、Router 全部组装好。

package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorag/internal/config"
	"gorag/internal/model"
	"log"

	"context"
	"gorag/internal/handler"
	"gorag/internal/middleware"
	"gorag/internal/repository"
	"gorag/internal/service"
	"gorag/internal/worker"
	"gorag/pkg/embedding"
	"gorag/pkg/llm"
	"os"
	"time"
)

type EchoRequest struct {
	//JSON的解析工具只匹配大写开头的，而JSON里面又只有小写的，所以互斥了,要使用标签匹配成功
	//标签：遇到 JSON 里的键名 message，就把它的值装进这个 Message 变量里
	Message string `json:"message"`
}

func main() {
	//初始化数据库，尝试连接mysql,返回*gorm.DB
	db, err := config.InitDB()
	if err != nil {
		//log 是 Go 标准库提供的“日志包”,直接打印在终端不保存
		log.Fatal("failed to connect database:", err)
	}

	//AutoMigrate：检查document表，并在mysql里创documentService建或补齐数据表
	//读取Document结构体并转换成MySQL表名document,以及创建结构体内部的字段
	err = db.AutoMigrate(&model.Document{}, &model.DocumentChunk{})
	if err != nil {
		log.Fatal("failed to migrate database:", err)
	}

	//初始化Redis
	redisClient, err := config.InitRedis()
	if err != nil {
		log.Fatal("failed to connect redis:", err)
	}
	defer redisClient.Close()

	//Go 标准库提供的创建目录函数,如果 uploads 已经存在，MkdirAll 不会重复创建，也不会报错
	if err := os.MkdirAll("uploads", 0755); err != nil {
		log.Fatal("failed to create uploads directory:", err)
	}

	// Module 8：从环境变量读取 DashScope API Key。
	// API Key 绝对不要写死进源码
	apiKey := os.Getenv("DASHSCOPE_API_KEY")
	if apiKey == "" {
		log.Fatal("DASHSCOPE_API_KEY is not set")
	}

	//创建 Embedding Client用于文本转化为向量
	embeddingClient := embedding.NewDashScopeClient(apiKey)

	//module 9:初始化DeepSeek
	// DASHSCOPE_API_KEY
	// 用于Embedding
	// DEEPSEEK_API_KEY
	// 用于LLM答案生成
	deepSeekAPIKey := os.Getenv("DEEPSEEK_API_KEY")

	if deepSeekAPIKey == "" {
		log.Fatal("DEEPSEEK_API_KEY is not set")
	}

	llmClient := llm.NewDeepSeekClient(deepSeekAPIKey)

	//Module 8：连接 Milvus,初始化设置最大 15 秒时间
	milvusCtx, milvusCancel := context.WithTimeout(
		context.Background(),
		15*time.Second,
	)

	milvusClient, err := config.InitMilvus(milvusCtx)

	milvusCancel()

	if err != nil {
		log.Fatal("failed to connect milvus:", err)
	}

	// 创建 Repository：负责数据库操作或者底层的API调用
	/*
			MySQL/GORM 的 db
		       ↓
			交给 Repository
	*/
	documentRepository := repository.NewDocumentRepository(db)
	documentChunkRepository := repository.NewDocumentChunkRepository(db)
	documentCacheRepository := repository.NewDocumentCacheRepository(redisClient)
	vectorRepository := repository.NewVectorRepository(milvusClient, embedding.DefaultDimension)

	//初始化 Milvus 集合（Collection）
	//确保应用启动后，Milvus 中用于存储向量数据的集合已经存在，并且已经加载到内存，可以正常执行搜索
	collectionCtx, collectionCancel := context.WithTimeout(
		context.Background(),
		60*time.Second,
	)
	err = vectorRepository.EnsureCollection(collectionCtx)
	collectionCancel()

	if err != nil {
		log.Fatal("failed to prepare milvus collection:", err)
	}

	log.Printf("milvus collection %s is ready", repository.MilvusCollectionName)

	// 创建 Service：负责业务逻辑
	/*
			Repository
		    ↓
			交给Service
	*/
	documentService := service.NewDocumentService(documentRepository, documentChunkRepository, documentCacheRepository, embeddingClient, vectorRepository)

	//module8新增RAG查询service层
	searchService := service.NewSearchService(embeddingClient, vectorRepository, documentChunkRepository, documentRepository)

	//module 9:：创建 RAGService， SearchService = Retrieval， llmClient = Generation。两者组合才是真正的RAG
	ragService := service.NewRAGService(
		searchService,
		llmClient,
	)

	//创建 Document Worker Pool
	//4：同时最多由 4 个 Worker 消费任务
	//100:内存 Task Queue 最多缓存 100 个任务,即任务队列大小
	documentWorkerPool := worker.NewPool(4, 100, documentService)

	//启动 4 个长期运行的 Worker goroutine
	documentWorkerPool.Start()
	defer documentWorkerPool.Stop()

	// 创建 Handler：负责 HTTP 请求和响应
	/*
			Service
		  	↓
			交给 Handler
	*/
	documentHandler := handler.NewDocumentHandler(documentService, documentWorkerPool)

	//module8新增：搜索handler层
	searchHandler := handler.NewSearchHandler(searchService)

	//module9新增：创建 RAG Handler
	ragHandler := handler.NewRAGHandler(ragService)

	//返回了一个结构体指针*gin.Engine，指向路由表
	r := gin.Default()

	//注册 Rate Limit Middleware
	r.Use(
		middleware.RateLimit(
			redisClient,
			60,
			time.Minute,
		),
	)

	//往路由表router里面注册一条GET记录：有 GET 请求访问 /health，就执行后面这个函数：handler：func(c *gin.Context)
	//handler：专门负责处理一次 HTTP 请求的函数
	//c(gin.Context)：这一次 HTTP 请求的上下文；与请求和响应有关的很多东西都通过c来获取或设置
	//gin.H:写json的
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "ok",
		})
	})

	//路由树分为静态节点和通配节点，静态节点要完全对应，Gin按/切割
	//通配节点 :name，不会去比较是否相等，而是将这段路径(name)的实际值保存下来，值被保存在上下文对象的键值对切片里面(Param)
	//curl http://localhost:8080/hello/chen -----{"message":"hellochen"}
	r.GET("/hello/:name", func(c *gin.Context) {
		//Param：上下文提供的成员方法：从这个请求的 URL 路径里，提取出名叫 name 的动态片段的值
		name := c.Param("name")

		c.JSON(http.StatusOK, gin.H{
			"message": "hello" + name,
		})
	})

	r.GET("/user/:id", func(c *gin.Context) {
		id := c.Param("id")

		c.JSON(http.StatusOK, gin.H{
			"id": id,
		})
	})

	r.POST("/echo", func(c *gin.Context) {
		var req EchoRequest

		//读取 HTTP 请求 Body 里的 JSON 数据，并把它自动填进 Go 结构体里
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid json",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message": req.Message,
		})
	})

	//注册路由：
	//r.GET() 和 r.POST() 就是两个注册函数,唯一的区别是它们把处理函数挂在了路由表的不同“方法标签”下面。
	//这里的documentHandler.Create并没有调用，调用要加括号，只是把这个函数传递给了某个变量来接收，Gin内部会自己调用函数
	r.POST("/documents", documentHandler.Create)
	r.GET("/documents", documentHandler.List)
	r.GET("/documents/:id", documentHandler.GetByID)
	r.DELETE("/documents/:id", documentHandler.Delete)
	//module8:向量语义检索
	r.POST("/search", searchHandler.Search)
	//module9:Retrieval + Generation,最终返回answer,sources
	r.POST("/ask", ragHandler.Ask)
	// Module 10：SSE 流式 RAG 问答
	// 普通 /ask 保持不变；流式问答使用独立路由。
	r.POST("/ask/stream", ragHandler.AskStream)

	// 提供前端静态文件。
	// 浏览器访问 /web/ 时，Gin 会读取 web/index.html。
	// CSS 和 JavaScript 也通过同一个 Gin 服务提供。
	r.Static("/web", "./web")

	//监听端口
	if err := r.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
