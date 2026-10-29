# GoRAG：基于 Go 的 PDF 知识库与 RAG 问答系统

GoRAG 是一个基于 Go 的文档知识库项目。用户上传 PDF
后，服务在后台提取文本、切分 chunk、调用 Embedding API 生成向量并写入
Milvus；用户提问时，系统进行语义检索，再调用 DeepSeek API
生成回答。项目提供 Gin 后端、Web 前端和 SSE 流式问答。

-   GitHub：<https://github.com/xueruichan/AI-RAG-Go->
-   后端：Go、Gin
-   数据存储：MySQL、Redis、Milvus
-   模型 API：DashScope Embedding、DeepSeek
-   PDF 文本提取：Poppler `pdftotext`
-   部署：Docker、Docker Compose
-   前端：HTML、CSS、JavaScript

> **项目定位：**
> 个人学习与面试演示项目。目前以共享知识库为主，尚未实现完整的注册登录、用户租户隔离和生产级权限体系。正式开放公网访问前，应保护上传、删除等接口，并限制资源使用。

## 功能概览

-   PDF 上传与文档管理：上传、列表、详情、删除。
-   后台异步处理：通过 Worker Pool 处理上传后的文档。
-   PDF 文本提取与切分：使用 `pdftotext` 提取文本，再切分成 chunk。
-   中英文文本检索：支持可提取文本的中文和英文 PDF；纯扫描图片 PDF
    需要额外 OCR，当前流程不等同于 OCR。
-   向量化与语义检索：调用 DashScope Embedding API，使用 Milvus
    检索相关文本片段。
-   RAG 问答：将检索资料作为上下文，调用 DeepSeek API 生成回答。
-   SSE 流式输出：服务器分段推送回答内容，前端可以逐步显示。
-   Web 前端：提供上传、文档列表、语义检索和智能问答界面。
-   Docker 化运行：使用 Compose 管理 GoRAG、MySQL、Redis 等服务。

## 架构与数据流程

### 文档入库

``` text
浏览器上传 PDF
      |
      v
Gin POST /documents
      |
      v
保存文件并创建文档记录
      |
      v
Worker Pool 异步处理
      |
      v
pdftotext 提取文本 -> 文本切分
      |
      v
DashScope Embedding API
      |
      v
向量写入 Milvus，文档状态和元数据存入数据库
```

上传接口返回 `202 Accepted`
表示任务已被接受，不代表索引已经完成。应继续检查文档状态和服务日志。

### 检索与问答

``` text
用户问题 -> Query Embedding -> Milvus Top-K 检索
          -> 组装参考资料 -> DeepSeek API
                                  |
                                  +--> POST /ask：普通问答
                                  +--> POST /ask/stream：SSE 流式问答
```

## 项目结构

以下为核心目录示意，具体文件以仓库实际内容为准：

``` text
AI-RAG-Go-/
├── cmd/
│   ├── server/          # GoRAG 服务入口
│   └── test_embedding/  # Embedding API 测试程序
├── internal/
│   ├── config/          # 数据库、Redis、Milvus 等配置
│   ├── handler/         # Gin HTTP handlers
│   ├── service/         # 文档、检索、问答业务逻辑
│   └── ...              # repository、worker 等内部模块
├── pkg/                 # 可复用组件
├── web/
│   ├── index.html       # 前端页面
│   ├── app.js           # 前端交互
│   └── style.css        # 样式
├── vendor/              # Go 依赖；Docker 使用 -mod=vendor 时需要
├── Dockerfile
├── docker-compose.yml
├── .dockerignore
├── .gitignore
├── .env.example         # 环境变量模板，不包含真实密钥
├── go.mod
└── go.sum
```

## 运行前准备

-   Git
-   Docker Engine
-   Docker Compose v2（`docker compose`）
-   DashScope API Key
-   DeepSeek API Key
-   一个 GoRAG 容器可以访问的 Milvus 服务
-   可提取文本的 PDF 测试文件

当前 Dockerfile 使用 `golang:1.26.5` 作为构建阶段基础镜像，并通过
`go build -mod=vendor` 使用仓库中的 `vendor/` 目录。修改 Go
依赖后，应同步检查 `go.mod`、`go.sum` 和 `vendor/`。

### Milvus 部署说明

当前开发环境中的 Milvus Compose 项目与 GoRAG 仓库分开维护。GoRAG 的
Compose 配置主要管理 GoRAG、MySQL 和
Redis；在新机器上运行前，必须单独启动或提供兼容的 Milvus 服务，并确保
GoRAG 容器能访问 `MILVUS_ADDRESS`。因此，仅克隆本仓库并启动
Compose，不一定会自动启动 Milvus。云服务器部署时需要一并规划 Milvus
服务、网络与持久化卷。

## 环境变量

复制环境变量模板：

``` bash
cp .env.example .env
```

