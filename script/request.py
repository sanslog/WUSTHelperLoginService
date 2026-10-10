import urllib.request
import urllib.error
import time
from datetime import datetime
import sys

# 测试cookie失效时间的python脚本

# 目标URL
url = "https://bkjx.wust.edu.cn/jsxsd/framework/student/images/xs_bg.jpg"

# 自定义请求头
headers = {
    "Cookie": "testcookie",
    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
}

def send_request():
    """发送GET请求并返回结果"""
    current_time = datetime.now().strftime('%Y-%m-%d %H:%M:%S')
    
    try:
        # 创建请求对象
        req = urllib.request.Request(url, headers=headers, method='GET')
        
        # 发送请求并获取响应
        with urllib.request.urlopen(req, timeout=10) as response:
            status_code = response.getcode()
            
            # 读取响应内容（可选，这里只是读取但不使用）
            # content = response.read()
            # content_length = len(content)
            
            if status_code == 200:
                print(f"[{current_time}] ✅ 请求成功！状态码: {status_code}")
                # 可以取消下面的注释来查看响应内容大小
                # print(f"    响应大小: {content_length} 字节")
            else:
                print(f"[{current_time}] ⚠️ 请求返回状态码: {status_code}")
            
            return True
            
    except urllib.error.HTTPError as e:
        print(f"[{current_time}] ❌ HTTP错误: {e.code} - {e.reason}")
        return False
    except urllib.error.URLError as e:
        print(f"[{current_time}] ❌ URL错误: {e.reason}")
        return False
    except Exception as e:
        print(f"[{current_time}] ❌ 未知错误: {e}")
        return False

def main():
    """主函数，循环执行定时任务"""
    print("=" * 60)
    print("         定时HTTP请求脚本 (使用urllib原生库)")
    print("=" * 60)
    print(f"目标URL: {url}")
    print(f"Cookie头: {headers['Cookie']}")
    print(f"请求间隔: 5分钟 (300秒)")
    print("-" * 60)
    print("按 Ctrl+C 停止运行")
    print("=" * 60)
    print()
    
    request_count = 0
    
    while True:
        request_count += 1
        print(f"📋 第 {request_count} 次请求")
        
        # 发送请求
        send_request()
        
        # 计算下一次请求时间
        next_time = datetime.fromtimestamp(time.time() + 300)
        next_time_str = next_time.strftime('%Y-%m-%d %H:%M:%S')
        print(f"⏰ 下一次请求时间: {next_time_str}")
        print("-" * 60)
        
        # 等待5分钟
        try:
            time.sleep(300)
        except KeyboardInterrupt:
            print("\n🛑 收到中断信号，正在退出...")
            break

if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print("\n🛑 脚本已停止")
        sys.exit(0)