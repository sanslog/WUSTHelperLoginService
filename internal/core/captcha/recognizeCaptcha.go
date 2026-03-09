package captcha

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ocrTimeout   = 2 * time.Second
	queueTimeout = 1000 * time.Millisecond
)

var (
	maxOCRWorkers int
	tesseractPath string
	ocrSem        chan struct{}
	tmpDir        string
	initOnce      sync.Once
)

// ======================= 生命周期管理 =======================

// Init 必须在服务启动时调用
func InitOCR(path string, maxNum int) error {
	tesseractPath = path
	maxOCRWorkers = maxNum
	var err error
	initOnce.Do(func() {
		ocrSem = make(chan struct{}, maxOCRWorkers)
		tmpDir, err = os.MkdirTemp("", "ocr-runtime-*")
	})
	return err
}

// Shutdown 在服务关闭时调用
func ShutdownOCR() {
	if tmpDir != "" {
		_ = os.RemoveAll(tmpDir)
	}
}

// ======================= 对外主入口 =======================
// 实现了资源池模式
func RecognizeMathCaptcha(ctx context.Context, img64 string) (float64, error) {
	// 获取 OCR slot（带超时）
	select {
	case ocrSem <- struct{}{}:
		defer func() { <-ocrSem }()
	case <-time.After(queueTimeout):
		return 0, fmt.Errorf("OCR 队列繁忙")
	case <-ctx.Done():
		return 0, ctx.Err()
	}

	processed, err := preprocessCaptcha(img64)
	if err != nil {
		return 0, fmt.Errorf("预处理失败: %w", err)
	}

	text, err := runTesseractOCR(processed)
	if err != nil {
		return 0, fmt.Errorf("OCR 失败: %w", err)
	}

	return calculateExpression(cleanText(text))
}

// ======================= OCR 调用 =======================

func runTesseractOCR(img []byte) (string, error) {
	f, err := os.CreateTemp(tmpDir, "ocr-*.png")
	if err != nil {
		return "", err
	}
	defer func() {
		f.Close()
		os.Remove(f.Name())
	}()

	if _, err := f.Write(img); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), ocrTimeout)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		tesseractPath,
		f.Name(),
		"stdout",
		"--oem", "1",
		"--psm", "7",
		"-c", "tessedit_char_whitelist=0123456789+-*/=xX",
	)

	cmd.Env = append(os.Environ(),
		"OMP_THREAD_LIMIT=1",
		"OMP_NUM_THREADS=1",
	)

	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v (%s)", err, errBuf.String())
	}

	return strings.TrimSpace(out.String()), nil
}

// ======================= 图片预处理 =======================

func preprocessCaptcha(img64 string) ([]byte, error) {
	raw, err := decodeBase64(img64)
	if err != nil {
		return nil, err
	}

	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}

	bounds := src.Bounds()
	gray := image.NewGray(bounds)

	hist := [256]int{}

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := src.At(x, y).RGBA()
			r8 := uint8(r >> 8)
			g8 := uint8(g >> 8)
			b8 := uint8(b >> 8)

			// 原始灰度
			y0 := uint8(
				(299*uint32(r8) + 587*uint32(g8) + 114*uint32(b8)) / 1000,
			)

			// ===== 关键补偿逻辑 =====
			// 高亮但非白的彩色字符，主动压暗
			if isLightColorButNotWhite(r8, g8, b8) {
				if y0 > 160 {
					y0 = 160
				}
			}

			gray.SetGray(x, y, color.Gray{y0})
			hist[y0]++
		}
	}

	threshold := otsu(hist, bounds.Dx()*bounds.Dy())
	binary := image.NewGray(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if gray.GrayAt(x, y).Y < uint8(threshold) {
				binary.SetGray(x, y, color.Gray{0})
			} else {
				binary.SetGray(x, y, color.Gray{255})
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, binary); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func isLightColorButNotWhite(r, g, b uint8) bool {
	max := max3(r, g, b)
	min := min3(r, g, b)

	// 非白（防止误伤背景）
	if r > 240 && g > 240 && b > 240 {
		return false
	}

	// 亮度高
	if max < 180 {
		return false
	}

	// 有颜色差（说明不是灰）
	if max-min < 12 {
		return false
	}

	return true
}

func max3(a, b, c uint8) uint8 {
	if a > b {
		if a > c {
			return a
		}
		return c
	}
	if b > c {
		return b
	}
	return c
}

func min3(a, b, c uint8) uint8 {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// ======================= 文本处理 & 计算 =======================

func cleanText(text string) string {
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "x", "*")
	text = strings.ReplaceAll(text, "X", "*")

	var b strings.Builder
	for _, r := range text {
		if (r >= '0' && r <= '9') || strings.ContainsRune("+-*/=", r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func calculateExpression(text string) (float64, error) {
	text = strings.ReplaceAll(text, "=", "")
	if len(text) < 3 {
		return 0, fmt.Errorf("表达式过短: %q", text)
	}

	for i := 1; i < len(text); i++ {
		if strings.ContainsRune("+-*/", rune(text[i])) {
			a, err1 := strconv.ParseFloat(text[:i], 64)
			b, err2 := strconv.ParseFloat(text[i+1:], 64)
			if err1 != nil || err2 != nil {
				return 0, fmt.Errorf("数字解析失败")
			}
			switch text[i] {
			case '+':
				return a + b, nil
			case '-':
				return a - b, nil
			case '*':
				return a * b, nil
			case '/':
				if math.Abs(b) < 1e-9 {
					return 0, fmt.Errorf("除零")
				}
				return math.Round((a/b)*100) / 100, nil
			}
		}
	}

	fmt.Println("code_text:", text) //TODO: 删除多余输出语句
	return 0, fmt.Errorf("未识别运算符")
}

// ======================= 工具函数 =======================

func decodeBase64(s string) ([]byte, error) {
	if i := strings.Index(s, ","); i != -1 {
		s = s[i+1:]
	}
	return base64.StdEncoding.DecodeString(s)
}

func otsu(hist [256]int, total int) int {
	var sum int
	for i := 0; i < 256; i++ {
		sum += i * hist[i]
	}
	var sumB, wB, max int
	var t1, t2 int

	for i := 0; i < 256; i++ {
		wB += hist[i]
		if wB == 0 {
			continue
		}
		wF := total - wB
		if wF == 0 {
			break
		}
		sumB += i * hist[i]
		mB := float64(sumB) / float64(wB)
		mF := float64(sum-sumB) / float64(wF)
		v := float64(wB*wF) * (mB - mF) * (mB - mF)
		if int(v) >= max {
			if int(v) > max {
				max = int(v)
				t1, t2 = i, i
			} else {
				t2 = i
			}
		}
	}
	return (t1 + t2) / 2
}
