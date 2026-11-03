# Lỗi và giới hạn

Lỗi dùng định dạng của OpenAI: một mã trạng thái HTTP kèm phần thân JSON có `error.message` và `error.code`. Thông báo cho biết lỗi là gì; mã trạng thái cho biết bạn nên làm gì.

## Mã trạng thái

| Mã | Ý nghĩa | Cách xử lý |
|---|---|---|
| 400 | Yêu cầu không hợp lệ, ví dụ phần thân sai định dạng hoặc có tham số mà mô hình không chấp nhận | Sửa yêu cầu; gửi lại đúng yêu cầu đó sẽ vẫn lỗi |
| 401 | Khóa API bị thiếu, sai, đã tắt, đã hết hạn hoặc đã dùng hết hạn mức riêng của khóa | Kiểm tra header `Authorization: Bearer sk-...` và trạng thái của khóa trên trang [Khóa API](/keys) |
| 402 | Yêu cầu thanh toán. ADTY AI báo số dư không đủ bằng mã 403 (bên dưới), nên mã 402 thường đến từ nhà cung cấp upstream | Thử lại sau; liên hệ hỗ trợ nếu lỗi vẫn tiếp diễn |
| 403 | Số dư không đủ (`insufficient_user_quota`), hoặc khóa không được phép dùng mô hình hay nhóm này | [Nạp tiền](/wallet), hoặc kiểm tra cài đặt mô hình và nhóm của khóa |
| 404 | Sai đường dẫn, hoặc mô hình không tồn tại | Kiểm tra địa chỉ (có hay không có `/v1`, xem hướng dẫn cho từng client) và tên mô hình trên trang [Bảng giá](/pricing) |
| 429 | Tài khoản gửi quá nhiều yêu cầu (`rate_limit_exceeded`), hoặc upstream đang bận | Chờ rồi thử lại; xem các giới hạn bên dưới |
| 500, 502, 503 | Nhà cung cấp upstream gặp lỗi hoặc không có kênh nào khả dụng cho mô hình | Thử lại với thời gian chờ tăng dần; liên hệ hỗ trợ nếu lỗi vẫn tiếp diễn |
| 524 | Yêu cầu mất quá nhiều thời gian và kết nối bị đóng (xem bên dưới) | Dùng streaming |

Các yêu cầu bị từ chối do lỗi không bị tính phí.

## Giới hạn theo tài khoản

Một tài khoản có thể có ba giới hạn, được tính chung cho tất cả các khóa của tài khoản:

- **RPM**: số yêu cầu mỗi phút.
- **TPM**: số token mỗi phút.
- **Đồng thời**: số yêu cầu đang chạy cùng lúc.

Vượt quá bất kỳ giới hạn nào sẽ trả về 429 kèm header `Retry-After` cho biết cần chờ bao nhiêu giây. Hãy thử lại sau khoảng thời gian đó thay vì thử lại ngay. Nếu khối lượng công việc của bạn cần giới hạn cao hơn, hãy liên hệ hỗ trợ.

## Dùng streaming cho các yêu cầu dài

Lưu lượng đi qua Cloudflare, và Cloudflare đóng kết nối nếu không có dữ liệu nào được gửi trong khoảng **125 giây**. Một yêu cầu không dùng streaming mà mất nhiều thời gian hơn thế (đầu ra dài, mô hình suy luận, chỉnh sửa mã lớn) sẽ bị cắt ngang và client của bạn nhận lỗi, thường là 524, dù mô hình vẫn đang xử lý.

Hãy đặt `"stream": true`. Phản hồi streaming bắt đầu gửi dữ liệu ngay và giữ kết nối cho đến khi mô hình hoàn tất. Claude Code, Codex CLI và hầu hết các client chat đều dùng streaming theo mặc định.
