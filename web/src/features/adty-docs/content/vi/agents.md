# Tác nhân

Gọi mô hình có công cụ bằng máy khách tương thích OpenAI. Địa chỉ gốc là `{{BASE_URL}}/v1`, khóa là khóa ADTY AI. Vòng lặp tác nhân nằm trong chương trình của bạn: trang trả về lời gọi công cụ, bạn gửi kết quả ở yêu cầu kế tiếp.

```python
from openai import OpenAI

client = OpenAI(base_url="{{BASE_URL}}/v1", api_key="sk-your-key")
```

Khóa này được tính phí như khóa API thông thường. Không dùng nó để đọc ví hoặc đăng nhập.
