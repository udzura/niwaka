package main

import (
	"image"
	"image/color"
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParseSize(t *testing.T) {
	tests := []struct {
		input    string
		expected Size
		hasError bool
	}{
		{"1000x1000", Size{Width: 1000, Height: 1000}, false},
		{"300x300", Size{Width: 300, Height: 300}, false},
		{"30x30", Size{Width: 30, Height: 30}, false},
		{"invalid", Size{}, true},
		{"100x", Size{}, true},
		{"x100", Size{}, true},
	}

	for _, test := range tests {
		result, err := parseSize(test.input)
		if test.hasError {
			if err == nil {
				t.Errorf("Expected error for input %s, but got none", test.input)
			}
		} else {
			if err != nil {
				t.Errorf("Unexpected error for input %s: %v", test.input, err)
			}
			if result != test.expected {
				t.Errorf("Expected %v for input %s, but got %v", test.expected, test.input, result)
			}
		}
	}
}

func TestResizeImage(t *testing.T) {
	// 100x100の単色画像を作成
	src := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			src.Set(x, y, color.RGBA{255, 0, 0, 255}) // 赤色
		}
	}

	// 50x50にリサイズ
	resized := resizeImage(src, 50, 50)
	bounds := resized.Bounds()

	if bounds.Max.X != 50 || bounds.Max.Y != 50 {
		t.Errorf("Expected resized image to be 50x50, but got %dx%d", bounds.Max.X, bounds.Max.Y)
	}

	// リサイズ後も色が保持されているかチェック
	c := resized.At(25, 25)
	r, g, b, a := c.RGBA()
	if r == 0 || g != 0 || b != 0 || a == 0 {
		t.Errorf("Expected red color to be preserved after resize, but got R:%d G:%d B:%d A:%d", r, g, b, a)
	}
}

func TestCacheManager(t *testing.T) {
	cm := NewCacheManager("./test_cache", 2)
	defer func() {
		// テスト後にクリーンアップ
		// 実際のテストではtmp dirを使用するべき
	}()

	testData := []byte("test image data")
	cacheKey := "test_key"

	// キャッシュに保存
	err := cm.Set(cacheKey, testData)
	if err != nil {
		t.Errorf("Failed to set cache: %v", err)
	}

	// キャッシュから取得
	data, found := cm.Get(cacheKey)
	if !found {
		t.Error("Expected to find cached data, but not found")
	}
	if string(data) != string(testData) {
		t.Errorf("Expected cached data to be %s, but got %s", string(testData), string(data))
	}

	// 存在しないキーの取得
	_, found = cm.Get("nonexistent_key")
	if found {
		t.Error("Expected not to find nonexistent key, but found")
	}
}

func TestResolveBucketName(t *testing.T) {
	// テスト用の設定を作成
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

	tests := []struct {
		alias       string
		expected    string
		expectError bool
	}{
		{"images", "my-company-prod-images-bucket", false},
		{"assets", "my-company-prod-assets-bucket", false},
		{"uploads", "my-company-user-uploads-bucket", false},
		{"nonexistent", "", true},
		{"", "", true},
	}

	for _, test := range tests {
		result, err := server.resolveBucketName(test.alias)
		if test.expectError {
			if err == nil {
				t.Errorf("Expected error for alias %s, but got none", test.alias)
			}
		} else {
			if err != nil {
				t.Errorf("Unexpected error for alias %s: %v", test.alias, err)
			}
			if result != test.expected {
				t.Errorf("Expected %s for alias %s, but got %s", test.expected, test.alias, result)
			}
		}
	}
}

func TestConfigLoad(t *testing.T) {
	// テスト用の一時設定ファイルを作成
	configContent := `buckets:
  test: "test-bucket"
  demo: "demo-bucket"
assortments:
  avatar:
    large: 1000x1000
    medium: 300x300
server:
  port: 8080
  cache_dir: "./cache"
  max_cache_files: 1000
gcs:
  project_id: "test-project"
  credentials_file: ""`

	// 一時ファイルに書き込み
	tempFile := "./test_config.yaml"
	err := os.WriteFile(tempFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}
	defer os.Remove(tempFile)

	// 設定読み込みテスト
	configData, err := os.ReadFile(tempFile)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	var config Config
	if err := yaml.Unmarshal(configData, &config); err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	// 検証
	if len(config.Buckets) != 2 {
		t.Errorf("Expected 2 bucket aliases, but got %d", len(config.Buckets))
	}

	if config.Buckets["test"] != "test-bucket" {
		t.Errorf("Expected bucket 'test' to resolve to 'test-bucket', but got %s", config.Buckets["test"])
	}

	if config.Server.Port != 8080 {
		t.Errorf("Expected port to be 8080, but got %d", config.Server.Port)
	}
}
