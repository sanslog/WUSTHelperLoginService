package captcha

import (
	under "WUSThelperLoginSystem/internal/core/captcha/internal"
	logger "WUSThelperLoginSystem/internal/util"
	"context"
	"fmt"
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
// 截止目前，auth用途不明，但是为了代码安全，请不要动这部分了
func RecognizeMathCaptcha(ctx context.Context, img64 string, auth int) (float64, error) {

	if auth == 0 {
		//  preprocessCaptcha(img64)
		processed, err := under.UnderGraduatepreprocessCaptcha(img64)
		if err != nil {
			return 0, err
		}

		text, err := runOCR(ctx, processed)
		if err != nil {
			return 0, err
		}

		return under.UnderGraduatecalculateExpression(under.UnderGraduatecleanText(text))

	} else {
		return 0, nil
	}

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
		return "", fmt.Errorf("服务器繁忙，请稍后重试")
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
