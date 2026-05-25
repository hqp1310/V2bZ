# HƯỚNG DẪN CẤU HÌNH & KẾT NỐI ZICNODE VỚI ZICBOARD V3

Tài liệu này cung cấp hướng dẫn chi tiết từng bước bằng tiếng Việt để cấu hình kết nối ứng dụng đầu cuối **ZicNode** (chạy trên VPS Backend) với bảng điều khiển **ZicBoard V3** (Panel quản lý đăng ký dịch vụ của bạn).

---

## 1. THÔNG TIN CHUẨN BỊ TỪ TRANG QUẢN TRỊ ZICBOARD (PANEL)

Trước khi tiến hành cài đặt trên VPS Backend, bạn cần đăng nhập vào trang quản trị **ZicBoard** của mình để lấy các thông số xác thực quan trọng sau đây:

### 1.1. Địa chỉ API Host
Là URL dùng để truy cập vào trang quản trị ZicBoard của bạn.
*   *Định dạng:* `https://your-panel-domain.com/`
*   *Lưu ý:* Vui lòng đảm bảo ghi chính xác giao thức `https://` và có dấu `/` ở cuối đường dẫn.

### 1.2. Khóa bảo mật giao tiếp (Server Token / Node Key)
Mã khóa bí mật dùng để xác thực và mã hóa luồng dữ liệu truyền tải giữa ZicNode và ZicBoard.
*   *Cách lấy:* Truy cập trang Admin ZicBoard -> **Cài đặt hệ thống** -> **Cài đặt Node**.
*   *Tên biến trong cơ sở dữ liệu:* Được lưu tự động trong file `/config/zicboard.php` dưới khóa `'server_token'`.

### 1.3. Mã số nhận dạng Node (Node ID)
Bạn cần tạo mới một bản ghi Node trên giao diện quản trị để cấp phát ID:
*   *Cách tạo:* Truy cập Admin ZicBoard -> **Quản lý Node** -> **Thêm Node**.
*   *Loại Node (Type):* Chọn giao thức kết nối là `zicnode`.
*   *Nhận ID:* Sau khi lưu cấu hình, hệ thống sẽ cấp một ID dạng số nguyên cho Node của bạn (ví dụ: `1`, `2`, `3`...). Vui lòng ghi nhớ ID này.

---

## 2. TIẾN HÀNH CÀI ĐẶT & THIẾT LẬP TRÊN VPS BACKEND

Hãy truy cập vào VPS Backend của bạn bằng quyền SSH (người dùng `root`) và lựa chọn một trong các phương thức cài đặt tự động bên dưới:

### Phương pháp A: Cài đặt nhanh kèm tham số cấu hình (Khuyên Dùng)

Phương pháp này tự động thực hiện tải xuống, giải nén và cấu hình ZicNode chỉ trong 1 dòng lệnh mà không yêu cầu nhập thủ công bất kỳ câu hỏi nào:

```bash
wget -N https://raw.githubusercontent.com/kutycma/zicnode/main/script/install.sh && bash install.sh --api-host <Địa_chỉ_API_ZicBoard> --node-id <ID_Node> --api-key <Mã_Server_Token>
```

*Ví dụ thực tế:*
```bash
wget -N https://raw.githubusercontent.com/kutycma/zicnode/main/script/install.sh && bash install.sh --api-host https://panel.example.com/ --node-id 1 --api-key mysecrettoken123
```

---

### Phương pháp B: Cài đặt tương tác từng bước

Nếu bạn chạy lệnh cài đặt tiêu chuẩn:
```bash
wget -N https://raw.githubusercontent.com/kutycma/zicnode/main/script/install.sh && bash install.sh
```

Hệ thống sẽ tải phiên bản mới nhất và đưa ra các câu hỏi tương tác bằng tiếng Việt sau đây (đã được dịch toàn bộ từ tiếng Trung):

1.  **Hỏi:** *Phát hiện đây là lần đầu tiên bạn cài đặt zicnode, bạn có muốn tự động tạo tệp cấu hình /etc/zicnode/config.json không? (y/n):* 
    *   👉 Gõ **`y`** và nhấn **Enter**.
