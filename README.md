# Niwaka - GCS Image Resize Server

GoでGCS (Google Cloud Storage) の画像を取得してリサイズして表示するウェブサーバーです。

## 特徴

- **Pure Go実装**: cgoを使用しない純Goライブラリのみ使用
- **設定ベース**: YAMLファイルでサイズセット（assortments）を管理
- **ファイルキャッシュ**: LRU方式でキャッシュを管理、設定可能な最大ファイル数
- **標準ライブラリ**: Goの標準ライブラリ（image/jpeg, image/png）を使用した画像処理

## API

```
GET /${bucket_name}/${assortment_name}/${full_object_key_including_ext}/${size}.${ext}
```

### パラメータ

- `bucket_name`: GCSバケット名
- `assortment_name`: 設定ファイルで定義されたサイズセット名
- `full_object_key_including_ext`: GCSオブジェクトキー（拡張子を含む）
- `size`: assortmentで定義されたサイズ名
- `ext`: 出力画像の拡張子（jpg, jpeg, png）

### 例

```bash
# avatar assortmentのlarge サイズで画像を取得
GET /my-bucket/avatar/images/user/profile.jpg/large.jpg

# emoji assortmentのmedium サイズで画像を取得  
GET /my-bucket/emoji/icons/smile.png/medium.png
```

## 設定

`config.yaml`ファイルで設定を行います：

```yaml
assortments:
  avatar:
    large: 1000x1000
    medium: 300x300
    small: 100x100
  emoji:
    medium: 30x30
    mini: 15x15

server:
  port: 8080
  cache_dir: "./cache"
  max_cache_files: 1000

gcs:
  project_id: ""  # 環境変数から取得
  credentials_file: ""  # 環境変数から取得
```

## セットアップ

### 1. GCP認証設定

```bash
# サービスアカウントキーを使用する場合
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account-key.json

# gcloud CLIでの認証を使用する場合
gcloud auth application-default login
```

### 2. 依存関係のインストール

```bash
go mod tidy
```

### 3. ビルド

```bash
go build -o niwaka
```

### 4. 実行

```bash
# デフォルト設定ファイル (config.yaml) を使用
./niwaka

# カスタム設定ファイルを指定
./niwaka /path/to/custom-config.yaml
```

## キャッシュ管理

- キャッシュファイルは `cache_dir` で指定されたディレクトリに保存されます
- `max_cache_files` で指定された数を超えると、LRU（Least Recently Used）方式で古いファイルが削除されます
- キャッシュキーはMD5ハッシュを使用して生成されます

## 対応画像フォーマット

- **入力**: JPEG, PNG（Goの標準ライブラリがサポートするフォーマット）
- **出力**: JPEG, PNG

## ディレクトリ構造

```
.
├── config.yaml          # 設定ファイル
├── main.go              # メインアプリケーション
├── go.mod               # Go modules設定
├── niwaka               # ビルド済みバイナリ
├── cache/               # キャッシュディレクトリ（自動作成）
└── README.md            # このファイル
```

## 開発

### テスト実行

```bash
go test ./...
```

### フォーマット

```bash
go fmt ./...
```

### ベンチマーク

キャッシュの効果を測定するには：

```bash
# 同じ画像を複数回リクエスト
time curl http://localhost:8080/my-bucket/avatar/test.jpg/large.jpg
time curl http://localhost:8080/my-bucket/avatar/test.jpg/large.jpg  # キャッシュから取得
```

## ライセンス

MIT License
