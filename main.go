package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"gopkg.in/yaml.v3"
)

// 設定構造体
type Config struct {
	Buckets     map[string]string            `yaml:"buckets"`
	Assortments map[string]map[string]string `yaml:"assortments"`
	Server      ServerConfig                 `yaml:"server"`
	GCS         GCSConfig                    `yaml:"gcs"`
}

type ServerConfig struct {
	Port          int    `yaml:"port"`
	CacheDir      string `yaml:"cache_dir"`
	MaxCacheFiles int    `yaml:"max_cache_files"`
}

type GCSConfig struct {
	ProjectID       string `yaml:"project_id"`
	CredentialsFile string `yaml:"credentials_file"`
}

// CLIオプション構造体
type CLIOptions struct {
	ConfigPath      string
	Port            int
	CacheDir        string
	MaxCacheFiles   int
	ProjectID       string
	CredentialsFile string
}

// サイズ構造体
type Size struct {
	Width  uint
	Height uint
}

// パースサイズ
func parseSize(sizeStr string) (Size, error) {
	parts := strings.Split(sizeStr, "x")
	if len(parts) != 2 {
		return Size{}, fmt.Errorf("invalid size format: %s", sizeStr)
	}

	width, err := strconv.ParseUint(parts[0], 10, 32)
	if err != nil {
		return Size{}, fmt.Errorf("invalid width: %s", parts[0])
	}

	height, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return Size{}, fmt.Errorf("invalid height: %s", parts[1])
	}

	return Size{Width: uint(width), Height: uint(height)}, nil
}

// 純Goでの画像リサイズ実装
func resizeImage(src image.Image, width, height int) image.Image {
	srcBounds := src.Bounds()
	srcW := srcBounds.Max.X
	srcH := srcBounds.Max.Y

	// アスペクト比を保持したリサイズ
	if width == 0 && height == 0 {
		return src
	}

	if width == 0 {
		width = srcW * height / srcH
	}
	if height == 0 {
		height = srcH * width / srcW
	}

	// 新しい画像を作成
	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	// シンプルな線形補間でリサイズ
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// 元画像の対応する座標を計算
			srcX := x * srcW / width
			srcY := y * srcH / height

			// 境界チェック
			if srcX >= srcW {
				srcX = srcW - 1
			}
			if srcY >= srcH {
				srcY = srcH - 1
			}

			// ピクセル値をコピー
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}

	return dst
}

// バケット名解決関数
func (s *Server) resolveBucketName(bucketAlias string) (string, error) {
	if actualBucket, exists := s.config.Buckets[bucketAlias]; exists {
		return actualBucket, nil
	}
	return "", fmt.Errorf("unknown bucket alias: %s", bucketAlias)
}

// キャッシュマネージャー
type CacheManager struct {
	cacheDir    string
	maxFiles    int
	accessTimes map[string]time.Time
}

func NewCacheManager(cacheDir string, maxFiles int) *CacheManager {
	os.MkdirAll(cacheDir, 0755)
	return &CacheManager{
		cacheDir:    cacheDir,
		maxFiles:    maxFiles,
		accessTimes: make(map[string]time.Time),
	}
}

func (cm *CacheManager) getCacheKey(bucket, objectKey, assortment, sizeName, ext string) string {
	key := fmt.Sprintf("%s/%s/%s/%s.%s", bucket, objectKey, assortment, sizeName, ext)
	hash := md5.Sum([]byte(key))
	return hex.EncodeToString(hash[:])
}

func (cm *CacheManager) getCachePath(cacheKey string) string {
	return filepath.Join(cm.cacheDir, cacheKey)
}

func (cm *CacheManager) Get(cacheKey string) ([]byte, bool) {
	cachePath := cm.getCachePath(cacheKey)
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		return nil, false
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, false
	}

	cm.accessTimes[cacheKey] = time.Now()
	return data, true
}

func (cm *CacheManager) Set(cacheKey string, data []byte) error {
	// キャッシュファイル数制限チェック
	if len(cm.accessTimes) >= cm.maxFiles {
		cm.evictLRU()
	}

	cachePath := cm.getCachePath(cacheKey)
	err := os.WriteFile(cachePath, data, 0644)
	if err != nil {
		return err
	}

	cm.accessTimes[cacheKey] = time.Now()
	return nil
}

func (cm *CacheManager) evictLRU() {
	// アクセス時間でソートして古いファイルを削除
	type keyTime struct {
		key  string
		time time.Time
	}

	var keyTimes []keyTime
	for key, time := range cm.accessTimes {
		keyTimes = append(keyTimes, keyTime{key: key, time: time})
	}

	sort.Slice(keyTimes, func(i, j int) bool {
		return keyTimes[i].time.Before(keyTimes[j].time)
	})

	// 古い半分を削除
	toDelete := len(keyTimes) / 2
	for i := 0; i < toDelete; i++ {
		key := keyTimes[i].key
		cachePath := cm.getCachePath(key)
		os.Remove(cachePath)
		delete(cm.accessTimes, key)
	}
}

// サーバー構造体
type Server struct {
	config       *Config
	gcsClient    *storage.Client
	cacheManager *CacheManager
}

