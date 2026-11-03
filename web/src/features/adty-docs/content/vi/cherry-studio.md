# Cherry Studio

1. Mở **Cài đặt → Nhà cung cấp mô hình**, nhấn **Thêm** ở phía dưới, nhập tên là `ADTY AI` và chọn loại nhà cung cấp là **OpenAI**.
2. Điền:

| Trường | Giá trị |
|---|---|
| Khóa API | `sk-khoa-cua-ban` |
| Địa chỉ API | `{{BASE_URL}}` |

3. Nhấn **Kiểm tra** để thử khóa, sau đó bật nhà cung cấp.
4. Nhấn **Quản lý** (hoặc **Thêm**) bên dưới danh sách mô hình và thêm các mô hình bạn muốn dùng, theo tên trên trang [Bảng giá](/pricing), ví dụ `claude-sonnet-4-5` hoặc `gpt-4o-mini`.
5. Chọn mô hình ở phía trên khung chat và bắt đầu trò chuyện.

## Mẹo

- Nhập địa chỉ API không có `/v1`; Cherry Studio tự thêm `/v1/chat/completions`. Nếu yêu cầu lỗi 404, hãy kiểm tra xem địa chỉ có kết thúc bằng `/v1` không.
- Kiểm tra lỗi 401: khóa sai hoặc đã bị tắt. Kiểm tra lỗi 403: số dư đã hết hoặc khóa không được phép dùng mô hình đó. Xem [Lỗi và giới hạn](/docs/errors-and-limits).
