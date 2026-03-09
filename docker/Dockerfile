# 根目录运行 docker build -f docker/Dockerfile .

# 编译阶段
FROM  golang:1.25.5-alpine3.22 AS builder

WORKDIR /app

RUN apk add --no-cache git

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN  go build -ldflags="-s -w" -o myapp ./internal

#构建阶段
FROM alpine:3.22

RUN apk add --no-cache \
    tesseract-ocr \
    tesseract-ocr-data-eng

WORKDIR /root/

COPY --from=builder /app/myapp .

EXPOSE 8080

ENTRYPOINT ["/root/myapp"]
CMD ["-p=8080", "-T=/usr/bin/tesseract","-w=3"]