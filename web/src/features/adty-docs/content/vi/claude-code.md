# Claude Code

Claude Code giao tiếp qua Anthropic Messages API, vì vậy hãy trỏ nó tới địa chỉ **không có** `/v1`.

## Biến môi trường

macOS và Linux (thêm các dòng này vào `~/.zshrc` hoặc `~/.bashrc` để giữ lại):

```bash
export ANTHROPIC_BASE_URL="{{BASE_URL}}"
export ANTHROPIC_AUTH_TOKEN="sk-khoa-cua-ban"
claude
```

Windows PowerShell:

```powershell
$env:ANTHROPIC_BASE_URL = "{{BASE_URL}}"
$env:ANTHROPIC_AUTH_TOKEN = "sk-khoa-cua-ban"
claude
```

## Tệp cài đặt

Thay vì dùng biến môi trường, bạn có thể đặt các giá trị tương tự vào `~/.claude/settings.json`:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "{{BASE_URL}}",
    "ANTHROPIC_AUTH_TOKEN": "sk-khoa-cua-ban"
  }
}
```

## Kiểm tra hoạt động

Chạy `claude`, rồi gõ `/status`. Base URL hiển thị phải là `{{BASE_URL}}`. Gửi một tin nhắn bất kỳ; sau đó yêu cầu sẽ xuất hiện trong nhật ký sử dụng trên bảng điều khiển.

## Mẹo

- Dùng `ANTHROPIC_AUTH_TOKEN`, không dùng `ANTHROPIC_API_KEY`. Nếu `ANTHROPIC_API_KEY` cũng được đặt, Claude Code có thể hỏi bạn muốn dùng khóa nào hoặc tiếp tục gửi khóa chính thức của bạn; hãy bỏ đặt biến đó.
- Nếu trước đây bạn đã đăng nhập bằng tài khoản Claude, hãy chạy `/logout` trước để Claude Code dùng khóa.
- Để chọn mô hình, chạy `/model` trong Claude Code hoặc đặt `ANTHROPIC_MODEL`. Chỉ các mô hình có trên trang [Bảng giá](/pricing) mới dùng được.
- Bạn cũng có thể nhập cài đặt bằng CC Switch từ trang [Khóa API](/keys).
