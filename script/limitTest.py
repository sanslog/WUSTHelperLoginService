import time
import threading
import requests
from datetime import datetime

# ===================== 配置项 =====================
TEST_URL = "http://localhost:8080/login"
REQUEST_BODY = {"username": "202413407195", "password": "eye15236"}
TARGET_QPS = 100  # 目标每秒请求数
TEST_DURATION = 5  # 测试持续时间（秒）
# ====================================================

# 统计变量
total_requests = 0
success_count = 0
fail_count = 0
lock = threading.Lock()

def send_request():
    """发送单个POST请求"""
    global total_requests, success_count, fail_count
    try:
        resp = requests.post(
            TEST_URL,
            json=REQUEST_BODY,
            timeout=5
        )
        try:
            data = resp.json()
        except:
            with lock:
                total_requests+=1
                fail_count+=1
                return
        with lock:
            total_requests += 1
            if data.get("error") is None:
                success_count += 1
            else:
                fail_count += 1
    except Exception:
        with lock:
            total_requests += 1
            fail_count += 1

def worker(qps, duration):
    """按QPS持续发送请求"""
    interval = 1.0 / qps  # 每个请求间隔
    end_time = time.time() + duration
    
    while time.time() < end_time:
        threading.Thread(target=send_request, daemon=True).start()
        time.sleep(interval)

def main():
    print(f"===== 开始压测 =====")
    print(f"请求地址: {TEST_URL}")
    print(f"目标 QPS: {TARGET_QPS}")
    print(f"测试时长: {TEST_DURATION} 秒\n")
    
    start_time = datetime.now()
    start_total = time.time()
    
    # 启动压测协程
    worker(TARGET_QPS, TEST_DURATION)
    
    # 等待所有请求完成
    time.sleep(1)
    
    # 计算结果
    cost_time = time.time() - start_total
    real_qps = round(total_requests / cost_time, 2)
    
    print("\n===== 压测完成 =====")
    print(f"总耗时: {round(cost_time, 2)} 秒")
    print(f"总请求数: {total_requests}")
    print(f"成功: {success_count}")
    print(f"失败: {fail_count}")
    print(f"实际 QPS: {real_qps}")

if __name__ == "__main__":
    main()