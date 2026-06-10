#!/bin/bash

red='\033[0;31m'
green='\033[0;32m'
yellow='\033[0;33m'
plain='\033[0m'

cur_dir=$(pwd)
repo="kutycma/zicnode"

# check root
[[ $EUID -ne 0 ]] && echo -e "${red}Lỗi:${plain} Bắt buộc phải sử dụng người dùng root để chạy script này!\n" && exit 1

# check os
if [[ -f /etc/redhat-release ]]; then
    release="centos"
elif cat /etc/issue | grep -Eqi "alpine"; then
    release="alpine"
elif cat /etc/issue | grep -Eqi "debian"; then
    release="debian"
elif cat /etc/issue | grep -Eqi "ubuntu"; then
    release="ubuntu"
elif cat /etc/issue | grep -Eqi "centos|red hat|redhat|rocky|alma|oracle linux"; then
    release="centos"
elif cat /proc/version | grep -Eqi "debian"; then
    release="debian"
elif cat /proc/version | grep -Eqi "ubuntu"; then
    release="ubuntu"
elif cat /proc/version | grep -Eqi "centos|red hat|redhat|rocky|alma|oracle linux"; then
    release="centos"
elif cat /proc/version | grep -Eqi "arch"; then
    release="arch"
else
    echo -e "${red}Không phát hiện được phiên bản hệ thống, vui lòng liên hệ tác giả script!${plain}\n" && exit 1
fi

arch=$(uname -m)

if [[ $arch == "x86_64" || $arch == "x64" || $arch == "amd64" ]]; then
    arch="64"
elif [[ $arch == "aarch64" || $arch == "arm64" ]]; then
    arch="arm64-v8a"
elif [[ $arch == "s390x" ]]; then
    arch="s390x"
else
    arch="64"
    echo -e "${red}Phát hiện kiến trúc thất bại, sử dụng kiến trúc mặc định: ${arch}${plain}"
fi

if [ "$(getconf WORD_BIT)" != '32' ] && [ "$(getconf LONG_BIT)" != '64' ] ; then
    echo "Phần mềm này không hỗ trợ hệ thống 32-bit (x86), vui lòng sử dụng hệ thống 64-bit (x86_64). Nếu phát hiện sai sót, vui lòng liên hệ tác giả."
    exit 2
fi

# os version
if [[ -f /etc/os-release ]]; then
    os_version=$(awk -F'[= ."]' '/VERSION_ID/{print $3}' /etc/os-release)
fi
if [[ -z "$os_version" && -f /etc/lsb-release ]]; then
    os_version=$(awk -F'[= ."]+' '/DISTRIB_RELEASE/{print $2}' /etc/lsb-release)
fi

if [[ x"${release}" == x"centos" ]]; then
    if [[ ${os_version} -le 6 ]]; then
        echo -e "${red}Vui lòng sử dụng hệ thống CentOS 7 hoặc cao hơn!${plain}\n" && exit 1
    fi
    if [[ ${os_version} -eq 7 ]]; then
        echo -e "${red}Lưu ý: CentOS 7 không thể sử dụng giao thức hysteria1/2!${plain}\n"
    fi
elif [[ x"${release}" == x"ubuntu" ]]; then
    if [[ ${os_version} -lt 16 ]]; then
        echo -e "${red}Vui lòng sử dụng hệ thống Ubuntu 16 hoặc cao hơn!${plain}\n" && exit 1
    fi
elif [[ x"${release}" == x"debian" ]]; then
    if [[ ${os_version} -lt 8 ]]; then
        echo -e "${red}Vui lòng sử dụng hệ thống Debian 8 hoặc cao hơn!${plain}\n" && exit 1
    fi
fi