func NewServer(options *CLIOptions) (*Server, error) {
	// 設定読み込み
	configData, err := os.ReadFile(options.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// CLIオプションで設定を上書き
	if options.Port != 0 {
		config.Server.Port = options.Port
	}
	if options.CacheDir != "" {
		config.Server.CacheDir = options.CacheDir
	}
	if options.MaxCacheFiles != 0 {
		config.Server.MaxCacheFiles = options.MaxCacheFiles
	}
	if options.ProjectID != "" {
		config.GCS.ProjectID = options.ProjectID
	}
	if options.CredentialsFile != "" {
		config.GCS.CredentialsFile = options.CredentialsFile
	}

	// 環境変数からGCS設定を取得（CLIオプションが優先）
	if config.GCS.ProjectID == "" {
		config.GCS.ProjectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}
	if config.GCS.CredentialsFile == "" {
		config.GCS.CredentialsFile = os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	}

	// GCSクライアント初期化
	ctx := context.Background()
	var gcsClient *storage.Client
	if config.GCS.CredentialsFile != "" {
		// サービスアカウントキーファイルを使用
		os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", config.GCS.CredentialsFile)
	}

	gcsClient, err = storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}

	// キャッシュマネージャー初期化
	cacheManager := NewCacheManager(config.Server.CacheDir, config.Server.MaxCacheFiles)

	return &Server{
		config:       &config,
		gcsClient:    gcsClient,
		cacheManager: cacheManager,
	}, nil
}

func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) {
	// URLパース: /{bucket}/{assortment}/{object_key}/{size}.{ext}
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	bucketAlias := pathParts[0]
	assortment := pathParts[1]

	// バケットエイリアスを実際のバケット名に変換
	actualBucket, err := s.resolveBucketName(bucketAlias)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid bucket alias: %s", bucketAlias), http.StatusBadRequest)
		return
	}

	// object_keyは複数のパートを持つ可能性がある
	objectKeyParts := pathParts[2 : len(pathParts)-1]
	objectKey := strings.Join(objectKeyParts, "/")

	// 最後のパートから size.ext を抽出
	lastPart := pathParts[len(pathParts)-1]
	dotIndex := strings.LastIndex(lastPart, ".")
	if dotIndex == -1 {
		http.Error(w, "Invalid file format", http.StatusBadRequest)
		return
	}

	sizeName := lastPart[:dotIndex]
	ext := lastPart[dotIndex+1:]

	// assortmentとsizeの検証
	sizes, exists := s.config.Assortments[assortment]
	if !exists {
		http.Error(w, "Unknown assortment", http.StatusBadRequest)
		return
	}

	sizeStr, exists := sizes[sizeName]
	if !exists {
		http.Error(w, "Unknown size", http.StatusBadRequest)
		return
	}

	size, err := parseSize(sizeStr)
	if err != nil {
		http.Error(w, "Invalid size format", http.StatusBadRequest)
		return
	}

	// キャッシュキー生成（エイリアス名を使用してキャッシュキーの一意性を保つ）
	cacheKey := s.cacheManager.getCacheKey(bucketAlias, objectKey, assortment, sizeName, ext)

	// キャッシュチェック
	if cachedData, found := s.cacheManager.Get(cacheKey); found {
		w.Header().Set("Content-Type", fmt.Sprintf("image/%s", ext))
		w.Write(cachedData)
		return
	}

	// GCSから画像を取得（実際のバケット名を使用）
	ctx := context.Background()
	obj := s.gcsClient.Bucket(actualBucket).Object(objectKey)
	reader, err := obj.NewReader(ctx)
	if err != nil {
		http.Error(w, "Failed to read object from GCS", http.StatusNotFound)
		return
	}
	defer reader.Close()

	// 画像データを読み込み
	imageData, err := io.ReadAll(reader)
	if err != nil {
		http.Error(w, "Failed to read image data", http.StatusInternalServerError)
		return
	}

	// 画像をデコード
	img, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		http.Error(w, "Failed to decode image", http.StatusInternalServerError)
		return
	}

	// リサイズ（純Go実装）
	resizedImg := resizeImage(img, int(size.Width), int(size.Height))

	// エンコード
	var buf bytes.Buffer
	switch ext {
	case "jpg", "jpeg":
		err = jpeg.Encode(&buf, resizedImg, &jpeg.Options{Quality: 85})
	case "png":
		err = png.Encode(&buf, resizedImg)
	default:
		http.Error(w, "Unsupported image format", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, "Failed to encode image", http.StatusInternalServerError)
		return
	}

	encodedData := buf.Bytes()

	// キャッシュに保存
	s.cacheManager.Set(cacheKey, encodedData)

	// レスポンス返却
	w.Header().Set("Content-Type", fmt.Sprintf("image/%s", ext))
	w.Write(encodedData)
}

func (s *Server) Start() error {
	http.HandleFunc("/", s.handleImage)

	port := s.config.Server.Port
	log.Printf("Starting server on port %d", port)
	log.Printf("Cache directory: %s", s.config.Server.CacheDir)
	log.Printf("Max cache files: %d", s.config.Server.MaxCacheFiles)

	return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}

func main() {
	// CLIオプションを定義
	options := &CLIOptions{}

	flag.StringVar(&options.ConfigPath, "config", "config.yaml", "Path to config file")
	flag.IntVar(&options.Port, "port", 0, "Server port (overrides config)")
	flag.StringVar(&options.CacheDir, "cache-dir", "", "Cache directory (overrides config)")
	flag.IntVar(&options.MaxCacheFiles, "max-cache-files", 0, "Maximum number of cache files (overrides config)")
	flag.StringVar(&options.ProjectID, "project-id", "", "GCS project ID (overrides config and env)")
	flag.StringVar(&options.CredentialsFile, "credentials-file", "", "GCS credentials file path (overrides config and env)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nGCS Image Resize Server\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment Variables:\n")
		fmt.Fprintf(os.Stderr, "  GOOGLE_CLOUD_PROJECT        GCS project ID\n")
		fmt.Fprintf(os.Stderr, "  GOOGLE_APPLICATION_CREDENTIALS  Path to GCS credentials file\n")
	}

	flag.Parse()

	server, err := NewServer(options)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	if err := server.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
