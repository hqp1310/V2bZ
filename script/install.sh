#!/bin/bash
set -o pipefail

red='\033[0;31m'
green='\033[0;32m'
yellow='\033[0;33m'
plain='\033[0m'

cur_dir=$(pwd)
repo="kutycma/zicnode"
script_repo="hqp1310/zicnode"

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

########################
# Phân tích tham số
########################
VERSION_ARG=""
API_HOST_ARG=""
NODE_ID_ARG=""
API_KEY_ARG=""

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --api-host)
                API_HOST_ARG="$2"; shift 2 ;;
            --node-id)
                NODE_ID_ARG="$2"; shift 2 ;;
            --api-key)
                API_KEY_ARG="$2"; shift 2 ;;
            -h|--help)
                echo "Cách dùng: $0 [Phiên bản] [--api-host URL] [--node-id ID] [--api-key KEY]"
                exit 0 ;;
            --*)
                echo "Tham số không xác định: $1"; exit 1 ;;
            *)
                # Tương thích tham số vị trí đầu tiên làm số phiên bản
                if [[ -z "$VERSION_ARG" ]]; then
                    VERSION_ARG="$1"; shift
                else
                    shift
                fi ;;
        esac
    done
}

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

install_base() {
    # Phiên bản tối ưu: Kiểm tra và cài đặt gói hàng loạt, giảm cuộc gọi hệ thống
    need_install_apt() {
        local packages=("$@")
        local missing=()
        
        # Kiểm tra hàng loạt các gói đã cài đặt
        local installed_list=$(dpkg-query -W -f='${Package}\n' 2>/dev/null | sort)
        
        for p in "${packages[@]}"; do
            if ! echo "$installed_list" | grep -q "^${p}$"; then
                missing+=("$p")
            fi
        done
        
        if [[ ${#missing[@]} -gt 0 ]]; then
            echo "Cài đặt các gói còn thiếu: ${missing[*]}"
            apt-get update -y >/dev/null 2>&1
            DEBIAN_FRONTEND=noninteractive apt-get install -y "${missing[@]}" >/dev/null 2>&1
        fi
    }

    need_install_yum() {
        local packages=("$@")
        local missing=()
        
        # Kiểm tra hàng loạt các gói đã cài đặt
        local installed_list=$(rpm -qa --qf '%{NAME}\n' 2>/dev/null | sort)
        
        for p in "${packages[@]}"; do
            if ! echo "$installed_list" | grep -q "^${p}$"; then
                missing+=("$p")
            fi
        done
        
        if [[ ${#missing[@]} -gt 0 ]]; then
            echo "Cài đặt các gói còn thiếu: ${missing[*]}"
            yum install -y "${missing[@]}" >/dev/null 2>&1
        fi
    }

    need_install_apk() {
        local packages=("$@")
        local missing=()
        
        # Kiểm tra hàng loạt các gói đã cài đặt
        local installed_list=$(apk info 2>/dev/null | sort)
        
        for p in "${packages[@]}"; do
            if ! echo "$installed_list" | grep -q "^${p}$"; then
                missing+=("$p")
            fi
        done
        
        if [[ ${#missing[@]} -gt 0 ]]; then
            echo "Cài đặt các gói còn thiếu: ${missing[*]}"
            apk add --no-cache "${missing[@]}" >/dev/null 2>&1
        fi
    }

    # Cài đặt tất cả các gói bắt buộc cùng một lúc
    if [[ x"${release}" == x"centos" ]]; then
        # Kiểm tra và cài đặt epel-release
        if ! rpm -q epel-release >/dev/null 2>&1; then
            echo "Đang cài đặt kho EPEL..."
            yum install -y epel-release >/dev/null 2>&1
        fi
        need_install_yum wget curl unzip tar cronie socat ca-certificates pv
        update-ca-trust force-enable >/dev/null 2>&1 || true
    elif [[ x"${release}" == x"alpine" ]]; then
        need_install_apk wget curl unzip tar socat ca-certificates pv
        update-ca-certificates >/dev/null 2>&1 || true
    elif [[ x"${release}" == x"debian" ]]; then
        need_install_apt wget curl unzip tar cron socat ca-certificates pv
        update-ca-certificates >/dev/null 2>&1 || true
    elif [[ x"${release}" == x"ubuntu" ]]; then
        need_install_apt wget curl unzip tar cron socat ca-certificates pv
        update-ca-certificates >/dev/null 2>&1 || true
    elif [[ x"${release}" == x"arch" ]]; then
        echo "Đang cập nhật cơ sở dữ liệu gói..."
        pacman -Sy --noconfirm >/dev/null 2>&1
        # --needed sẽ bỏ qua các gói đã được cài đặt, rất hiệu quả
        echo "Đang cài đặt các gói bắt buộc..."
        pacman -S --noconfirm --needed wget curl unzip tar cronie socat ca-certificates pv >/dev/null 2>&1
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
            echo -e "${red}zicnode có thể đã khởi động thất bại, vui lòng sử dụng lệnh 'zicnode log' để kiểm tra nhật ký lỗi${plain}"
        fi
}

generate_multi_node() {
    local api_host="$1"
    local node_ids_str="$2"
    local api_key="$3"
    
    node_ids_str="${node_ids_str//,/ }"
    
    local first=true
    local count=2
    
    for nid in $node_ids_str; do
        if [[ $first == true ]]; then
            generate_zicnode_config "$api_host" "$nid" "$api_key"
            first=false
        else
            echo -e "${green}Đang tạo thêm Node ID: $nid...${plain}"
            CONF_DIR="/etc/zicnode"
            EXT="json"
            NEW_CONF="$CONF_DIR/config${count}.${EXT}"
            
            cp "$CONF_DIR/config.${EXT}" "$NEW_CONF"
            sed -i -E "s/\"NodeID\":[[:space:]]*[0-9]+/\"NodeID\": ${nid}/gi" "$NEW_CONF"
            sed -i -E "s/\"node_id\":[[:space:]]*[0-9]+/\"node_id\": ${nid}/gi" "$NEW_CONF"
            
            if [[ x"${release}" == x"alpine" ]]; then
                NEW_SVC="/etc/init.d/zicnode${count}"
                cp /etc/init.d/zicnode "$NEW_SVC"
                sed -i "s/name=\"zicnode\"/name=\"zicnode${count}\"/g" "$NEW_SVC"
                sed -i "s/config\.${EXT}/config${count}\.${EXT}/g" "$NEW_SVC"
                sed -i "s/zicnode\.pid/zicnode${count}\.pid/g" "$NEW_SVC"
                chmod +x "$NEW_SVC"
                rc-update add "zicnode${count}" default
                service "zicnode${count}" start
            else
                NEW_SVC="/etc/systemd/system/zicnode${count}.service"
                cp /etc/systemd/system/zicnode.service "$NEW_SVC"
                sed -i "s/config\.${EXT}/config${count}\.${EXT}/g" "$NEW_SVC"
                sed -i "s/Description=.*/Description=ZicNode ${count} Service/g" "$NEW_SVC"
                systemctl daemon-reload
                systemctl enable "zicnode${count}" >/dev/null 2>&1
                systemctl start "zicnode${count}"
            fi
            echo -e "${green}Khởi chạy thành công zicnode${count} cho Node ID: ${nid}${plain}"
            ((count++))
        fi
    done
    echo -e "${green}Đã hoàn tất thiết lập tất cả các Node!${plain}"
}

install_zicnode() {
    local version_param="$1"
    if [[ -e /usr/local/zicnode/ ]]; then
        rm -rf /usr/local/zicnode/
    fi

    mkdir /usr/local/zicnode/ -p
    cd /usr/local/zicnode/

    if  [[ -z "$version_param" ]] ; then
        last_version=$(curl -Ls "https://api.github.com/repos/${repo}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
        if [[ ! -n "$last_version" ]]; then
            echo -e "${red}Phát hiện phiên bản zicnode thất bại: repo ${repo} chưa có GitHub Release mới nhất hoặc GitHub API đang giới hạn. Vui lòng tạo release hoặc chỉ định phiên bản cài đặt thủ công.${plain}"
            exit 1
        fi
        echo -e "${green}Phát hiện phiên bản mới nhất: ${last_version}, bắt đầu cài đặt...${plain}"
        url="https://github.com/${repo}/releases/download/${last_version}/zicnode-linux-${arch}.zip"
        curl -fsL "$url" | pv -s 30M -W -N "Tiến trình tải" > /usr/local/zicnode/zicnode-linux.zip
        if [[ $? -ne 0 ]]; then
            echo -e "${red}Tải xuống zicnode thất bại, vui lòng đảm bảo máy chủ của bạn có thể tải xuống tệp tin từ GitHub${plain}"
            exit 1
        fi
    else
    last_version=$version_param
        url="https://github.com/${repo}/releases/download/${last_version}/zicnode-linux-${arch}.zip"
        curl -fsL "$url" | pv -s 30M -W -N "Tiến trình tải" > /usr/local/zicnode/zicnode-linux.zip
        if [[ $? -ne 0 ]]; then
            echo -e "${red}Tải xuống zicnode phiên bản $1 thất bại, vui lòng đảm bảo phiên bản này tồn tại${plain}"
            exit 1
        fi
    fi

    unzip zicnode-linux.zip
    rm zicnode-linux.zip -f
    chmod +x zicnode
    mkdir /etc/zicnode/ -p
    cp geoip.dat /etc/zicnode/
    cp geosite.dat /etc/zicnode/
    if [[ x"${release}" == x"alpine" ]]; then
        rm /etc/init.d/zicnode -f
        cat <<EOF > /etc/init.d/zicnode
#!/sbin/openrc-run

name="zicnode"
description="zicnode"

command="/usr/local/zicnode/zicnode"
command_args="server"
command_user="root"

pidfile="/run/zicnode.pid"
command_background="yes"

depend() {
        need net
}
EOF
        chmod +x /etc/init.d/zicnode
        rc-update add zicnode default
        echo -e "${green}zicnode ${last_version}${plain} đã cài đặt hoàn tất, đã thiết lập tự khởi động cùng hệ thống"
    else
        rm /etc/systemd/system/zicnode.service -f
        cat <<EOF > /etc/systemd/system/zicnode.service
[Unit]
Description=zicnode Service
After=network.target nss-lookup.target
Wants=network.target
StartLimitIntervalSec=0

[Service]
User=root
Group=root
Type=simple
LimitAS=infinity
LimitRSS=infinity
LimitCORE=infinity
LimitNOFILE=999999
WorkingDirectory=/usr/local/zicnode/
ExecStart=/usr/local/zicnode/zicnode server --config /etc/zicnode/config.json
Restart=always
RestartSec=3
TimeoutStopSec=30

[Install]
WantedBy=multi-user.target
EOF
        systemctl daemon-reload
        systemctl stop zicnode
        systemctl enable zicnode
        echo -e "${green}zicnode ${last_version}${plain} đã cài đặt hoàn tất, đã thiết lập tự khởi động cùng hệ thống"
    fi

    if [[ ! -f /etc/zicnode/config.json ]]; then
        # Nếu các tham số đầy đủ được truyền qua CLI, cấu hình sẽ được tạo trực tiếp và bỏ qua tương tác
        if [[ -n "$API_HOST_ARG" && -n "$NODE_ID_ARG" && -n "$API_KEY_ARG" ]]; then
            generate_multi_node "$API_HOST_ARG" "$NODE_ID_ARG" "$API_KEY_ARG"
            echo -e "${green}Đã tạo tệp /etc/zicnode/config.json dựa trên các tham số được cung cấp${plain}"
            first_install=false
        else
            cp config.json /etc/zicnode/
            first_install=true
        fi
    else
        if [[ x"${release}" == x"alpine" ]]; then
            service zicnode start
        else
            systemctl start zicnode
        fi
        sleep 2
        check_status
        echo -e ""
        if [[ $? == 0 ]]; then
            echo -e "${green}zicnode khởi động lại thành công${plain}"
        else
            echo -e "${red}zicnode có thể đã khởi động thất bại, vui lòng dùng 'zicnode log' để xem chi tiết nhật ký lỗi${plain}"
        fi
        first_install=false
    fi


    curl -o /usr/bin/zicnode -Ls https://raw.githubusercontent.com/${script_repo}/main/script/zicnode.sh
    chmod +x /usr/bin/zicnode

    cd $cur_dir
    rm -f install.sh
    echo "------------------------------------------"
    echo -e "Cách sử dụng Script quản trị: "
    echo "------------------------------------------"
    echo "zicnode              - Hiển thị Menu quản trị (nhiều tính năng)"
    echo "zicnode start        - Khởi động zicnode"
    echo "zicnode stop         - Dừng zicnode"
    echo "zicnode restart      - Khởi động lại zicnode"
    echo "zicnode status       - Xem trạng thái zicnode"
    echo "zicnode enable       - Bật tự khởi động zicnode"
    echo "zicnode disable      - Tắt tự khởi động zicnode"
    echo "zicnode log          - Xem nhật ký (logs) zicnode"
    echo "zicnode generate     - Tạo tệp cấu hình zicnode"
    echo "zicnode update       - Cập nhật zicnode"
    echo "zicnode update x.x.x - Cập nhật zicnode phiên bản chỉ định"
    echo "zicnode install      - Cài đặt zicnode"
    echo "zicnode uninstall    - Gỡ cài đặt zicnode"
    echo "zicnode version      - Xem phiên bản zicnode"
    echo "------------------------------------------"

}

parse_args "$@"

if [[ ! -f /etc/zicnode/config.json && (-z "$API_HOST_ARG" || -z "$NODE_ID_ARG" || -z "$API_KEY_ARG") ]]; then
    if [[ -z "$API_HOST_ARG" && -z "$NODE_ID_ARG" && -z "$API_KEY_ARG" ]]; then
        read -rp "Phát hiện đây là lần đầu tiên bạn cài đặt zicnode, bạn có muốn tự động tạo tệp cấu hình không? (y/n): " if_generate
    else
        if_generate="y"
    fi
    
    if [[ "$if_generate" =~ ^[Yy]$ ]]; then
        if [[ -z "$API_HOST_ARG" ]]; then
            read -rp "Địa chỉ API của Panel [Định dạng: https://example.com/]: " API_HOST_ARG
            API_HOST_ARG=${API_HOST_ARG:-https://example.com/}
        fi
        
        if [[ -z "$NODE_ID_ARG" ]]; then
            read -rp "Nhập các ID của Node, cách nhau bằng khoảng trắng hoặc phẩy (VD: 2 3 5): " NODE_ID_ARG
            NODE_ID_ARG=${NODE_ID_ARG:-1}
        fi
        
        if [[ -z "$API_KEY_ARG" ]]; then
            read -rp "Mã bảo mật kết nối Node (Server Token): " API_KEY_ARG
        fi
    fi
fi

echo -e "${green}Bắt đầu cài đặt...${plain}"
install_base
install_zicnode "$VERSION_ARG"
