# =====================================================
# 第一阶段：编译 Go 程序
# =====================================================

# 使用与你当前 go.mod 匹配的 Go 版本。
FROM golang:1.26.5 AS builder

WORKDIR /src

# 先复制依赖清单，便于 Docker 缓存依赖下载。
COPY go.mod go.sum ./

# 多个代理依次尝试；使用 | 允许在网络错误时继续尝试下一个代理
ENV GOPROXY=https://goproxy.cn|https://proxy.golang.com.cn|direct

RUN go mod download

# 再复制项目源码。
COPY . .

# 编译 Linux 可执行文件。
# CGO_ENABLED=0 生成不依赖系统 C 库的静态 Go 程序。
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" \
    -o /out/gorag ./cmd/server


# =====================================================
# 第二阶段：运行 Go 服务
# =====================================================

FROM debian:bookworm-slim

# 安装 HTTPS 请求所需的 CA 证书和时区数据。
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# 只复制编译产物和前端静态文件，不复制 Go 编译环境。
COPY --from=builder /out/gorag /app/gorag
COPY web/ /app/web/

# 为上传文件创建目录，并使用非 root 用户运行服务。
RUN groupadd --system app \
    && useradd --system --gid app --no-create-home app \
    && mkdir -p /app/uploads \
    && chown -R app:app /app

USER app

# Go 服务监听的容器内部端口。
EXPOSE 8080

CMD ["./gorag"]