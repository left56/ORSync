FROM golang:1.21-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# 与运行镜像同平台构建（docker build / buildx 会选用匹配架构的 golang 基础镜像）
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/orsync ./cmd/orsync

FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S orsync \
    && adduser -S -G orsync -u 65532 orsync

COPY --from=build /out/orsync /usr/local/bin/orsync

WORKDIR /app
RUN mkdir -p /app/data && chown -R orsync:orsync /app

USER orsync

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/orsync"]