2.  **Hỏi:** *Địa chỉ API của Panel [Định dạng: https://example.com/]:*
    *   👉 Nhập địa chỉ tên miền Panel của bạn (ví dụ: `https://panel.example.com/`) rồi nhấn **Enter**.
3.  **Hỏi:** *ID của Node:*
    *   👉 Nhập ID số nguyên của Node (ví dụ: `1`) rồi nhấn **Enter**.
4.  **Hỏi:** *Mã bảo mật kết nối Node (Server Token):*
    *   👉 Dán khóa **Server Token** của bạn vào và nhấn **Enter**.

---

## 3. LỆNH QUẢN TRỊ NHANH TRÊN VPS

Sau khi hoàn tất cài đặt, script sẽ tự động cấu hình dịch vụ chạy ngầm của hệ thống (`zicnode.service` thông qua `systemd`). Bạn có thể sử dụng các lệnh trực quan sau để điều khiển và quản trị Node:

*   **Menu quản trị đầy đủ tính năng:**
    ```bash
    zicnode
    ```
    *Gõ `zicnode` để mở Menu đồ họa trực quan tiếng Việt hỗ trợ sửa cấu hình, bật/tắt tự khởi động, mở toàn bộ cổng VPS, gỡ cài đặt v.v.*

*   **Khởi động dịch vụ:** `zicnode start`
*   **Dừng dịch vụ:** `zicnode stop`
*   **Khởi động lại dịch vụ:** `zicnode restart`
*   **Xem trạng thái chi tiết:** `zicnode status`
*   **Xem logs lỗi thời gian thực:** `zicnode log`
*   **Mở toàn bộ cổng mạng (Tường lửa):** `zicnode open_ports` (Hữu ích khi bạn bị chặn kết nối từ bên ngoài).

---

## 4. QUY TRÌNH HOẠT ĐỘNG VÀ KHẮC PHỤC SỰ CỐ (TROUBLESHOOTING)

### 4.1. Quy trình kết nối diễn ra như thế nào?
1.  **Handshake Cấu Hình:** ZicNode khởi động -> Gửi yêu cầu GET đến `/api/v3/server/config` để lấy cấu hình (IP, Cổng, Giao thức, Chứng chỉ TLS).
2.  **Kéo Danh Sách User:** ZicNode định kỳ gọi GET đến `/api/v3/server/UniProxy/user` để tải danh sách các mã UUID người dùng khả dụng.
3.  **Đẩy Dữ Liệu:** Định kỳ, ZicNode đẩy thống kê lưu lượng tiêu dùng của khách hàng qua POST đến `/api/v3/server/UniProxy/push` và đẩy IP đang hoạt động qua POST đến `/api/v3/server/UniProxy/alive`.

### 4.2. Các lỗi kết nối thường gặp
*   **Lỗi `missing base_config from ZicBoard` hoặc `token is error`:**
    *   👉 *Nguyên nhân:* Token kết nối bạn nhập trên VPS không khớp với Server Token trong cài đặt Admin ZicBoard.
    *   👉 *Cách sửa:* Chạy lệnh `zicnode` -> chọn phím `0` để sửa cấu hình, kiểm tra kỹ xem Token đã chính xác chưa, sau đó lưu lại để hệ thống tự khởi động lại.
*   **Lỗi `unsupport protocol: hysteria`:**
    *   👉 *Nguyên nhân:* zicnode chỉ hỗ trợ giao thức hiện đại `hysteria2`. Trong phần thêm node trên Admin Panel của bạn, bạn đã chọn nhầm giao thức phiên bản cũ `hysteria` (v1).
    *   👉 *Cách sửa:* Sửa lại giao thức của Node trên Admin Panel thành `hysteria2` hoặc `vless`/`vmess`/`trojan` rồi restart lại node trên VPS.
*   **Không kết nối được, Logs báo lỗi Timeout:**
    *   👉 *Nguyên nhân:* Tường lửa của VPS chặn cổng giao tiếp hoặc Panel của bạn chặn kết nối đến.
    *   👉 *Cách sửa:* Chạy lệnh `zicnode` -> chọn chức năng `14` để tự động dọn sạch quy tắc tường lửa và mở các cổng trên VPS.
