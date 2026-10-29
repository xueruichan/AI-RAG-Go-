
# =====================================================
# 第一阶段：编译 Go 程序
# =====================================================

FROM golang:1.26.5 AS builder

WORKDIR /src

# 复制 Go 模块配置和完整项目源码，包括 vendor 目录。
COPY go.mod go.sum ./
COPY . .

# 使用 vendor 中的本地依赖编译。
# GOPROXY=off 确保这一阶段不会尝试通过网络下载 Go 模块。
RUN mkdir -p /out && \
    GOPROXY=off CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -mod=vendor -trimpath -ldflags="-s -w" \
    -o /out/gorag ./cmd/server


# =====================================================
# 第二阶段：运行 Go 服务
# =====================================================

FROM debian:bookworm-slim

# 切换到经过连通性验证的清华 Debian 镜像。
# 使用 HTTP 软件源，避免在安装 ca-certificates 之前遇到 HTTPS
# 证书验证问题。
RUN set -eux; \
    rm -f /etc/apt/sources.list; \
    printf '%s\n' \
      'deb http://mirrors.tuna.tsinghua.edu.cn/debian/ bookworm main' \
      'deb http://mirrors.tuna.tsinghua.edu.cn/debian/ bookworm-updates main' \
      'deb http://mirrors.tuna.tsinghua.edu.cn/debian-security/ bookworm-security main' \
      > /etc/apt/sources.list; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
      ca-certificates \
      tzdata \
      poppler-utils; \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

# 复制 Go 可执行文件和前端静态资源。
COPY --from=builder /out/gorag /app/gorag
COPY web/ /app/web/

# 创建上传目录，并使用非 root 用户运行。
RUN groupadd --system app \
    && useradd --system --gid app --no-create-home app \
    && mkdir -p /app/uploads \
    && chown -R app:app /app

USER app

EXPOSE 8080

CMD ["./gorag"]
