package captcha

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const baseURL = "https://auth.wust.edu.cn/lyuapServer/kaptcha"

// KaptchaResponse 定义JSON响应结构体
type KaptchaResponse struct {
	KaptchaType string `json:"kaptchaType"`
	UID         string `json:"uid"`
	Content     string `json:"content"`
	Timeout     int    `json:"timeout"`
}

// ===========根据uid获取对应验证码图片=============
func GetKaptchaJSON(client *http.Client) (*KaptchaResponse, error) {
	uid, err := generateUID()
	if err != nil {
		return nil, fmt.Errorf("uid生成失败: %v", err)
	}

	// 使用当前时间戳（毫秒）
	timestamp := time.Now().UnixMilli()
	url := fmt.Sprintf("%s?_t=%d&uid=%s", baseURL, timestamp, uid)

	// 发送GET请求
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("服务器返回错误状态码: %d", resp.StatusCode)
	}

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	// 解析JSON
	var kaptchaResp KaptchaResponse
	if err := json.Unmarshal(body, &kaptchaResp); err != nil {
		return nil, fmt.Errorf("解析JSON失败: %v\n响应内容: %s", err, string(body))
	}

	return &kaptchaResp, nil
}

// ===========生成uid============
func generateUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b), nil
}
