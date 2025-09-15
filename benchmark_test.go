package main

import (
	"image"
	"image/color"
	"os"
	"testing"
)

// ベンチマークテスト
func BenchmarkParseSize(b *testing.B) {
	for i := 0; i < b.N; i++ {
		parseSize("1000x1000")
	}
}

func BenchmarkResizeImage(b *testing.B) {
	// 100x100の単色画像を作成
	src := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			src.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resizeImage(src, 50, 50)
	}
}

func BenchmarkResolveBucketName(b *testing.B) {
	config := &Config{
		Buckets: map[string]string{
			"images":  "my-company-prod-images-bucket",
			"assets":  "my-company-prod-assets-bucket",
			"uploads": "my-company-user-uploads-bucket",
		},
	}

	server := &Server{
		config: config,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.resolveBucketName("images")
	}
}

func BenchmarkCacheOperations(b *testing.B) {
	cm := NewCacheManager("./bench_cache", 100)
	defer os.RemoveAll("./bench_cache")

	testData := []byte("test image data for benchmarking")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cacheKey := "benchmark_key"
		cm.Set(cacheKey, testData)
		cm.Get(cacheKey)
	}
}

// より大きな画像でのリサイズベンチマーク
func BenchmarkResizeImageLarge(b *testing.B) {
	// 500x500の画像を作成
	src := image.NewRGBA(image.Rect(0, 0, 500, 500))
	for y := 0; y < 500; y++ {
		for x := 0; x < 500; x++ {
			src.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 128, 255})
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resizeImage(src, 100, 100)
	}
}

// キャッシュマネージャーのLRU動作ベンチマーク
func BenchmarkCacheLRU(b *testing.B) {
	cm := NewCacheManager("./bench_lru_cache", 10) // 小さなキャッシュサイズ
	defer os.RemoveAll("./bench_lru_cache")

	testData := []byte("test data")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// キャッシュサイズを超えるキーを使用してLRU動作をトリガー
		cacheKey := string(rune('a' + (i % 20)))
		cm.Set(cacheKey, testData)
		cm.Get(cacheKey)
	}
}
