package captcha

import (
	"context"
	"net/http"
	"testing"
	"time"
)

const (
	testTesseractPath = "C:\\Users\\39737\\AppData\\Local\\Programs\\Tesseract-OCR\\tesseract.exe"
	testOCRWorkers    = 3
)

/**
* 2026/3/9最后一次基准测试qps:222
 */
// BenchmarkRecognizeMathCaptcha
// 模拟真实用户请求验证码并识别
func BenchmarkRecognizeMathCaptcha(b *testing.B) {

	// 初始化 OCR
	err := InitOCR(testTesseractPath, testOCRWorkers)
	if err != nil {
		b.Fatalf("InitOCR failed: %v", err)
	}
	defer ShutdownOCR()

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// 预热
	for i := 0; i < 10; i++ {
		resp, err := GetKaptchaJSON(client)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, _ = RecognizeMathCaptcha(ctx, resp.Content)
		cancel()
	}

	b.ResetTimer()

	start := time.Now()

	// 并发模拟真实用户
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {

			resp, err := GetKaptchaJSON(client)
			if err != nil {
				b.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

			_, err = RecognizeMathCaptcha(ctx, resp.Content)

			cancel()

			if err != nil {
				// 真实系统中允许一定失败率
				continue
			}
		}
	})

	duration := time.Since(start)

	total := b.N
	qps := float64(total) / duration.Seconds()

	b.ReportMetric(qps, "qps")
}
