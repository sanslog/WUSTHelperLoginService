package sever

import (
	"WUSThelperLoginSystem/test/captcha"
	"WUSThelperLoginSystem/test/ticket"
	logger "WUSThelperLoginSystem/test/util"
	"context"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"time"
)

// 对于登录请求因为codefalse导致的请求失败所进行的最大重试次数
const maxRetry = 5

func MockLogin(ctx context.Context, username, rawPassword string) (string, string, string, error) {

	tryTimes := 0

	//cookie和seesion管理
	jar, err := cookiejar.New(nil)
	if err != nil {
		logger.Log.Error("创建cookieJar失败:" + err.Error())
		panic(err)
	}

	//使用同一个client
	client := &http.Client{
		Jar:     jar,
		Timeout: 10 * time.Second,
		//关闭客户端自动重定向，手动处理请求
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

retryHere:
	/*<p>第一步，使用本地生成uid作为凭证请求验证码图片</p>
	* <p>我多说一点吧，这里的逻辑是本地生成一个uid，把这个uid作为参数放到到(GET)请求参数上,</p>
	* <p>然后把当前时间戳作为参数也放进去，最后就会返回验证码图片的base64字符串编码，这个在服务器</p>
	* <p>上的实现应该是拿到前端的uid，然后生成一个验证码图片，将图片与uid绑定放到redis中缓存，有效期300s</p>
	 */
	kapMsg, GetKaptchaJSONErr := captcha.GetKaptchaJSON(client)
	if GetKaptchaJSONErr != nil {
		logger.Log.Error("uid生成失败: " + GetKaptchaJSONErr.Error())
		return "", "", "", errors.New("服务器内部错误")
	}

	/*第二步，获取验证码计算值*/
	code, codeErr := captcha.RecognizeMathCaptcha(ctx, kapMsg.Content)
	if codeErr != nil {
		logger.Log.Error("图片解析失败: " + codeErr.Error())

		tryTimes++
		if tryTimes < maxRetry {
			time.Sleep(50 * time.Millisecond)
			goto retryHere
		}

		return "", "", "", errors.New("服务器内部错误")
	}

	/*第三步，获取ticket*/
	result, ticketErr := ticket.LoginAndGetTicket(client, username, rawPassword, kapMsg.UID, code)
	if ticketErr != nil {
		logger.Log.Info("用户" + username + " 获取ticket失败: " + ticketErr.Error())
		//codefalse就retry,其它错误则原样报上去
		if ticketErr.Error() == ticket.CODEFALSE {
			if tryTimes < maxRetry {
				time.Sleep(50 * time.Millisecond)
				goto retryHere
			}
		}

		return "", "", "", ticketErr
	}

	/*<p>第四步，两次重定向对CAS服务器进行验证</p>
	* <p>这里我采取手动重定向，避免浪费网络请求资源</p>
	 */
	bzbj, severID, cookieErr := ticket.FetchBKJXCookies(client, result.Ticket)
	logger.Log.Info("用户" + username + ": bzbj:" + bzbj + " severID:" + severID)
	if cookieErr != nil {
		logger.Log.Error("获取cookie失败: " + cookieErr.Error())
		return "", "", "", errors.New("服务器内部错误")
	}

	return bzbj, severID, result.Tgt, nil
}
