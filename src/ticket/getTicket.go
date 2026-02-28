package ticket

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	targetURL = "https://auth.wust.edu.cn/lyuapServer/v1/tickets"
	//密码加密:
	modulusHex  = "b5eeb166e069920e80bebd1fea4829d3d1f3216f2aabe79b6c47a3c18dcee5fd22c2e7ac519cab59198ece036dcf289ea8201e2a0b9ded307f8fb704136eaeb670286f5ad44e691005ba9ea5af04ada5367cd724b5a26fdb5120cc95b6431604bd219c6b7d83a6f8f24b43918ea988a76f93c333aa5a20991493d4eb1117e7b1"
	exponentHex = "10001"
)

const ( //错误类型
	PASSERROR    = "PASSERROR"
	CODEFALSE    = "CODEFALSE"
	NOUSER       = "NOUSER"
	USERLOCK     = "USERLOCK"
	USERDISABLED = "USERDISABLED"
	NOREGISTER   = "NOREGISTER"
)

type CASmsg struct {
	Tgt    string `json:"tgt"`
	Ticket string `json:"ticket"`
	Data   struct {
		Code string `json:"code"`
		Data string `json:"data"`
	} `json:"data"`
}

func LoginAndGetTicket(client *http.Client, username, password, id string, code float64) (*CASmsg, error) {

	encryptedPassword, err := encryptPassword(password)
	if err != nil {
		return nil, err
	}

	// 表单数据
	formData := url.Values{
		"username":  {username},
		"password":  {encryptedPassword},
		"service":   {"https://bkjx.wust.edu.cn/jsxsd/"},
		"loginType": {},
		"id":        {id},
		"code":      {strconv.FormatFloat(code, 'f', 0, 64)},
	}

	// 创建请求
	req, err := http.NewRequest("POST", targetURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, err
	}

	// 设置必须的请求头
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", "https://auth.wust.edu.cn/lyuapServer/login?service=https%3A%2F%2Fbkjx.wust.edu.cn%2Fjsxsd%2F")
	req.Header.Set("Origin", "https://auth.wust.edu.cn")

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	//反序列化
	var msg CASmsg
	err = json.Unmarshal(body, &msg)
	if err != nil {
		return nil, err
	}

	//进行code验证
	if msg.Data.Code != "" {
		result := judgeErr(msg.Data.Code)
		if result.Error() == PASSERROR {
			result = errors.New("密码错误[锁定,剩余]:[" + msg.Data.Data + "]")
		}
		return nil, result
	}

	return &msg, nil
}

// ==========密码加密============
func encryptPassword(password string) (string, error) {

	// 1. 反转明文
	plain := []byte(reverseString(password))

	// 2. 构造大整数 m
	m := new(big.Int).SetBytes(plain)

	// 3. 构造 RSA 公钥参数
	n := new(big.Int)
	n.SetString(modulusHex, 16)

	e := new(big.Int)
	e.SetString(exponentHex, 16)

	// 4. RSA NoPadding: c = m^e mod n
	c := new(big.Int).Exp(m, e, n)

	// 5. 输出为定长字节（左侧补 0）
	keyLen := (n.BitLen() + 7) / 8
	out := c.FillBytes(make([]byte, keyLen))

	// 6. hex 小写
	return hex.EncodeToString(out), nil
}

func reverseString(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

// =============错误类型判断============
func judgeErr(code string) error {
	switch code {
	case PASSERROR:
		return errors.New("密码错误")
	case CODEFALSE:
		return errors.New("CODEFALSE")
	case USERLOCK:
		return errors.New("登录验证次数达到上限，用户被锁定")
	case NOUSER:
		return errors.New("用户不存在")
	case USERDISABLED:
		return errors.New("账户被停用")
	case NOREGISTER:
		return errors.New("用户未注册")
	default:
		return errors.New("未知错误")
	}

}