编辑 `.env`，填入自己的 API Key 和密码。**`.env`
含有敏感信息，禁止提交到公开 GitHub 仓库。**

  ----------------------------------------------------------------------------------
  变量                    用途                    配置提示
  ----------------------- ----------------------- ----------------------------------
  `DASHSCOPE_API_KEY`     Embedding API 密钥      填入真实 DashScope Key

  `DEEPSEEK_API_KEY`      问答模型 API 密钥       填入真实 DeepSeek Key

  `DB_HOST`               MySQL 主机              Docker 网络内通常使用 Compose
                                                  服务名，例如
                                                  `mysql`，以实际配置为准

  `DB_PORT`               MySQL 端口              常见为 `3306`

  `DB_USER`               应用数据库用户          与 MySQL 初始化配置一致

  `DB_PASSWORD`           应用数据库密码          使用强密码

  `DB_NAME`               数据库名                例如 `gorag`

  `MYSQL_ROOT_PASSWORD`   MySQL root 密码         与应用用户密码分开设置

  `REDIS_ADDR`            Redis 地址              Docker
                                                  网络内通常使用服务名和端口，例如
                                                  `redis:6379`

  `REDIS_PASSWORD`        Redis 密码              与 Redis 服务配置一致

  `MILVUS_ADDRESS`        Milvus 地址             必须能从 GoRAG 容器访问
  ----------------------------------------------------------------------------------

**容器中的 `127.0.0.1` 指向 GoRAG 容器自身，而不是其他服务。** Docker
部署时，数据库和 Milvus 地址应按实际 Docker
网络填写，不要直接照抄本地运行时的地址。

## 本地 Docker 启动

以下步骤用于启动仓库 Compose 中配置的 GoRAG、MySQL、Redis 服务；Milvus
需要先单独启动并确保可达。

### 1. 克隆项目

``` bash
git clone https://github.com/xueruichan/AI-RAG-Go-.git
cd AI-RAG-Go-
```

### 2. 配置环境变量

``` bash
cp .env.example .env
```

编辑 `.env`，填写 API Key、数据库密码、Redis
密码，以及当前容器网络中正确的数据库和 Milvus 地址。

### 3. 启动 Milvus

先启动你部署的 Milvus Compose 项目，并确认 GoRAG 容器可以访问
`MILVUS_ADDRESS`。当前仓库没有包含独立 Milvus Compose
项目的完整文件，因此新环境需单独准备 Milvus。

### 4. 构建并启动 GoRAG 相关服务

``` bash
sudo docker compose up -d --build
```

查看状态和日志：

``` bash
sudo docker compose ps
sudo docker compose logs --tail=100 gorag
```

持续查看日志：

``` bash
sudo docker compose logs -f gorag
```

### 5. 健康检查与前端

``` bash
curl -i http://127.0.0.1:8080/health
```

正常情况下返回 HTTP `200 OK`。

在服务所在机器打开：

<http://127.0.0.1:8080/web/>

从其他电脑访问时，应使用服务器可达的 IP
或域名，并正确配置防火墙、安全组和反向代理。

## API 接口

以下路由来自当前服务注册信息。具体请求字段和返回结构以代码实现为准。

  方法       路径               用途
  ---------- ------------------ ------------------
  `GET`      `/health`          健康检查
  `GET`      `/hello/:name`     基础路由测试
  `GET`      `/user/:id`        基础参数路由测试
  `POST`     `/echo`            JSON 回显测试
  `POST`     `/documents`       上传 PDF
  `GET`      `/documents`       获取文档列表
  `GET`      `/documents/:id`   获取指定文档
  `DELETE`   `/documents/:id`   删除指定文档
  `POST`     `/search`          语义检索
  `POST`     `/ask`             普通 RAG 问答
  `POST`     `/ask/stream`      SSE 流式问答

### 上传 PDF

``` bash
curl -i -F "file=@./chinese_test.pdf" \
  http://127.0.0.1:8080/documents
```

返回 `202 Accepted` 代表请求已接收，后台仍需完成 PDF 提取、Embedding
和向量入库。

### 文档列表

``` bash
curl -i http://127.0.0.1:8080/documents
```

### 语义检索

``` bash
curl -sS -X POST \
  http://127.0.0.1:8080/search \
  -H 'Content-Type: application/json' \
  -d '{
    "query": "文档中的紧急验证码是什么？",
    "top_k": 5
  }' | python3 -m json.tool
```

### 普通 RAG 问答

``` bash
curl -i -X POST \
  http://127.0.0.1:8080/ask \
  -H 'Content-Type: application/json' \
  -d '{
    "query": "请总结文档的主要内容",
    "top_k": 5
  }'
```

## SSE 流式问答

SSE（Server-Sent Events）允许服务器在一个 HTTP
请求中持续推送事件，前端可以逐步显示生成的回答。

用 `curl -N` 测试，避免 curl 缓冲输出：

``` bash
curl -N -X POST \
  http://127.0.0.1:8080/ask/stream \
  -H 'Content-Type: application/json' \
  -H 'Accept: text/event-stream' \
  -d '{
    "query": "请总结文档的主要内容",
    "top_k": 5
  }'
```

