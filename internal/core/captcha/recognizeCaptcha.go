package captcha

import (
	logger "WUSThelperLoginSystem/internal/util"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/otiai10/gosseract/v2"
)

// ======================= 全局状态 =======================

const (
	ocrTimeout                = 2 * time.Second
	queueTimeout              = 1000 * time.Millisecond
	clearDictCahceLimit int32 = 2000
)

var (
	taskQueue chan ocrTask

	workerNum int

	wgWorkers  sync.WaitGroup // worker 生命周期
	wgTasks    sync.WaitGroup // 任务计数（优雅关闭用）
	initOnce   sync.Once
	shutdownMu sync.Mutex

	closed    int32 // 0=运行中 1=关闭中
	ocrCount  int32 //计数器
	cleanChan chan struct{}
)

// ======================= 数据结构 =======================
type ocrTask struct {
	img  []byte
	resp chan ocrResult
}

type ocrResult struct {
	text string
	err  error
}

//=======================生命周期相关函数==================

// InitOCR 初始化
func InitOCR(workers int, queueSize int) {
	initOnce.Do(func() {
		workerNum = workers
		taskQueue = make(chan ocrTask, queueSize)
		cleanChan = make(chan struct{})
		for i := 0; i < workerNum; i++ {
			wgWorkers.Add(1)
			go startWorker()
		}
		go cleanDictCache()
	})

}

// worker：每个 goroutine 独占一个 client
func startWorker() {
	defer wgWorkers.Done()

	client := gosseract.NewClient()
	defer client.Close()

	client.SetPageSegMode(gosseract.PSM_SINGLE_LINE)
	client.SetWhitelist("0123456789+-*/=")

	for task := range taskQueue {

		// 同步执行 OCR（绝不并发）
		err := client.SetImageFromBytes(task.img)
		if err != nil {
			safeSend(task.resp, ocrResult{"", err})
			wgTasks.Done()
			continue
		}

		text, err := client.Text()
		safeSend(task.resp, ocrResult{text, err})

		wgTasks.Done()
		atomic.AddInt32(&ocrCount, 1)
		validCleanCount()
	}
}

func safeSend(ch chan ocrResult, r ocrResult) {
	defer func() {
		if recover() != nil {
			// 防止调用方已放弃接收导致 panic
		}
	}()
	ch <- r
}

// Shutdown，释放ocr进程
func ShutdownOCR() {

	shutdownMu.Lock()
	defer shutdownMu.Unlock()
	if !atomic.CompareAndSwapInt32(&closed, 0, 1) {
		return
	}
	wgTasks.Wait()
	close(taskQueue)
	close(cleanChan)
	wgWorkers.Wait()
}

// tessarct-OCR清理字典缓存
func cleanDictCache() {
	for {
		//阻塞等待
		_, ok := <-cleanChan
		//协程退出
		if !ok {
			break
		}
		gosseract.ClearPersistentCache()
		logger.Log.Info("完成一次字典缓存清理")
	}
}

func validCleanCount() {
	if old := atomic.LoadInt32(&ocrCount); old >= clearDictCahceLimit {
		for {
			// 再次原子读取最新值（防止并发被修改）
			current := atomic.LoadInt32(&ocrCount)
			// 如果最新值已经 < 阈值了，说明被别的协程清过了，直接退出
			if current < clearDictCahceLimit {
				break
			}
			// 原子 CAS：只有当前值还是 current，才置 0
			if atomic.CompareAndSwapInt32(&ocrCount, current, 0) {
				// 成功清零，current 就是你取到的值
				logger.Log.Info("触发tessarct清理字典缓存")
				cleanChan <- struct{}{}
				break
			}
			// CAS 失败 = 并发被改了，自动重试
		}
	}
}

// =========================== 主入口 ==========================
// 提供给外部调用ocr服务的接口
func RecognizeMathCaptcha(ctx context.Context, img64 string) (float64, error) {

	processed, err := preprocessCaptcha(img64)
	if err != nil {
		return 0, err
	}

	text, err := runOCR(ctx, processed)
	if err != nil {
		return 0, err
	}

	return calculateExpression(cleanText(text))
}

func runOCR(ctx context.Context, img []byte) (string, error) {

	// 已关闭直接拒绝
	if atomic.LoadInt32(&closed) == 1 {
		return "", fmt.Errorf("OCR服务已关闭")
	}

	resp := make(chan ocrResult, 1)

	task := ocrTask{
		img:  img,
		resp: resp,
	}

	// 增加任务计数（必须在投递前）
	wgTasks.Add(1)

	// 投递任务（带超时）
	select {
	case taskQueue <- task:
	case <-time.After(queueTimeout):
		wgTasks.Done()
		return "", fmt.Errorf("OCR队列已满")
	case <-ctx.Done():
		wgTasks.Done()
		return "", ctx.Err() //上游context取消
	}

	// 等待结果
	select {
	case r := <-resp:
		if r.err != nil {
			return "", r.err
		}
		return strings.TrimSpace(r.text), nil

	case <-ctx.Done():
		return "", ctx.Err()

	case <-time.After(ocrTimeout):
		return "", fmt.Errorf("OCR超时")
	}
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

			y0 := uint8((299*uint32(r8) + 587*uint32(g8) + 114*uint32(b8)) / 1000)

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

// ======================= 文本处理 =======================

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

	return 0, fmt.Errorf("未识别运算符")
}

// ======================= 工具函数 =======================

func decodeBase64(s string) ([]byte, error) {
	if i := strings.Index(s, ","); i != -1 {
		s = s[i+1:]
	}
	return base64.StdEncoding.DecodeString(s)
}

func isLightColorButNotWhite(r, g, b uint8) bool {
	if r > 240 && g > 240 && b > 240 {
		return false
	}
	max := max3(r, g, b)
	min := min3(r, g, b)
	return max >= 180 && (max-min) >= 12
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
