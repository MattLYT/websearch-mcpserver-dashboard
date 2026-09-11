# Go 交叉编译在宿主机架构上完成（--platform=$BUILDPLATFORM），
# 不要在 QEMU 里跑编译器。最终阶段无 RUN，buildx 无需 binfmt。
# TARGETOS/TARGETARCH 由 buildx 注入；本地 docker build 则跟随当前机器。
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o websearch ./cmd/

# ca-certificates 同样在宿主架构安装，再拷进目标镜像（避免 apk 走 QEMU）
FROM --platform=$BUILDPLATFORM alpine:latest AS certs
RUN apk --no-cache add ca-certificates

FROM alpine:latest

WORKDIR /app/

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/cert.pem
COPY --from=builder /app/websearch .
COPY --from=builder /app/config.example.yaml ./config.yaml

EXPOSE 8338

CMD ["./websearch", "start"]