当前实现会推送状态、参考资料和回答片段。输出格式可能类似：

``` text
event:status
data:{"status":"retrieving"}

event:sources
data:{"sources":[...]}

event:delta
data:{"content":"根据"}

event:delta
data:{"content":"资料"}
```

以上仅为格式示意，实际字段和事件顺序以当前服务输出为准。多个 `delta`
事件拼接后构成完整回答。

## 功能测试清单

1.  `/health` 返回 HTTP 200。
2.  上传中文和英文 PDF，获取文档 ID。
3.  确认文档异步处理最终成功，并检查日志无文本提取、Embedding 或 Milvus
    写入错误。
4.  中文、英文语义检索均能返回相关 chunk 和来源。
5.  RAG 回答与检索资料相关，参考资料可追溯。
6.  SSE 测试能持续收到事件，前端逐步显示回答。
7.  删除测试文档后检查列表与检索结果，并确认当前删除实现对关联向量的处理符合预期。
8.  重启容器后检查数据库、向量服务和上传文件是否按数据卷配置保留。

## 常用运维命令

在仓库目录执行：

``` bash
# 查看状态
sudo docker compose ps

# 查看最近日志
sudo docker compose logs --tail=100 gorag

# 持续查看 GoRAG 日志
sudo docker compose logs -f gorag

# 启动服务
sudo docker compose up -d

# 修改代码或 Dockerfile 后重新构建 GoRAG
sudo docker compose up -d --build gorag

# 停止服务，但保留容器和数据卷
sudo docker compose stop

# 检查磁盘容量
df -h /
```

普通停止服务时不要使用 `docker compose down -v`，该命令可能删除 Compose
管理的数据卷。清理镜像、数据卷或数据库数据前，先确认用途和备份情况。

## 常见问题

### Docker 构建时无法下载 Go 依赖

当前 Dockerfile 使用 `vendor/` 和
`go build -mod=vendor`，避免构建阶段依赖 Go
模块代理。修改依赖后同步更新：

``` bash
go mod tidy
go mod vendor
GOPROXY=off go build -mod=vendor -o /tmp/gorag-check ./cmd/server
```

### PDF 上传返回 500

先看日志：

``` bash
sudo docker compose logs --since=5m --tail=100 gorag
```

再检查磁盘和上传目录：

``` bash
df -h /
sudo docker compose exec gorag sh -lc \
  'df -h /app/uploads; ls -ld /app/uploads; command -v pdftotext'
```

可能原因包括磁盘空间不足、上传目录权限、PDF 文件异常或容器缺少
`pdftotext`。不要只根据 HTTP 500 就直接删除数据或重建数据库。

### 上传返回 202，但文档一直没有完成

`202 Accepted` 只代表任务已接收。继续查看 Worker 日志，排查 PDF
文本提取、Embedding API 限流或认证、Milvus 连接和向量写入问题。

### 容器无法连接 MySQL、Redis 或 Milvus

检查环境变量、Compose 服务名、Docker 网络和服务状态。容器中的
`127.0.0.1` 不指向其他容器。

### SSE 没有逐步输出

先用 `curl -N` 测试，并检查响应头是否为
`text/event-stream`。如果放在反向代理后，还需要检查代理缓冲和超时配置。

### 中文 PDF 提取不到文字

在宿主机检查 PDF 文本层：

``` bash
pdftotext -enc UTF-8 ./chinese_test.pdf -
```

如果 PDF 是纯扫描图片，当前 `pdftotext` 流程不能替代 OCR。

## 安全与公网部署注意事项

-   不要将 `.env`、真实 API Key、数据库密码提交到 GitHub。
-   如果密钥曾公开，应及时轮换。
-   不要把 MySQL、Redis、Milvus、etcd、MinIO 等内部端口直接暴露给公网。
-   公网演示建议使用域名、HTTPS 和反向代理。
-   对上传、删除、检索和问答接口增加访问控制、请求大小限制、速率限制和资源保护。
-   当前项目尚未实现完整的多用户文档隔离，不应直接用来存储不同用户的私密文件。
-   使用 Docker 数据卷持久化数据库数据，定期检查磁盘空间并制定备份方案。
-   日志应避免输出 API Key、密码及不必要的文档敏感内容。

## 后续计划

-   [x] Go + Gin 后端基础接口
-   [x] PDF 上传与后台异步处理
-   [x] 中文 PDF 文本提取与语义检索验证
-   [x] RAG 问答
-   [x] SSE 流式问答
-   [x] Web 前端基础交互
-   [x] GoRAG、MySQL、Redis 的 Docker 化运行
-   [ ] 完整回归测试
-   [ ] 云服务器部署、HTTPS 和公网访问
-   [ ] 上线前接口访问控制与资源限制
-   [ ] 视需要增加账号系统与用户文档隔离

## License

如果准备公开分发或接受外部贡献，建议为项目补充合适的开源许可证。仓库是否已有许可证文件，以
GitHub 实际内容为准。
