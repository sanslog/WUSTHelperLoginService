import requests
import uuid
import time
import json
from typing import List

"""
脚本说明: 从教务系统拿到100个真实的验证码图片的base64编码,详细逻辑见下面程序
最后是将结果写入output.txt中

"""

# 基础URL
BASE_URL = "https://auth.wust.edu.cn/lyuapServer/kaptcha"

def generate_uid() -> str:
    """
    生成符合 UUID 版本4标准的 uid
    与 Go 代码中的 generateUID() 逻辑完全一致
    """
    # 生成 UUID4 (随机UUID)
    uid = uuid.uuid4()
    # 返回不带连字符的十六进制字符串（与Go的hex.EncodeToString结果一致）
    return uid.hex

def get_kaptcha_json(client: requests.Session) -> dict:
    """
    获取验证码JSON响应
    完全模拟 Go 代码的 GetKaptchaJSON 函数逻辑
    """
    # 生成 uid
    uid = generate_uid()
    
    # 使用当前时间戳（毫秒）
    timestamp = int(time.time() * 1000)
    
    # 构建URL（与Go代码格式完全一致）
    url = f"{BASE_URL}?_t={timestamp}&uid={uid}"
    
    # 发送GET请求
    response = client.get(url)
    
    # 检查状态码
    response.raise_for_status()
    
    # 解析JSON
    kaptcha_resp = response.json()
    
    return kaptcha_resp

def main():
    """
    主函数：请求100次，提取content字段，保存到output.txt
    """
    contents: List[str] = []
    
    # 创建带连接池的 Session（提高性能，类似 Go 的 http.Client）
    with requests.Session() as session:
        # 设置超时时间
        session.timeout = 10
        
        for i in range(100):
            try:
                # 发送请求
                resp_data = get_kaptcha_json(session)
                
                # 提取 content 字段
                content = resp_data.get('content', '')
                contents.append(content)
                
                print(f"请求 {i+1}/100 成功, content长度: {len(content)}")
                
                # 间隔100ms（除了最后一次）
                if i < 99:
                    time.sleep(0.1)
                    
            except requests.exceptions.RequestException as e:
                print(f"请求 {i+1}/100 失败: {e}")
                # 失败时添加空字符串占位
                contents.append("")
                if i < 99:
                    time.sleep(0.1)
            except json.JSONDecodeError as e:
                print(f"请求 {i+1}/100 JSON解析失败: {e}")
                contents.append("")
                if i < 99:
                    time.sleep(0.1)
    
    # 将结果写入 output.txt 文件
    output_file = "output.txt"
    with open(output_file, "w", encoding="utf-8") as f:
        for content in contents:
            f.write(content + "\n")
    
    print(f"\n完成！共获取 {len(contents)} 个验证码，已保存到 {output_file}")

if __name__ == "__main__":
    main()