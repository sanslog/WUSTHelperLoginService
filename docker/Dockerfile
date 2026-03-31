# 根目录运行 docker build -f docker/Dockerfile .

# ===== 构建阶段 =====
FROM golang:1.22 AS builder

# 换国内镜像源
RUN sed -i 's/deb.debian.org/mirrors.aliyun.com/g' /etc/apt/sources.list.d/debian.sources

RUN apt-get update && apt-get install -y \
    libtesseract-dev \
    libleptonica-dev \
    pkg-config \
    build-essential

WORKDIR /app

# 国内七牛云镜像
ENV GOPROXY=https://goproxy.cn,direct \
    CGO_ENABLED=1 \
    GOOS=linux \
    GOARCH=amd64

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -ldflags="-s -w" -o app ./internal

# ===== 运行阶段 =====
FROM debian:bookworm-slim

# 换国内镜像源
RUN sed -i 's/deb.debian.org/mirrors.aliyun.com/g' /etc/apt/sources.list.d/debian.sources

RUN apt-get update && apt-get install -y \
    tesseract-ocr \
    libtesseract-dev \
    libleptonica-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /app/app .

# 控制每个进程的线程数
ENV OMP_THREAD_LIMIT=1

EXPOSE 8080

ENTRYPOINT ["./app"]
CMD ["-p=8080", "-w=3"]