package ticket

import (
	"errors"
	"net/http"
	"net/url"
)

const (
	baseURL    = "https://bkjx.wust.edu.cn"        //SEVERID的网域
	entryURL   = "https://bkjx.wust.edu.cn/jsxsd/" //bzb_jsxsd的网域
	casReferer = "https://auth.wust.edu.cn/"
	userAgent  = "Mozilla/5.0"
)

// FetchBKJXCookies 使用 CAS ticket 换取 bzb_jsxsd 和 SERVERID
func FetchBKJXCookies(client *http.Client, ticket string) (string, string, error) {
	if ticket == "" {
		return "", "", errors.New("ticket is empty")
	}
	if client == nil || client.Jar == nil {
		return "", "", errors.New("http client or cookie jar is nil")
	}

	// ---------- 第一次请求 ----------
	req1, err := http.NewRequest(
		http.MethodGet,
		entryURL+"?ticket="+ticket,
		nil,
	)
	if err != nil {
		return "", "", err
	}

	req1.Header.Set("User-Agent", userAgent)
	req1.Header.Set("Referer", casReferer)

	resp1, err := client.Do(req1)
	if err != nil {
		return "", "", err
	}
	resp1.Body.Close()

	if resp1.StatusCode != http.StatusFound {
		return "", "", errors.New("unexpected status code on first redirect")
	}

	location := resp1.Header.Get("Location")
	if location == "" {
		return "", "", errors.New("missing Location header")
	}

	// ---------- 第二次请求：直接使用 Location ----------
	req2, err := http.NewRequest(http.MethodGet, location, nil)
	if err != nil {
		return "", "", err
	}

	req2.Header.Set("User-Agent", userAgent)

	resp2, err := client.Do(req2)
	if err != nil {
		return "", "", err
	}
	resp2.Body.Close()

	if resp2.StatusCode != http.StatusFound {
		return "", "", errors.New("unexpected status code on second redirect")
	}

	//---------- 读取 Cookie ----------
	jar := client.Jar

	uBase, _ := url.Parse(baseURL)
	cookiesBase := jar.Cookies(uBase)

	uEntry, _ := url.Parse(entryURL)
	cookiesEntry := jar.Cookies(uEntry)

	var bzbJSXSD, serverID string
	for _, c := range cookiesBase {
		if c.Name == "SERVERID" {
			serverID = c.Value
			break
		}
	}

	for _, c := range cookiesEntry {
		if c.Name == "bzb_jsxsd" {
			bzbJSXSD = c.Value
			break
		}
	}

	if bzbJSXSD == "" || serverID == "" {
		return "", "", errors.New("required cookies not found")
	}

	return bzbJSXSD, serverID, nil
}
