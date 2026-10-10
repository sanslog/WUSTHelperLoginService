package under

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// 图片预处理：

func UnderGraduatepreprocessCaptcha(img64 string) ([]byte, error) {
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

func UnderGraduatecleanText(text string) string {
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

func UnderGraduatecalculateExpression(text string) (float64, error) {
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