confirm() {
    if [[ $# > 1 ]]; then
        echo && read -rp "$1 [Mặc định $2]: " temp
        if [[ x"${temp}" == x"" ]]; then
            temp=$2
        fi
    else
        read -rp "$1 [y/n]: " temp
    fi
    if [[ x"${temp}" == x"y" || x"${temp}" == x"Y" ]]; then
        return 0
    else
        return 1
    fi
}

confirm_restart() {
    confirm "Bạn có muốn khởi động lại zicnode không?" "y"
    if [[ $? == 0 ]]; then
        restart
    else
        show_menu
    fi
}

before_show_menu() {
    echo && echo -n -e "${yellow}Nhấn Enter để quay lại menu chính: ${plain}" && read temp
    show_menu
}

install() {
    bash <(curl -Ls https://raw.githubusercontent.com/${repo}/main/script/install.sh)
    if [[ $? == 0 ]]; then
        if [[ $# == 0 ]]; then
            start
        else
            start 0
        fi
    fi
}

update() {
    if [[ $# == 0 ]]; then
        echo && echo -n -e "Nhập phiên bản chỉ định (mặc định là mới nhất): " && read version
    else
        version=$2
    fi
    bash <(curl -Ls https://raw.githubusercontent.com/${repo}/main/script/install.sh) $version
    if [[ $? == 0 ]]; then
        echo -e "${green}Cập nhật hoàn tất, đã tự động khởi động lại zicnode, vui lòng dùng lệnh 'zicnode log' để xem nhật ký hoạt động${plain}"
        exit
    fi

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

config() {
    echo "zicnode sẽ tự động thử khởi động lại sau khi sửa đổi cấu hình"
    nano /etc/zicnode/config.json
    sleep 2
    restart
    check_status
    case $? in
        0)
            echo -e "Trạng thái zicnode: ${green}Đang chạy${plain}"
            ;;
        1)
            echo -e "Phát hiện bạn chưa khởi động zicnode hoặc khởi động lại thất bại, bạn có muốn xem logs không? [Y/n]" && echo
            read -e -rp "(Mặc định: y):" yn
            [[ -z ${yn} ]] && yn="y"
            if [[ ${yn} == [Yy] ]]; then
               show_log
            fi
            ;;
        2)
            echo -e "Trạng thái zicnode: ${red}Chưa cài đặt${plain}"
    esac
}

uninstall() {
    confirm "Bạn có chắc chắn muốn gỡ cài đặt zicnode không?" "n"
    if [[ $? != 0 ]]; then
        if [[ $# == 0 ]]; then
            show_menu
        fi
        return 0
    fi
    if [[ x"${release}" == x"alpine" ]]; then
        service zicnode stop
        rc-update del zicnode
        rm /etc/init.d/zicnode -f
    else
        systemctl stop zicnode
        systemctl disable zicnode
        rm /etc/systemd/system/zicnode.service -f
        systemctl daemon-reload
        systemctl reset-failed
    fi
    rm /etc/zicnode/ -rf
    rm /usr/local/zicnode/ -rf

    echo ""
    echo -e "Gỡ cài đặt thành công, nếu muốn xóa script này, vui lòng thoát script rồi chạy lệnh ${green}rm /usr/bin/zicnode -f${plain} để xóa"
    echo ""

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

start() {
    check_status
    if [[ $? == 0 ]]; then
        echo ""
        echo -e "${green}zicnode đã chạy, không cần khởi động lại, nếu muốn khởi động lại vui lòng chọn chức năng khởi động lại${plain}"
    else
        if [[ x"${release}" == x"alpine" ]]; then
            service zicnode start
        else
            systemctl start zicnode
        fi
        sleep 2
        check_status
        if [[ $? == 0 ]]; then
            echo -e "${green}zicnode đã khởi động thành công, vui lòng dùng 'zicnode log' để xem nhật ký hoạt động${plain}"
        else
            echo -e "${red}zicnode có thể đã khởi động thất bại, vui lòng dùng 'zicnode log' để kiểm tra lỗi sau${plain}"
        fi
    fi

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

stop() {
    if [[ x"${release}" == x"alpine" ]]; then
        service zicnode stop
    else
        systemctl stop zicnode
    fi
    sleep 2
    check_status
    if [[ $? == 1 ]]; then
        echo -e "${green}zicnode đã dừng thành công${plain}"
    else
        echo -e "${red}zicnode dừng thất bại, có thể do thời gian dừng vượt quá 2 giây, vui lòng kiểm tra lại logs sau${plain}"
    fi

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

restart() {
    if [[ x"${release}" == x"alpine" ]]; then
        service zicnode restart
    else
        systemctl restart zicnode
    fi
    sleep 2
    check_status
    if [[ $? == 0 ]]; then
        echo -e "${green}zicnode khởi động lại thành công, vui lòng dùng 'zicnode log' để xem nhật ký hoạt động${plain}"
    else
        echo -e "${red}zicnode có thể đã khởi động thất bại, vui lòng dùng 'zicnode log' để kiểm tra lỗi${plain}"
    fi
    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

status() {
    if [[ x"${release}" == x"alpine" ]]; then
        service zicnode status
    else
        systemctl status zicnode --no-pager -l
    fi
    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

enable() {
    if [[ x"${release}" == x"alpine" ]]; then
        rc-update add zicnode
    else
        systemctl enable zicnode
    fi
    if [[ $? == 0 ]]; then
        echo -e "${green}zicnode đã thiết lập tự khởi động cùng hệ thống thành công${plain}"
    else
        echo -e "${red}zicnode thiết lập tự khởi động cùng hệ thống thất bại${plain}"
    fi

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

disable() {
    if [[ x"${release}" == x"alpine" ]]; then
        rc-update del zicnode
    else
        systemctl disable zicnode
    fi
    if [[ $? == 0 ]]; then
        echo -e "${green}zicnode đã hủy tự khởi động cùng hệ thống thành công${plain}"
    else
        echo -e "${red}zicnode hủy tự khởi động cùng hệ thống thất bại${plain}"
    fi

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

show_log() {
    if [[ x"${release}" == x"alpine" ]]; then
        echo -e "${red}Hệ thống Alpine tạm thời chưa hỗ trợ xem logs${plain}\n" && exit 1
    else
        journalctl -u zicnode.service -e --no-pager -f
    fi
    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

json_value_from_file() {
    local file="$1"
    local key="$2"
    if command -v jq >/dev/null 2>&1; then
        jq -r "$key // empty" "$file" 2>/dev/null
    else
        local simple_key
        simple_key=$(echo "$key" | sed -E 's/^\.Nodes\[0\]\.//;s/^\.//')
        grep -m1 "\"${simple_key}\"" "$file" 2>/dev/null | sed -E 's/.*"'"${simple_key}"'"[[:space:]]*:[[:space:]]*"?([^",}]*)"?.*/\1/'
    fi
}

json_value_from_text() {
    local json="$1"
    local key="$2"
    if command -v jq >/dev/null 2>&1; then
        printf '%s' "$json" | jq -r "$key // empty" 2>/dev/null
    else
        local simple_key
        simple_key=$(echo "$key" | sed -E 's/^\.warp_settings\.//;s/^\.//')
        local compact
        compact=$(printf '%s' "$json" | tr -d '\n')
        if echo "$compact" | grep -q "\"${simple_key}\""; then
            echo "$compact" | sed -E 's/.*"'"${simple_key}"'"[[:space:]]*:[[:space:]]*"?([^",}]*)"?.*/\1/'
        fi
    fi
}

json_bool_from_text() {
    local json="$1"
    local key="$2"
    if command -v jq >/dev/null 2>&1; then
        printf '%s' "$json" | jq -r "($key // false) as \$v | if (\$v == true or \$v == 1 or \$v == \"1\" or \$v == \"true\") then \"true\" else \"false\" end" 2>/dev/null
    else
        local compact
        compact=$(printf '%s' "$json" | tr -d '\n ')
        if echo "$compact" | grep -Eq '"warp_settings":\{[^}]*"enable":(true|1|"1"|"true")'; then
            echo "true"
        else
            echo "false"
        fi
    fi
}

print_warp_sidecar_status() {
    local node_id="$1"
    local sidecar_files=(/etc/zicnode/warp/*-zicnode-${node_id}.json)
    if [[ -e "${sidecar_files[0]}" ]]; then
        local sidecar="${sidecar_files[0]}"
        echo -e "Sidecar: ${green}Có${plain}"
        echo "Sidecar path: ${sidecar}"
        if command -v stat >/dev/null 2>&1; then
            echo "Sidecar updated: $(stat -c '%y' "$sidecar" 2>/dev/null | cut -d '.' -f1)"
        fi
        local endpoint addresses
        endpoint=$(json_value_from_file "$sidecar" '.endpoint')
        if command -v jq >/dev/null 2>&1; then
            addresses=$(jq -r '.addresses // [] | join(",")' "$sidecar" 2>/dev/null)
        else
            addresses=$(grep -m1 '"addresses"' "$sidecar" 2>/dev/null | sed -E 's/.*\[([^]]*)\].*/\1/' | tr -d '" ')
        fi
        [[ -n "$endpoint" ]] && echo "Sidecar endpoint: ${endpoint}"
        [[ -n "$addresses" ]] && echo "Sidecar addresses: ${addresses}"
    else
        echo -e "Sidecar: ${yellow}Chưa có${plain}"
    fi
}

print_warp_logs() {
    echo ""
    echo "Log WARP gần nhất:"
    if [[ x"${release}" == x"alpine" ]]; then
        echo -e "${yellow}Alpine chưa hỗ trợ journalctl trong script này, vui lòng xem log service thủ công.${plain}"
        return
    fi
    if ! command -v journalctl >/dev/null 2>&1; then
        echo -e "${yellow}Không tìm thấy journalctl.${plain}"
        return
    fi
    local logs
    logs=$(journalctl -u zicnode.service -n 200 --no-pager 2>/dev/null | grep -iE 'warp|wireguard' | tail -n 30)
    if [[ -n "$logs" ]]; then
        echo "$logs"
    else
        echo -e "${yellow}Không thấy log WARP/WireGuard trong 200 dòng gần nhất.${plain}"
    fi
}

check_cloudflare_warp_api() {
    if ! command -v curl >/dev/null 2>&1; then
        echo -e "Cloudflare API: ${yellow}Bỏ qua vì chưa cài curl${plain}"
        return
    fi
    local http_code
    http_code=$(curl -sS --connect-timeout 5 --max-time 10 -o /dev/null -w "%{http_code}" https://api.cloudflareclient.com/ 2>/dev/null)
    if [[ "$http_code" != "000" && -n "$http_code" ]]; then
        echo -e "Cloudflare API: ${green}Kết nối được${plain} (HTTP ${http_code})"
    else
        echo -e "Cloudflare API: ${red}Không kết nối được${plain}"
    fi
}

print_panel_warp_status() {
    local api_host="$1"
    local node_id="$2"
    local api_key="$3"

    if ! command -v curl >/dev/null 2>&1; then
        echo -e "Panel WARP: ${yellow}Không kiểm tra được vì chưa cài curl${plain}"
        return
    fi
    local url body enabled mode fail_policy mtu endpoint
    url="${api_host%/}/api/v3/server/config?node_type=zicnode&node_id=${node_id}&token=${api_key}"
    body=$(curl -fsS --connect-timeout 8 --max-time 15 "$url" 2>/dev/null)
    if [[ -z "$body" ]]; then
        echo -e "Panel WARP: ${yellow}Không lấy được config từ panel${plain}"
        return
    fi
    enabled=$(json_bool_from_text "$body" '.warp_settings.enable')
    if [[ "$enabled" == "true" ]]; then
        echo -e "Panel WARP: ${green}Bật${plain}"
        mode=$(json_value_from_text "$body" '.warp_settings.mode')
        fail_policy=$(json_value_from_text "$body" '.warp_settings.fail_policy')
        mtu=$(json_value_from_text "$body" '.warp_settings.mtu')
        endpoint=$(json_value_from_text "$body" '.warp_settings.endpoint')
        [[ -n "$mode" ]] && echo "Mode: ${mode}"
        [[ -n "$fail_policy" ]] && echo "Fail Policy: ${fail_policy}"
        [[ -n "$mtu" ]] && echo "MTU: ${mtu}"
        [[ -n "$endpoint" ]] && echo "Endpoint: ${endpoint}"
    else
        echo -e "Panel WARP: ${yellow}Tắt hoặc chưa cấu hình${plain}"
        echo "Ghi chú: WARP được bật/tắt trong ZicBoard, không chỉnh tại VPS."
    fi
}

warp_status_for_node() {
    local api_host="$1"
    local node_id="$2"
    local api_key="$3"

    echo "------------------------------------------"
    echo "Node ID: ${node_id}"
    echo "Panel: ${api_host}"
    print_panel_warp_status "$api_host" "$node_id" "$api_key"
    print_warp_sidecar_status "$node_id"
}

warp_status() {
    check_status
    case $? in
        0)
            echo -e "Trạng thái zicnode: ${green}Đang chạy${plain}"
            ;;
        1)
            echo -e "Trạng thái zicnode: ${yellow}Không chạy${plain}"
            ;;
        2)
            echo -e "Trạng thái zicnode: ${red}Chưa cài đặt${plain}"
            if [[ $# == 0 ]]; then
                before_show_menu
            fi
            return
            ;;
    esac

    local config_file="/etc/zicnode/config.json"
    if [[ ! -f "$config_file" ]]; then
        echo -e "${red}Không tìm thấy ${config_file}${plain}"
        if [[ $# == 0 ]]; then
            before_show_menu
        fi
        return
    fi

    if command -v jq >/dev/null 2>&1; then
        local node_count i api_host node_id api_key
        node_count=$(jq '.Nodes | length' "$config_file" 2>/dev/null)
        [[ -z "$node_count" || "$node_count" == "null" ]] && node_count=0
        if [[ "$node_count" -eq 0 ]]; then
            echo -e "${red}Không tìm thấy Nodes[] trong ${config_file}${plain}"
        fi
        for ((i=0; i<node_count; i++)); do
            api_host=$(jq -r ".Nodes[$i].ApiHost // empty" "$config_file" 2>/dev/null)
            node_id=$(jq -r ".Nodes[$i].NodeID // empty" "$config_file" 2>/dev/null)
            api_key=$(jq -r ".Nodes[$i].ApiKey // empty" "$config_file" 2>/dev/null)
            if [[ -n "$api_host" && -n "$node_id" && -n "$api_key" ]]; then
                warp_status_for_node "$api_host" "$node_id" "$api_key"
            else
                echo -e "${yellow}Bỏ qua node index ${i} vì thiếu ApiHost/NodeID/ApiKey.${plain}"
            fi
        done
    else
        local api_host node_id api_key
        api_host=$(json_value_from_file "$config_file" '.Nodes[0].ApiHost')
        node_id=$(json_value_from_file "$config_file" '.Nodes[0].NodeID')
        api_key=$(json_value_from_file "$config_file" '.Nodes[0].ApiKey')
        if [[ -n "$api_host" && -n "$node_id" && -n "$api_key" ]]; then
            warp_status_for_node "$api_host" "$node_id" "$api_key"
        else
            echo -e "${red}Không đọc được cấu hình node đầu tiên. Cài jq để kiểm tra JSON chính xác hơn.${plain}"
        fi
    fi

    echo "------------------------------------------"
    check_cloudflare_warp_api
    print_warp_logs

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

update_shell() {
    wget -O /usr/bin/zicnode -N --no-check-certificate https://raw.githubusercontent.com/${repo}/main/script/zicnode.sh
    if [[ $? != 0 ]]; then
        echo ""
        echo -e "${red}Tải xuống script thất bại, vui lòng kiểm tra kết nối tới GitHub${plain}"
        before_show_menu
    else
        chmod +x /usr/bin/zicnode
        echo -e "${green}Nâng cấp script thành công, vui lòng chạy lại script${plain}" && exit 0
    fi
}

# 0: running, 1: not running, 2: not installed
check_status() {
    if [[ ! -f /usr/local/zicnode/zicnode ]]; then
        return 2
    fi
    if [[ x"${release}" == x"alpine" ]]; then
        temp=$(service zicnode status | awk '{print $3}')
        if [[ x"${temp}" == x"started" ]]; then
            return 0
        else
            return 1
        fi
    else
        temp=$(systemctl status zicnode | grep Active | awk '{print $3}' | cut -d "(" -f2 | cut -d ")" -f1)
        if [[ x"${temp}" == x"running" ]]; then
            return 0
        else
            return 1
        fi
    fi
}

check_enabled() {
    if [[ x"${release}" == x"alpine" ]]; then
        temp=$(rc-update show | grep zicnode)
        if [[ x"${temp}" == x"" ]]; then
            return 1
        else
            return 0
        fi
    else
        temp=$(systemctl is-enabled zicnode)
        if [[ x"${temp}" == x"enabled" ]]; then
            return 0
        else
            return 1;
        fi
    fi
}

check_uninstall() {
    check_status
    if [[ $? != 2 ]]; then
        echo ""
        echo -e "${red}zicnode đã được cài đặt, vui lòng không cài đặt lại${plain}"
        if [[ $# == 0 ]]; then
            before_show_menu
        fi
        return 1
    else
        return 0
    fi
}

check_install() {
    check_status
    if [[ $? == 2 ]]; then
        echo ""
        echo -e "${red}Vui lòng cài đặt zicnode trước${plain}"
        if [[ $# == 0 ]]; then
            before_show_menu
        fi
        return 1
    else
        return 0
    fi
}

show_status() {
    check_status
    case $? in
        0)
            echo -e "Trạng thái zicnode: ${green}Đang chạy${plain}"
            show_enable_status
            ;;
        1)
            echo -e "Trạng thái zicnode: ${yellow}Không chạy${plain}"
            show_enable_status
            ;;
        2)
            echo -e "Trạng thái zicnode: ${red}Chưa cài đặt${plain}"
    esac
}

show_enable_status() {
    check_enabled
    if [[ $? == 0 ]]; then
        echo -e "Tự khởi động cùng hệ thống: ${green}Có${plain}"
    else
        echo -e "Tự khởi động cùng hệ thống: ${red}Không${plain}"
    fi
}

show_zicnode_version() {
    echo -n "Phiên bản zicnode: "
    /usr/local/zicnode/zicnode version
    echo ""
    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

generate_zicnode_config() {
        local api_host="$1"
        local node_id="$2"
        local api_key="$3"

        mkdir -p /etc/zicnode >/dev/null 2>&1
        cat > /etc/zicnode/config.json <<EOF
{
    "Log": {
        "Level": "error",
        "Output": "",
        "Access": ""
    },
    "Nodes": [
        {
            "ApiHost": "${api_host}",
            "NodeID": ${node_id},
            "ApiKey": "${api_key}",
            "Timeout": 15
        }
    ]
}
EOF
        echo -e "${green}Đã tạo xong tệp cấu hình ZicNode, đang khởi động lại dịch vụ...${plain}"
        if [[ x"${release}" == x"alpine" ]]; then
            service zicnode restart
        else
            systemctl restart zicnode
        fi
        sleep 2
        check_status
        echo -e ""
        if [[ $? == 0 ]]; then
            echo -e "${green}zicnode khởi động lại thành công${plain}"
        else
            echo -e "${red}zicnode có thể đã khởi động thất bại, vui lòng dùng 'zicnode log' để kiểm tra lỗi${plain}"
        fi
}


generate_config_file() {
    # Thu thập các tham số tương tác, cung cấp giá trị mặc định làm ví dụ
    read -rp "Địa chỉ API của Panel [Định dạng: https://example.com/]: " api_host
    api_host=${api_host:-https://example.com/}
    read -rp "ID của Node: " node_id
    node_id=${node_id:-1}
    read -rp "Mã bảo mật kết nối Node (Server Token): " api_key

    # Tạo cấu hình (ghi đè lên mẫu có thể đã được sao chép từ gói)
    generate_zicnode_config "$api_host" "$node_id" "$api_key"
}

# Mở cổng tường lửa
open_ports() {
    systemctl stop firewalld.service 2>/dev/null
    systemctl disable firewalld.service 2>/dev/null
    setenforce 0 2>/dev/null
    ufw disable 2>/dev/null
    iptables -P INPUT ACCEPT 2>/dev/null
    iptables -P FORWARD ACCEPT 2>/dev/null
    iptables -P OUTPUT ACCEPT 2>/dev/null
    iptables -t nat -F 2>/dev/null
    iptables -t mangle -F 2>/dev/null
    iptables -F 2>/dev/null
    iptables -X 2>/dev/null
    netfilter-persistent save 2>/dev/null
    echo -e "${green}Đã mở tất cả các cổng tường lửa thành công!${plain}"
}

show_usage() {
    echo "Cách sử dụng Script quản trị zicnode: "
    echo "------------------------------------------"
    echo "zicnode              - Hiển thị Menu quản trị (nhiều tính năng)"
    echo "zicnode start        - Khởi động zicnode"
    echo "zicnode stop         - Dừng zicnode"
    echo "zicnode restart      - Khởi động lại zicnode"
    echo "zicnode status       - Xem trạng thái zicnode"
    echo "zicnode enable       - Bật tự khởi động zicnode"
    echo "zicnode disable      - Tắt tự khởi động zicnode"
    echo "zicnode log          - Xem nhật ký (logs) zicnode"
    echo "zicnode x25519       - Tạo khóa x25519"
    echo "zicnode generate     - Tạo tệp cấu hình zicnode"
    echo "zicnode update       - Cập nhật zicnode"
    echo "zicnode update x.x.x - Cài đặt zicnode phiên bản chỉ định"
    echo "zicnode install      - Cài đặt zicnode"
    echo "zicnode uninstall    - Gỡ cài đặt zicnode"
    echo "zicnode version      - Xem phiên bản zicnode"
    echo "zicnode warp         - Kiểm tra trạng thái WARP"
    echo "zicnode warp_status  - Kiểm tra trạng thái WARP"
    echo "------------------------------------------"
}

show_menu() {
    echo -e "
  ${green}Script quản trị đầu cuối ZicNode,${plain} ${red}không áp dụng cho docker${plain}
--- https://github.com/kutycma/zicnode ---
  ${green}0.${plain} Sửa đổi cấu hình (config.json)
——————————————
  ${green}1.${plain} Cài đặt zicnode
  ${green}2.${plain} Cập nhật zicnode
  ${green}3.${plain} Gỡ cài đặt zicnode
——————————————
  ${green}4.${plain} Khởi động zicnode
  ${green}5.${plain} Dừng zicnode
  ${green}6.${plain} Khởi động lại zicnode
  ${green}7.${plain} Xem trạng thái zicnode
  ${green}8.${plain} Xem nhật ký (logs) zicnode
——————————————
  ${green}9.${plain} Bật tự khởi động zicnode cùng hệ thống
  ${green}10.${plain} Tắt tự khởi động zicnode cùng hệ thống
——————————————
  ${green}11.${plain} Xem phiên bản zicnode
  ${green}12.${plain} Nâng cấp script bảo trì zicnode
  ${green}13.${plain} Tạo tệp cấu hình zicnode
  ${green}14.${plain} Mở tất cả các cổng mạng của VPS
  ${green}15.${plain} Kiểm tra trạng thái WARP
  ${green}16.${plain} Thoát script
 "
    show_status
    echo && read -rp "Vui lòng chọn [0-16]: " num

    case "${num}" in
        0) config ;;
        1) check_uninstall && install ;;
        2) check_install && update ;;
        3) check_install && uninstall ;;
        4) check_install && start ;;
        5) check_install && stop ;;
        6) check_install && restart ;;
        7) check_install && status ;;
        8) check_install && show_log ;;
        9) check_install && enable ;;
        10) check_install && disable ;;
        11) check_install && show_zicnode_version ;;
        12) update_shell ;;
        13) generate_config_file ;;
        14) open_ports ;;
        15) check_install && warp_status ;;
        16) exit ;;
        *) echo -e "${red}Vui lòng nhập số chính xác [0-16]${plain}" ;;
    esac
}


if [[ $# > 0 ]]; then
    case $1 in
        "start") check_install 0 && start 0 ;;
        "stop") check_install 0 && stop 0 ;;
        "restart") check_install 0 && restart 0 ;;
        "status") check_install 0 && status 0 ;;
        "enable") check_install 0 && enable 0 ;;
        "disable") check_install 0 && disable 0 ;;
        "log") check_install 0 && show_log 0 ;;
        "update") check_install 0 && update 0 $2 ;;
        "config") config $* ;;
        "generate") generate_config_file ;;
        "install") check_uninstall 0 && install 0 ;;
        "uninstall") check_install 0 && uninstall 0 ;;
        "version") check_install 0 && show_zicnode_version 0 ;;
        "warp") check_install 0 && warp_status 0 ;;
        "warp_status") check_install 0 && warp_status 0 ;;
        "update_shell") update_shell ;;
        *) show_usage
    esac
else
    show_menu
fi
