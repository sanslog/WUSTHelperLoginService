package handler

import (
	"WUSThelperLoginSystem/internal/serve"
	logger "WUSThelperLoginSystem/internal/util"
	"encoding/json"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	// 只允许 POST
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(`{"error":"only POST is allowed"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	// 限制请求体大小
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()

	//解析请求体json字符串
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()

	var data map[string]string
	if err := decoder.Decode(&data); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid json format"}`))
		logger.Log.Error("login---json解析失败", zap.Error(err))
		return
	}

	username := strings.TrimSpace(data["username"])
	password := strings.TrimSpace(data["password"])

	if username == "" || password == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"username or password is empty"}`))
		return
	}

	// 登录逻辑
	ctx := r.Context()
	bzbj, serverID, tgc, loginErr := serve.MockLogin(ctx, username, password)

	if loginErr != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"` + loginErr.Error() + `"}`))
		return
	}

	// 成功返回
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(
		`{
			"cookie":"bzb_jsxsd=` + bzbj + `;SERVERID=` + serverID + `",
			"TGC":"` + tgc + `"
		}`,
	))
	logger.Log.Info(username + "登录成功")
}
