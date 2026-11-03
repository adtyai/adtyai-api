# Codex CLI

Codex CLI đọc danh sách nhà cung cấp từ `~/.codex/config.toml` (trên Windows là `%USERPROFILE%\.codex\config.toml`). Hãy thêm ADTY AI làm nhà cung cấp và đặt nó làm mặc định.

## 1. Sửa config.toml

```toml
model = "gpt-5-codex"
model_provider = "adtyai"

[model_providers.adtyai]
name = "ADTY AI"
base_url = "{{BASE_URL}}/v1"
env_key = "ADTYAI_API_KEY"
wire_api = "responses"
```

- `model_provider` phải trùng với tên đứng sau `model_providers.`.
- `base_url` kết thúc bằng `/v1`.
- Đặt `model` là một mô hình có trên trang [Bảng giá](/pricing).

## 2. Đặt khóa

macOS và Linux:

```bash
export ADTYAI_API_KEY="sk-khoa-cua-ban"
codex
```

Windows PowerShell:

```powershell
$env:ADTYAI_API_KEY = "sk-khoa-cua-ban"
codex
```

Tên biến là tên bạn ghi trong `env_key`; bản thân khóa không bao giờ được ghi vào `config.toml`.

## Mẹo

- Nếu Codex cứ yêu cầu đăng nhập bằng ChatGPT, hãy kiểm tra xem `model_provider` có nằm ở cấp cao nhất của tệp, trước mọi dòng `[section]` hay không. Trong TOML, các khóa viết sau một tiêu đề section thuộc về section đó.
- Để giữ nguyên cấu hình hiện có, hãy đặt nhà cung cấp trong một profile và khởi động Codex bằng `codex --profile adtyai`:

```toml
[profiles.adtyai]
model = "gpt-5-codex"
model_provider = "adtyai"
```
