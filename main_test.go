package main

import (
	"image"
	"image/color"
	"testing"
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
