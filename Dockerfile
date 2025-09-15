# マルチステージビルド用のDockerfile
FROM golang:1.24-alpine AS builder

WORKDIR /app

# 依存関係をコピーしてダウンロード
COPY go.mod go.sum ./
RUN go mod download

# ソースコードをコピーしてビルド
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o niwaka .

# 実行用の軽量イメージ
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# バイナリと設定ファイルをコピー
COPY --from=builder /app/niwaka .
COPY --from=builder /app/config.yaml .

# キャッシュディレクトリを作成
RUN mkdir -p cache

# ポートを公開
EXPOSE 8080

# 実行
CMD ["./niwaka"]
