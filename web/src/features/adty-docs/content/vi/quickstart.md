# Bắt đầu nhanh

ADTY AI là một cổng API tương thích với API của OpenAI và Anthropic. Chỉ với một khóa API, bạn có thể gọi Claude, GPT, Gemini, DeepSeek và các mô hình khác từ những công cụ bạn đang dùng.

## 1. Tạo tài khoản và nạp tiền

1. [Đăng ký](/sign-up) và xác minh email.
2. Mở [Ví](/wallet) và nạp tiền. Phí được tính theo mức sử dụng: mỗi yêu cầu bị tính phí theo số token đã dùng, với mức giá hiển thị trên trang [Bảng giá](/pricing).

## 2. Tạo khóa API

Mở [Khóa API](/keys) và tạo một khóa. Khóa bắt đầu bằng `sk-`. Hãy sao chép và giữ bí mật, vì bất kỳ ai có khóa đều có thể tiêu số dư của bạn. Bạn có thể tạo nhiều khóa, ví dụ mỗi công cụ một khóa, và tắt bất kỳ khóa nào vào bất cứ lúc nào.

## 3. Dùng địa chỉ API

| Loại client | Địa chỉ cần nhập |
|---|---|
| Tương thích OpenAI (OpenAI SDK, Codex CLI, hầu hết ứng dụng chat) | `{{BASE_URL}}/v1` |
| Tương thích Anthropic (Claude Code, Anthropic SDK) | `{{BASE_URL}}` |

Các endpoint được hỗ trợ gồm `/v1/chat/completions`, `/v1/responses`, `/v1/messages`, `/v1/embeddings`, `/v1/images/generations` và `/v1/models`.

## 4. Gửi yêu cầu thử

```bash
curl {{BASE_URL}}/v1/chat/completions \
  -H "Authorization: Bearer sk-khoa-cua-ban" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "stream": true,
    "messages": [{"role": "user", "content": "Xin chào"}]
  }'
```

Thay `gpt-4o-mini` bằng bất kỳ mô hình nào có trên trang [Bảng giá](/pricing). Hãy dùng tên mô hình đúng y như hiển thị ở đó.

## Bước tiếp theo

- [Claude Code](/docs/claude-code)
- [Codex CLI](/docs/codex-cli)
- [Cherry Studio](/docs/cherry-studio)
- [OpenAI SDK](/docs/openai-sdk)
- [Lỗi và giới hạn](/docs/errors-and-limits)
