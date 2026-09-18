#!/bin/bash

red='\033[0;31m'
green='\033[0;32m'
blue='\033[0;34m'
yellow='\033[0;33m'
plain='\033[0m'
pink='\033[38;5;213m'

# Unified project source. Panel binary, installer, service and menu all come
# from this repository. Override with XUI_REPO/XUI_BRANCH only for development.
XUI_REPO="${XUI_REPO:-shini74744/Dui}"
XUI_BRANCH="${XUI_BRANCH:-main}"
XUI_RAW_BASE="https://raw.githubusercontent.com/${XUI_REPO}/${XUI_BRANCH}"
XUI_RELEASE_BASE="https://github.com/${XUI_REPO}/releases"

dui_latest_version() {
    local final_url tag
    final_url=$(curl -fsSL --retry 2 --connect-timeout 10 --max-time 20 -o /dev/null -w '%{url_effective}' "${XUI_RELEASE_BASE}/latest" 2>/dev/null || true)
    tag="${final_url##*/}"
    if [[ "$tag" == v* && "$tag" != "latest" ]]; then
        printf '%s\n' "$tag"
        return 0
    fi
    return 1
}

dui_arch() {
    case "$(uname -m)" in
        x86_64 | x64 | amd64) echo 'amd64' ;;
        i*86 | x86) echo '386' ;;
        armv8* | armv8 | arm64 | aarch64) echo 'arm64' ;;
        armv7* | armv7 | arm) echo 'armv7' ;;
        armv6* | armv6) echo 'armv6' ;;
        armv5* | armv5) echo 'armv5' ;;
        s390x) echo 's390x' ;;
        *) return 1 ;;
    esac
}

#Add some basic function here
function LOGD() {
    echo -e "${yellow}[DEG] $* ${plain}"
}

function LOGE() {
    echo -e "${red}[ERR] $* ${plain}"
}

function LOGI() {
    echo -e "${green}[INF] $* ${plain}"
}

# check root
[[ $EUID -ne 0 ]] && echo -e "${red}致命错误: ${plain} 请使用 root 权限运行此脚本\n" && exit 1

# Check OS and set release variable
if [[ -f /etc/os-release ]]; then
    source /etc/os-release
    release=$ID
elif [[ -f /usr/lib/os-release ]]; then
    source /usr/lib/os-release
    release=$ID
else
    echo -e "${red}检查服务器操作系统失败，请联系作者!${plain}" >&2
    exit 1
fi

echo -e "——————————————————————"
echo -e "当前服务器的操作系统为:${red} $release${plain}"
echo ""
xui_version=$(/usr/local/x-ui/x-ui -v)
last_version=$(dui_latest_version || true)
echo -e "${green}当前 Dui 版本：${red}v${xui_version}${plain}"
echo ""
echo -e "${yellow}GitHub 最新 Release 版本：${red}${last_version}${plain}"

os_version=$(grep -i version_id /etc/os-release | cut -d \" -f2 | cut -d . -f1)

if [[ "${release}" == "centos" ]]; then
    if [[ ${os_version} -lt 8 ]]; then
        echo -e "${red} 请使用 CentOS 8 或更高版本 ${plain}\n" && exit 1
    fi
elif [[ "${release}" == "ubuntu" ]]; then
    if [[ ${os_version} -lt 20 ]]; then
        echo -e "${red} 请使用 Ubuntu 20 或更高版本!${plain}\n" && exit 1
    fi

elif [[ "${release}" == "fedora" ]]; then
    if [[ ${os_version} -lt 36 ]]; then
        echo -e "${red} 请使用 Fedora 36 或更高版本!${plain}\n" && exit 1
    fi

elif [[ "${release}" == "debian" ]]; then
    if [[ ${os_version} -lt 11 ]]; then
        echo -e "${red} 请使用 Debian 11 或更高版本 ${plain}\n" && exit 1
    fi

elif [[ "${release}" == "almalinux" ]]; then
    if [[ ${os_version} -lt 9 ]]; then
        echo -e "${red} 请使用 AlmaLinux 9 或更高版本 ${plain}\n" && exit 1
    fi

elif [[ "${release}" == "rocky" ]]; then
    if [[ ${os_version} -lt 9 ]]; then
        echo -e "${red} 请使用 RockyLinux 9 或更高版本 ${plain}\n" && exit 1
    fi
elif [[ "${release}" == "arch" ]]; then
    echo "您的操作系统是 ArchLinux"
elif [[ "${release}" == "manjaro" ]]; then
    echo "您的操作系统是 Manjaro"
elif [[ "${release}" == "armbian" ]]; then
    echo "您的操作系统是 Armbian"
elif [[ "${release}" == "alpine" ]]; then
    echo "您的操作系统是 Alpine Linux"
elif [[ "${release}" == "opensuse-tumbleweed" ]]; then
    echo "您的操作系统是 OpenSUSE Tumbleweed"
elif [[ "${release}" == "oracle" ]]; then
    if [[ ${os_version} -lt 8 ]]; then
        echo -e "${red} 请使用 Oracle Linux 8 或更高版本 ${plain}\n" && exit 1
    fi
else
    echo -e "${red}此脚本不支持您的操作系统。${plain}\n"
    echo "请确保您使用的是以下受支持的操作系统之一："
    echo "- Ubuntu 20.04+"
    echo "- Debian 11+"
    echo "- CentOS 8+"
    echo "- Fedora 36+"
    echo "- Arch Linux"
    echo "- Parch Linux"
    echo "- Manjaro"
    echo "- Armbian"
    echo "- Alpine Linux"
    echo "- AlmaLinux 9+"
    echo "- Rocky Linux 9+"
    echo "- Oracle Linux 8+"
    echo "- OpenSUSE Tumbleweed"
    exit 1

fi

# Declare Variables
log_folder="${XUI_LOG_FOLDER:=/var/log}"
iplimit_log_path="${log_folder}/3xipl.log"
iplimit_banned_log_path="${log_folder}/3xipl-banned.log"

confirm() {
    if [[ $# > 1 ]]; then
        echo && read -p "$1 [Default $2]: " temp
        if [[ "${temp}" == "" ]]; then
            temp=$2
        fi
    else
        read -p "$1 [y/n]: " temp
    fi
    if [[ "${temp}" == "y" || "${temp}" == "Y" ]]; then
        return 0
    else
        return 1
    fi
}

confirm_restart() {
    confirm "重启面板，注意：重启面板也会重启 Xray" "y"
    if [[ $? == 0 ]]; then
        restart
    else
        show_menu
    fi
}

before_show_menu() {
    echo && echo -n -e "${yellow}按 Enter 键返回主菜单：${plain}" && read temp
    show_menu
}

install() {
    bash <(curl -fsSL "${XUI_RAW_BASE}/install.sh")
    if [[ $? == 0 ]]; then
        if [[ $# == 0 ]]; then
            start
        else
            start 0
        fi
    fi
}

update() {
    confirm "$(echo -e "${green}该功能将从同一 Dui Release 更新面板二进制、菜单和 systemd 服务，数据库不会删除。${red}你想继续吗？${plain}---->>请输入")" "y"
    if [[ $? != 0 ]]; then
        LOGE "已取消"
        [[ $# == 0 ]] && before_show_menu
        return 0
    fi

    local xui_bin="/usr/local/x-ui/x-ui"
    local platform latest_version
    platform=$(dui_arch) || {
        LOGE "不支持的 CPU 架构：$(uname -m)"
        [[ $# == 0 ]] && before_show_menu
        return 1
    }
    latest_version=$(dui_latest_version || true)
    if [[ -z "$latest_version" ]]; then
        LOGE "获取 GitHub 最新 Release 版本失败，旧版本未改动"
        [[ $# == 0 ]] && before_show_menu
        return 1
    fi
    local asset="x-ui-linux-${platform}.tar.gz"
    local package_url="${XUI_RELEASE_BASE}/latest/download/${asset}"
    local tmpdir
    tmpdir=$(mktemp -d) || return 1

    LOGI "下载最新统一 Release 包: ${latest_version}"
    if ! curl -fL --retry 3 --connect-timeout 15 -o "${tmpdir}/${asset}" "$package_url"; then
        LOGE "Release 包下载失败，旧版本未改动"
        rm -rf "$tmpdir"
        [[ $# == 0 ]] && before_show_menu
        return 1
    fi
    if ! curl -fsSL --retry 3 -o "${tmpdir}/${asset}.sha256" "${package_url}.sha256" \
        || ! (cd "$tmpdir" && sha256sum -c "${asset}.sha256" >/dev/null); then
        LOGE "Release 包 SHA-256 校验失败，旧版本未改动"
        rm -rf "$tmpdir"
        [[ $# == 0 ]] && before_show_menu
        return 1
    fi

    if ! tar -tzf "${tmpdir}/${asset}" | grep -qx 'x-ui/x-ui' \
        || ! tar -tzf "${tmpdir}/${asset}" | grep -qx 'x-ui/x-ui.sh' \
        || ! tar -tzf "${tmpdir}/${asset}" | grep -qx 'x-ui/x-ui.service' \
        || ! tar -tzf "${tmpdir}/${asset}" | grep -qx 'x-ui/fb5.sh'; then
        LOGE "Release 包结构不完整，已取消更新"
        rm -rf "$tmpdir"
        [[ $# == 0 ]] && before_show_menu
        return 1
    fi

    if ! tar -xzf "${tmpdir}/${asset}" -C "$tmpdir" x-ui/x-ui x-ui/x-ui.sh x-ui/x-ui.service x-ui/fb5.sh; then
        LOGE "Release 包解压失败，已取消更新"
        rm -rf "$tmpdir"
        [[ $# == 0 ]] && before_show_menu
        return 1
    fi

    chmod +x "${tmpdir}/x-ui/x-ui" "${tmpdir}/x-ui/x-ui.sh" "${tmpdir}/x-ui/fb5.sh"
    if ! bash -n "${tmpdir}/x-ui/x-ui.sh"; then
        LOGE "新菜单脚本语法检查失败，已取消更新"
        rm -rf "$tmpdir"
        [[ $# == 0 ]] && before_show_menu
        return 1
    fi

    local new_version
    new_version=$("${tmpdir}/x-ui/x-ui" -v 2>/dev/null | head -n1)
    if [[ -z "$new_version" ]]; then
        LOGE "新二进制自检失败（无法读取版本），已取消更新"
        rm -rf "$tmpdir"
        [[ $# == 0 ]] && before_show_menu
        return 1
    fi
    LOGI "新面板版本: ${new_version}"

    # 第一次破坏性操作前，先备份当前三项文件。
    [[ -f "$xui_bin" ]] && cp -a "$xui_bin" "${tmpdir}/x-ui.bak"
    [[ -e /usr/bin/x-ui ]] && cp -aL /usr/bin/x-ui "${tmpdir}/x-ui.sh.bak"
    [[ -f /etc/systemd/system/x-ui.service ]] && cp -a /etc/systemd/system/x-ui.service "${tmpdir}/x-ui.service.bak"
    local had_fb5=false
    if [[ -e /usr/local/bin/fb5 ]]; then
        had_fb5=true
        cp -aL /usr/local/bin/fb5 "${tmpdir}/fb5.bak"
    fi

    systemctl stop x-ui 2>/dev/null || true
    # 服务停止后完整备份 /etc/x-ui，包含 SQLite WAL/SHM，便于新版本启动失败时回滚数据库。
    if [[ -d /etc/x-ui ]]; then
        cp -a /etc/x-ui "${tmpdir}/etc-x-ui.bak"
    fi
    install -m 0755 "${tmpdir}/x-ui/x-ui" "$xui_bin"
    install -m 0755 "${tmpdir}/x-ui/x-ui.sh" /usr/bin/x-ui
    ln -sfn /usr/bin/x-ui /usr/local/x-ui/x-ui.sh
    if [[ "$had_fb5" == true ]]; then
        install -m 0755 "${tmpdir}/x-ui/fb5.sh" /usr/local/bin/fb5
    fi
    install -m 0644 "${tmpdir}/x-ui/x-ui.service" /etc/systemd/system/x-ui.service
    systemctl daemon-reload

    if systemctl restart x-ui && sleep 2 && systemctl is-active --quiet x-ui; then
        LOGI "面板、菜单与服务文件已从同一 Release 更新完成，服务运行正常"
        rm -rf "$tmpdir"
        if [[ $# == 0 ]]; then before_show_menu; else exit 0; fi
        return 0
    fi

    LOGE "新版本启动失败，正在自动回滚..."
    [[ -f "${tmpdir}/x-ui.bak" ]] && install -m 0755 "${tmpdir}/x-ui.bak" "$xui_bin"
    [[ -f "${tmpdir}/x-ui.sh.bak" ]] && install -m 0755 "${tmpdir}/x-ui.sh.bak" /usr/bin/x-ui
    ln -sfn /usr/bin/x-ui /usr/local/x-ui/x-ui.sh
    [[ -f "${tmpdir}/x-ui.service.bak" ]] && install -m 0644 "${tmpdir}/x-ui.service.bak" /etc/systemd/system/x-ui.service
    rm -rf /etc/x-ui
    if [[ -f "${tmpdir}/fb5.bak" ]]; then
        install -m 0755 "${tmpdir}/fb5.bak" /usr/local/bin/fb5
    fi
    [[ -d "${tmpdir}/etc-x-ui.bak" ]] && cp -a "${tmpdir}/etc-x-ui.bak" /etc/x-ui
    systemctl daemon-reload
    systemctl restart x-ui 2>/dev/null || true
    rm -rf "$tmpdir"
    LOGE "更新失败，已尝试恢复旧版本"
    [[ $# == 0 ]] && before_show_menu
    return 1
}

update_menu() {
    echo -e "${yellow}更新菜单项${plain}"
    confirm "此功能只更新当前项目中的菜单脚本" "y"
    if [[ $? != 0 ]]; then
        LOGE "已取消"
        [[ $# == 0 ]] && before_show_menu
        return 0
    fi

    local tmpfile
    tmpfile=$(mktemp) || return 1
    if ! curl -fsSL --retry 3 "${XUI_RAW_BASE}/x-ui.sh" -o "$tmpfile" || ! bash -n "$tmpfile"; then
        LOGE "更新菜单项失败，旧菜单未改动"
        rm -f "$tmpfile"
        [[ $# == 0 ]] && before_show_menu
        return 1
    fi
    install -m 0755 "$tmpfile" /usr/bin/x-ui
    ln -sfn /usr/bin/x-ui /usr/local/x-ui/x-ui.sh
    rm -f "$tmpfile"
    LOGI "菜单更新成功"
    if [[ $# == 0 ]]; then before_show_menu; else exit 0; fi
}

custom_version() {
    echo "输入面板 Release 标签 (例: v25.11.30 或 25.11.30):"
    read -r panel_version

    if [[ -z "$panel_version" ]]; then
        echo "面板版本不能为空。"
        return 1
    fi

    # 统一为单个 v 前缀，避免输入 v25... 后变成 vv25...。
    panel_version="${panel_version#v}"
    panel_version="v${panel_version}"

    echo "从同一 Dui 项目安装 ${panel_version}..."
    bash <(curl -fsSL "${XUI_RAW_BASE}/install.sh") "$panel_version"
}

# Function to handle the deletion of the script file
delete_script() {
    rm "$0"  # Remove the script file itself
    exit 1
}

uninstall() {
    confirm "您确定要卸载面板吗? Xray 也将被卸载!" "n"
    if [[ $? != 0 ]]; then
        if [[ $# == 0 ]]; then
            show_menu
        fi
        return 0
    fi
    systemctl stop x-ui
    systemctl disable x-ui
    rm /etc/systemd/system/x-ui.service -f
    systemctl daemon-reload
    systemctl reset-failed
    rm /etc/x-ui/ -rf
    rm /usr/local/x-ui/ -rf

    echo ""
    echo -e "卸载成功\n"
    echo "如果您需要再次安装此面板，可以使用以下命令:"
    echo -e "${green}bash <(curl -fsSL "${XUI_RAW_BASE}/install.sh")${plain}"
    echo ""
    # Trap the SIGTERM signal
    trap delete_script SIGTERM
    delete_script
}

reset_user() {
    confirm "您确定重置面板的用户名和密码吗?" "n"
    if [[ $? != 0 ]]; then
        if [[ $# == 0 ]]; then
            show_menu
        fi
        return 0
    fi
    read -rp "请设置用户名 [默认为随机用户名]: " config_account
    [[ -z $config_account ]] && config_account=$(date +%s%N | md5sum | cut -c 1-8)
    read -rp "请设置密码 [默认为随机密码]: " config_password
    [[ -z $config_password ]] && config_password=$(date +%s%N | md5sum | cut -c 1-8)
    /usr/local/x-ui/x-ui setting -username ${config_account} -password ${config_password} >/dev/null 2>&1
    /usr/local/x-ui/x-ui setting -remove_secret >/dev/null 2>&1
    echo -e "面板登录用户名已重置为：${green} ${config_account} ${plain}"
    echo -e "面板登录密码已重置为：${green} ${config_password} ${plain}"
    echo -e "${yellow} 面板 Secret Token 已禁用 ${plain}"
    echo -e "${green} 请使用新的登录用户名和密码访问 Dui 面板。也请记住它们！${plain}"
    confirm_restart
}

gen_random_string() {
    local length="$1"
    local random_string=$(LC_ALL=C tr -dc 'a-zA-Z0-9' </dev/urandom | fold -w "$length" | head -n 1)
    echo "$random_string"
}

reset_webbasepath() {
    echo -e "${yellow}修改访问路径${plain}"
    
    # Prompt user to set a new web base path
    read -rp "请设置新的访问路径（若回车默认或输入y则为随机路径）: " config_webBasePath
    
    if [[ $config_webBasePath == "y" ]]; then
        config_webBasePath=$(gen_random_string 18)
    fi
    
    # Apply the new web base path setting
    /usr/local/x-ui/x-ui setting -webBasePath "${config_webBasePath}" >/dev/null 2>&1
    systemctl restart x-ui
    
    # Display confirmation message
    echo -e "面板访问路径已重置为: ${green}${config_webBasePath}${plain}"
    echo -e "${green}请使用新的路径登录访问面板${plain}"
}

reset_config() {
    confirm "您确定要重置所有面板设置，帐户数据不会丢失，用户名和密码不会更改" "n"
    if [[ $? != 0 ]]; then
        if [[ $# == 0 ]]; then
            show_menu
        fi
        return 0
    fi
    /usr/local/x-ui/x-ui setting -reset
    echo -e "所有面板设置已重置为默认，请立即重新启动面板，并使用默认的${green}13688${plain}端口访问网页面板"
    confirm_restart
}

check_config() {
    info=$(/usr/local/x-ui/x-ui setting -show true)
    if [[ $? != 0 ]]; then
        LOGE "获取当前设置错误，请检查日志"
        show_menu
    fi
    echo -e "${info}${plain}"
    echo ""
    
    # 获取 IPv4 和 IPv6 地址
    v4=$(curl -s4m8 http://ip.sb -k)
    v6=$(curl -s6m8 http://ip.sb -k)
    local existing_webBasePath=$(/usr/local/x-ui/x-ui setting -show true | grep -Eo 'webBasePath（访问路径）: .+' | awk '{print $2}') 
    local existing_port=$(/usr/local/x-ui/x-ui setting -show true | grep -Eo 'port（端口号）: .+' | awk '{print $2}') 
    local existing_cert=$(/usr/local/x-ui/x-ui setting -getCert true | grep -Eo 'cert: .+' | awk '{print $2}')
    local existing_key=$(/usr/local/x-ui/x-ui setting -getCert true | grep -Eo 'key: .+' | awk '{print $2}')

    if [[ -n "$existing_cert" && -n "$existing_key" ]]; then
        echo -e "${green}面板已安装证书采用SSL保护${plain}"
        echo ""
        local existing_cert=$(/usr/local/x-ui/x-ui setting -getCert true | grep -Eo 'cert: .+' | awk '{print $2}')
        domain=$(basename "$(dirname "$existing_cert")")
        echo -e "${green}登录访问面板URL: https://${domain}:${existing_port}${green}${existing_webBasePath}${plain}"
    fi
    echo ""
    if [[ -z "$existing_cert" && -z "$existing_key" ]]; then
        echo -e "${red}警告：未找到证书和密钥，面板不安全！${plain}"
        echo ""
        echo -e "${green}------->>>>请按照下述方法设置〔ssh转发〕<<<<-------${plain}"
        echo ""

        # 检查 IP 并输出相应的 SSH 和浏览器访问信息
        if [[ -z $v4 ]]; then
            echo -e "${green}1、本地电脑客户端转发命令：${plain} ${blue}ssh  -L [::]:15208:127.0.0.1:${existing_port}${blue} root@[$v6]${plain}"
            echo ""
            echo -e "${green}2、请通过快捷键【Win + R】调出运行窗口，在里面输入【cmd】打开本地终端服务${plain}"
            echo ""
            echo -e "${green}3、请在终端中成功输入服务器的〔root密码〕，注意区分大小写，用以上命令进行转发${plain}"
            echo ""
            echo -e "${green}4、请在浏览器地址栏复制${plain} ${blue}[::1]:15208${existing_webBasePath}${plain} ${green}进入〔Dui面板〕登录界面"
            echo ""
            echo -e "${red}注意：若不使用〔ssh转发〕请为Dui面板配置安装证书再行登录管理后台${plain}"
        elif [[ -n $v4 && -n $v6 ]]; then
            echo -e "${green}1、本地电脑客户端转发命令：${plain} ${blue}ssh -L 15208:127.0.0.1:${existing_port}${blue} root@$v4${plain} ${yellow}或者 ${blue}ssh  -L [::]:15208:127.0.0.1:${existing_port}${blue} root@[$v6]${plain}"
            echo ""
            echo -e "${green}2、请通过快捷键【Win + R】调出运行窗口，在里面输入【cmd】打开本地终端服务${plain}"
            echo ""
            echo -e "${green}3、请在终端中成功输入服务器的〔root密码〕，注意区分大小写，用以上命令进行转发${plain}"
            echo ""
            echo -e "${green}4、请在浏览器地址栏复制${plain} ${blue}127.0.0.1:15208${existing_webBasePath}${plain} ${yellow}或者${plain} ${blue}[::1]:15208${existing_webBasePath}${plain} ${green}进入〔Dui面板〕登录界面"
            echo ""
            echo -e "${red}注意：若不使用〔ssh转发〕请为Dui面板配置安装证书再行登录管理后台${plain}"
        else
            echo -e "${green}1、本地电脑客户端转发命令：${plain} ${blue}ssh -L 15208:127.0.0.1:${existing_port}${blue} root@$v4${plain}"
            echo ""
            echo -e "${green}2、请通过快捷键【Win + R】调出运行窗口，在里面输入【cmd】打开本地终端服务${plain}"
            echo ""
            echo -e "${green}3、请在终端中成功输入服务器的〔root密码〕，注意区分大小写，用以上命令进行转发${plain}"
            echo ""
            echo -e "${green}4、请在浏览器地址栏复制${plain} ${blue}127.0.0.1:15208${existing_webBasePath}${plain} ${green}进入〔Dui面板〕登录界面"
            echo ""
            echo -e "${red}注意：若不使用〔ssh转发〕请为Dui面板配置安装证书再行登录管理后台${plain}"
            echo ""
        fi
    fi
}

set_port() {
    echo && echo -n -e "输入端口号 [1-65535]: " && read port
    if [[ -z "${port}" ]]; then
        LOGD "Cancelled"
        before_show_menu
    else
        /usr/local/x-ui/x-ui setting -port ${port}
        echo -e "端口已设置，请立即重启面板，并使用新端口 ${green}${port}${plain} 以访问面板"
        confirm_restart
    fi
}

start() {
    check_status
    if [[ $? == 0 ]]; then
        echo ""
        LOGI "面板正在运行，无需再次启动，如需重新启动，请选择重新启动"
    else
        systemctl start x-ui
        sleep 2
        check_status
        if [[ $? == 0 ]]; then
            LOGI "Dui 已成功启动"
        else
            LOGE "面板启动失败，可能是启动时间超过两秒，请稍后查看日志信息"
        fi
    fi

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

stop() {
    check_status
    if [[ $? == 1 ]]; then
        echo ""
        LOGI "面板已关闭，无需再次关闭！"
    else
        systemctl stop x-ui
        sleep 2
        check_status
        if [[ $? == 1 ]]; then
            LOGI "Dui 和 Xray 已成功关闭"
        else
            LOGE "面板关闭失败，可能是停止时间超过两秒，请稍后查看日志信息"
        fi
    fi

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

restart() {
    systemctl restart x-ui
    sleep 2
    check_status
    if [[ $? == 0 ]]; then
        LOGI "Dui 和 Xray 已成功重启"
    else
        LOGE "面板重启失败，可能是启动时间超过两秒，请稍后查看日志信息"
    fi
    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

status() {
    systemctl status x-ui -l
    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

enable() {
    systemctl enable x-ui
    if [[ $? == 0 ]]; then
        LOGI "x-ui 已成功设置开机启动"
    else
        LOGE "x-ui 设置开机启动失败"
    fi

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

disable() {
    systemctl disable x-ui
    if [[ $? == 0 ]]; then
        LOGI "x-ui 已成功取消开机启动"
    else
        LOGE "x-ui 取消开机启动失败"
    fi

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

show_log() {
    journalctl -u x-ui.service -e --no-pager -f
    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

bbr_menu() {
    echo -e "${green}\t1.${plain} 启用 BBR"
    echo -e "${green}\t2.${plain} 禁用 BBR"
    echo -e "${green}\t0.${plain} 返回主菜单"
    read -p "请输入选项: " choice
    case "$choice" in
    0)
        show_menu
        ;;
    1)
        enable_bbr
        ;;
    2)
        disable_bbr
        ;;
    *) echo "无效选项" ;;
    esac
}

disable_bbr() {
    if ! grep -q "net.core.default_qdisc=fq" /etc/sysctl.conf || ! grep -q "net.ipv4.tcp_congestion_control=bbr" /etc/sysctl.conf; then
        echo -e "${yellow}BBR 当前未启用${plain}"
        exit 0
    fi

    # Replace BBR with CUBIC configurations
    sed -i 's/net.core.default_qdisc=fq/net.core.default_qdisc=pfifo_fast/' /etc/sysctl.conf
    sed -i 's/net.ipv4.tcp_congestion_control=bbr/net.ipv4.tcp_congestion_control=cubic/' /etc/sysctl.conf

    # Apply changes
    sysctl -p

    # Verify that BBR is replaced with CUBIC
    if [[ $(sysctl net.ipv4.tcp_congestion_control | awk '{print $3}') == "cubic" ]]; then
        echo -e "${green}BBR 已成功替换为 CUBIC${plain}"
    else
        echo -e "${red}用 CUBIC 替换 BBR 失败，请检查您的系统配置。${plain}"
    fi
}

enable_bbr() {
    if grep -q "net.core.default_qdisc=fq" /etc/sysctl.conf && grep -q "net.ipv4.tcp_congestion_control=bbr" /etc/sysctl.conf; then
        echo -e "${green}BBR 已经启用!${plain}"
        exit 0
    fi

    # Check the OS and install necessary packages
    case "${release}" in
    ubuntu | debian | armbian)
        apt-get update && apt-get install -yqq --no-install-recommends ca-certificates
        ;;
    centos | almalinux | rocky | oracle)
        yum -y update && yum -y install ca-certificates
        ;;
    fedora)
        dnf -y update && dnf -y install ca-certificates
        ;;
    arch | manjaro)
        pacman -Sy --noconfirm ca-certificates
        ;;
    *)
        echo -e "${red}不支持的操作系统。请检查脚本并手动安装必要的软件包${plain}\n"
        exit 1
        ;;
    esac

    # Enable BBR
    echo "net.core.default_qdisc=fq" | tee -a /etc/sysctl.conf
    echo "net.ipv4.tcp_congestion_control=bbr" | tee -a /etc/sysctl.conf

    # Apply changes
    sysctl -p

    # Verify that BBR is enabled
    if [[ $(sysctl net.ipv4.tcp_congestion_control | awk '{print $3}') == "bbr" ]]; then
        echo -e "${green}BBR 已成功启用${plain}"
    else
        echo -e "${red}启用 BBR 失败，请检查您的系统配置${plain}"
    fi
}

update_shell() {
    # Keep the local menu aligned with this same source project.
    local url="${XUI_RAW_BASE}/x-ui.sh"
    local tmpfile
    tmpfile=$(mktemp) || return 1
    LOGI "正在检查菜单脚本更新..."
    if ! curl -fsSL --retry 2 "$url" -o "$tmpfile" || ! bash -n "$tmpfile"; then
        LOGE "获取最新菜单失败，继续使用当前版本"
        rm -f "$tmpfile"
        return 1
    fi
    if cmp -s "$tmpfile" /usr/bin/x-ui 2>/dev/null; then
        rm -f "$tmpfile"
        return 0
    fi
    install -m 0755 "$tmpfile" /usr/bin/x-ui || { rm -f "$tmpfile"; return 1; }
    ln -sfn /usr/bin/x-ui /usr/local/x-ui/x-ui.sh
    rm -f "$tmpfile"
    LOGI "菜单脚本已自动更新"
}

# 0: running, 1: not running, 2: not installed
check_status() {
    if [[ ! -f /etc/systemd/system/x-ui.service ]]; then
        return 2
    fi
    temp=$(systemctl status x-ui | grep Active | awk '{print $3}' | cut -d "(" -f2 | cut -d ")" -f1)
    if [[ "${temp}" == "running" ]]; then
        return 0
    else
        return 1
    fi
}

check_enabled() {
    temp=$(systemctl is-enabled x-ui)
    if [[ "${temp}" == "enabled" ]]; then
        return 0
    else
        return 1
    fi
}

check_uninstall() {
    check_status
    if [[ $? != 2 ]]; then
        echo ""
        LOGE "面板已安装，请勿重新安装"
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
        LOGE "请先安装面板"
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
        echo -e "面板状态: ${green}运行中${plain}"
        show_enable_status
        ;;
    1)
        echo -e "面板状态: ${yellow}未运行${plain}"
        show_enable_status
        ;;
    2)
        echo -e "面板状态: ${red}未安装${plain}"
        ;;
    esac
    show_xray_status
}

show_enable_status() {
    check_enabled
    if [[ $? == 0 ]]; then
        echo -e "开机启动: ${green}是${plain}"
    else
        echo -e "开机启动: ${red}否${plain}"
    fi
}

check_xray_status() {
    count=$(ps -ef | grep "xray-linux" | grep -v "grep" | wc -l)
    if [[ count -ne 0 ]]; then
        return 0
    else
        return 1
    fi
}

show_xray_status() {
    check_xray_status
    if [[ $? == 0 ]]; then
        echo -e "Xray状态: ${green}运行中${plain}"
    else
        echo -e "Xray状态: ${red}未运行${plain}"
    fi
}

firewall_menu() {
    echo -e "${green}\t1.${plain} 安装防火墙并开放端口"
    echo -e "${green}\t2.${plain} 允许列表"
    echo -e "${green}\t3.${plain} 从列表中删除端口"
    echo -e "${green}\t4.${plain} 禁用防火墙"
    echo -e "${green}\t0.${plain} 返回主菜单"
    read -p "请输入选项: " choice
    case "$choice" in
    0)
        show_menu
        ;;
    1)
        open_ports
        ;;
    2)
        sudo ufw status
        ;;
    3)
        delete_ports
        ;;
    4)
        sudo ufw disable
        ;;
    *) echo "无效选项" ;;
    esac
}

open_ports() {
    if ! command -v ufw &>/dev/null; then
        echo "ufw 防火墙未安装，正在安装..."
        apt-get update
        apt-get install -y ufw
    else
        echo "ufw 防火墙已安装"
    fi

    # Check if the firewall is inactive
    if ufw status | grep -q "Status: active"; then
        echo "防火墙已经激活"
    else
        # Open the necessary ports
        ufw allow ssh
        ufw allow http
        ufw allow https
        ufw allow 13688/tcp

        # Enable the firewall
        ufw --force enable
    fi

    # Prompt the user to enter a list of ports
    read -p "输入您要打开的端口（例如 80,443,13688 或端口范围 400-500): " ports

    # Check if the input is valid
    if ! [[ $ports =~ ^([0-9]+|[0-9]+-[0-9]+)(,([0-9]+|[0-9]+-[0-9]+))*$ ]]; then
        echo "错误：输入无效。请输入以英文逗号分隔的端口列表或端口范围（例如 80,443,13688 或 400-500)" >&2
        exit 1
    fi

    # Open the specified ports using ufw
    IFS=',' read -ra PORT_LIST <<<"$ports"
    for port in "${PORT_LIST[@]}"; do
        if [[ $port == *-* ]]; then
            # Split the range into start and end ports
            start_port=$(echo $port | cut -d'-' -f1)
            end_port=$(echo $port | cut -d'-' -f2)
            # Loop through the range and open each port
            for ((i = start_port; i <= end_port; i++)); do
                ufw allow $i
            done
        else
            ufw allow "$port"
        fi
    done

    # Confirm that the ports are open
    ufw status | grep $ports
}

delete_ports() {
    # Prompt the user to enter the ports they want to delete
    read -p "输入要删除的端口（例如 80,443,13688 或范围 400-500): " ports

    # Check if the input is valid
    if ! [[ $ports =~ ^([0-9]+|[0-9]+-[0-9]+)(,([0-9]+|[0-9]+-[0-9]+))*$ ]]; then
        echo "错误：输入无效。请输入以英文逗号分隔的端口列表或端口范围（例如 80,443,13688 或 400-500)" >&2
        exit 1
    fi

    # Delete the specified ports using ufw
    IFS=',' read -ra PORT_LIST <<<"$ports"
    for port in "${PORT_LIST[@]}"; do
        if [[ $port == *-* ]]; then
            # Split the range into start and end ports
            start_port=$(echo $port | cut -d'-' -f1)
            end_port=$(echo $port | cut -d'-' -f2)
            # Loop through the range and delete each port
            for ((i = start_port; i <= end_port; i++)); do
                ufw delete allow $i
            done
        else
            ufw delete allow "$port"
        fi
    done

    # Confirm that the ports are deleted
    echo "删除指定端口:"
    ufw status | grep $ports
}

update_geo() {
    local defaultBinFolder="/usr/local/x-ui/bin"
    read -p "请输入 x-ui bin 文件夹路径，默认留空。（默认值：'${defaultBinFolder}')" binFolder
    binFolder=${binFolder:-${defaultBinFolder}}
    if [[ ! -d ${binFolder} ]]; then
        LOGE "文件夹 ${binFolder} 不存在！"
        LOGI "制作 bin 文件夹：${binFolder}..."
        mkdir -p ${binFolder}
    fi

    systemctl stop x-ui
    cd ${binFolder}
    rm -f geoip.dat geosite.dat geoip_IR.dat geosite_IR.dat geoip_VN.dat geosite_VN.dat
    wget -N https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat
    wget -N https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat
    wget -O geoip_IR.dat -N https://github.com/chocolate4u/Iran-v2ray-rules/releases/latest/download/geoip.dat
    wget -O geosite_IR.dat -N https://github.com/chocolate4u/Iran-v2ray-rules/releases/latest/download/geosite.dat
    wget -O geoip_VN.dat https://github.com/vuong2023/vn-v2ray-rules/releases/latest/download/geoip.dat
    wget -O geosite_VN.dat https://github.com/vuong2023/vn-v2ray-rules/releases/latest/download/geosite.dat
    systemctl start x-ui
    echo -e "${green}Geosite.dat + Geoip.dat + geoip_IR.dat + geosite_IR.dat 在 bin 文件夹: '${binfolder}' 中已经更新成功 !${plain}"
    before_show_menu
}

install_acme() { 
    # 检查是否已安装 acme.sh
    if command -v ~/.acme.sh/acme.sh &>/dev/null; then 
        LOGI "acme.sh 已经安装。" 
        return 0 
    fi 
 
    LOGI "正在安装 acme.sh..." 
    cd ~ || return 1 # 确保可以切换到主目录
 
    curl -s https://get.acme.sh | sh 
    if [ $? -ne 0 ]; then 
        LOGE "安装 acme.sh 失败。" 
        return 1 
    else 
        LOGI "安装 acme.sh 成功。" 
    fi 
 
    return 0 
} 

ssl_cert_issue_main() { 
    echo -e "${green}\t1.${plain} 获取 SSL 证书" 
    echo -e "${green}\t2.${plain} 撤销证书" 
    echo -e "${green}\t3.${plain} 强制更新证书" 
    echo -e "${green}\t4.${plain} 显示现有域名" 
    echo -e "${green}\t5.${plain} 为面板设置证书路径" 
    echo -e "${green}\t0.${plain} 返回主菜单" 
 
    read -rp "请选择一个选项：" choice 
    case "$choice" in 
    0) 
        show_menu 
        ;; 
    1) 
        ssl_cert_issue 
        ssl_cert_issue_main 
        ;; 
    2) 
        local domains=$(find /root/cert/ -mindepth 1 -maxdepth 1 -type d -exec basename {} \;) 
        if [ -z "$domains" ]; then 
            echo "未找到可撤销的证书。" 
        else 
            echo "现有域名：" 
            echo "$domains" 
            read -rp "请从列表中输入要撤销证书的域名：" domain 
            if echo "$domains" | grep -qw "$domain"; then 
                ~/.acme.sh/acme.sh --revoke -d ${domain} 
                LOGI "已撤销域名的证书：$domain" 
            else 
                echo "输入的域名无效。" 
            fi 
        fi 
        ssl_cert_issue_main 
        ;; 
    3) 
        local domains=$(find /root/cert/ -mindepth 1 -maxdepth 1 -type d -exec basename {} \;) 
        if [ -z "$domains" ]; then 
            echo "未找到可更新的证书。" 
        else 
            echo "现有域名：" 
            echo "$domains" 
            read -rp "请从列表中输入要强制更新 SSL 证书的域名：" domain 
            if echo "$domains" | grep -qw "$domain"; then 
                ~/.acme.sh/acme.sh --renew -d ${domain} --force 
                if [[ $? -eq 0 ]]; then
                    /usr/local/x-ui/x-ui cert-manager-touch -domain "${domain}" >/dev/null 2>&1 || true
                fi
                LOGI "已强制更新域名的证书：$domain" 
            else 
                echo "输入的域名无效。" 
            fi 
        fi 
        ssl_cert_issue_main 
        ;; 
    4) 
        local domains=$(find /root/cert/ -mindepth 1 -maxdepth 1 -type d -exec basename {} \;) 
        if [ -z "$domains" ]; then 
            echo "未找到证书。" 
        else 
            echo "现有域名及其路径：" 
            for domain in $domains; do 
                local cert_path="/root/cert/${domain}/fullchain.pem" 
                local key_path="/root/cert/${domain}/privkey.pem" 
                if [[ -f "${cert_path}" && -f "${key_path}" ]]; then 
                    echo -e "域名：${domain}" 
                    echo -e "\t证书路径：${cert_path}" 
                    echo -e "\t私钥路径：${key_path}" 
                else 
                    echo -e "域名：${domain} - 证书或私钥文件缺失。" 
                fi 
            done 
        fi 
        ssl_cert_issue_main 
        ;; 
    5) 
        local domains=$(find /root/cert/ -mindepth 1 -maxdepth 1 -type d -exec basename {} \;) 
        if [ -z "$domains" ]; then 
            echo "未找到证书。" 
        else 
            echo "可用域名：" 
            echo "$domains" 
            read -rp "请选择要为面板设置证书路径的域名：" domain 
 
            if echo "$domains" | grep -qw "$domain"; then 
                local webCertFile="/root/cert/${domain}/fullchain.pem" 
                local webKeyFile="/root/cert/${domain}/privkey.pem" 
 
                if [[ -f "${webCertFile}" && -f "${webKeyFile}" ]]; then 
                    /usr/local/x-ui/x-ui cert -webCert "$webCertFile" -webCertKey "$webKeyFile" 
                    echo "已为域名设置面板证书路径：$domain" 
                    echo "  - 证书文件：$webCertFile" 
                    echo "  - 私钥路径：$webKeyFile" 
                    restart 
                else 
                    echo "未找到域名的证书或私钥：$domain" 
                fi 
            else 
                echo "输入的域名无效。" 
            fi 
        fi 
        ssl_cert_issue_main 
        ;; 
 
    *) 
        echo -e "${red}无效选项。请选择有效的数字。${plain}\n" 
        ssl_cert_issue_main 
        ;; 
    esac 
} 

ssl_cert_issue() {
    local existing_webBasePath=$(/usr/local/x-ui/x-ui setting -show true | grep -Eo 'webBasePath（访问路径）: .+' | awk '{print $2}')
    local existing_port=$(/usr/local/x-ui/x-ui setting -show true | grep -Eo 'port（端口号）: .+' | awk '{print $2}')
    # 首先检查 acme.sh
    echo ""
    if ! command -v ~/.acme.sh/acme.sh &>/dev/null; then
        echo "未找到 acme.sh，将进行安装"
        install_acme
        if [ $? -ne 0 ]; then
            LOGE "安装 acme 失败，请检查日志"
            exit 1
        fi
    fi

    # 安装 socat 和 dnsutils/bind-utils (用于 dig)
    case "${release}" in
    ubuntu | debian | armbian)
        # 添加了 dnsutils 用于 dig 命令
        apt update && apt install socat dnsutils -y
        ;;
    centos | rhel | almalinux | rocky | ol)
        # 添加了 bind-utils 用于 dig 命令
        yum -y update && yum -y install socat bind-utils
        ;;
    fedora | amzn | virtuozzo)
        # 添加了 bind-utils 用于 dig 命令
        dnf -y update && dnf -y install socat bind-utils
        ;;
    arch | manjaro | parch)
        # 添加了 dnsutils 用于 dig 命令
        pacman -Sy --noconfirm socat dnsutils
        ;;
    *)
        echo ""
        echo -e "${red}不支持的操作系统。请检查脚本并手动安装必要的软件包。${plain}\n"
        exit 1
        ;;
    esac
    if [ $? -ne 0 ]; then
        LOGE "安装 socat 或 dig 工具 失败，请检查日志"
        echo ""
        exit 1
    else
        LOGI "安装 socat 和 dig 工具 成功..."
        echo ""
    fi

    # 在这里获取域名，我们需要验证它
    local domain=""
    # 强制从终端读取输入，避免被管道跳过
    read -rp "请输入您的域名: " domain </dev/tty
    echo ""
    LOGD "您的域名是: ${domain}, 正在检查..."
    echo ""

    # --- 新增：域名解析验证 ---
    LOGD "正在获取本机公共 IP..."
    echo ""
    public_ip=$(curl -s4m8 http://ip.sb -k)
    
    if [ -z "$public_ip" ]; then
        LOGE "获取本机公共 IP 失败，请检查网络连接！"
        exit 1
    fi
    LOGI "本机公共 IP: ${public_ip}"
    echo ""

    LOGD "正在查询域名 ${domain} 的 DNS ----->>> A 记录..."
    # 确保只获取A记录，并取第一个
    domain_ip=$(dig +short $domain A | head -n 1)

    if [ -z "$domain_ip" ]; then
        echo ""
        LOGE "未能查询到域名 ${domain} 的 “A 记录”。"
        echo ""
        LOGE "请确保您的域名已在 DNS 服务商处添加了〔A 记录〕，并指向本机 IP。"
        echo ""
        LOGE "提示：如果您使用的是 Cloudflare，请确保【小黄云】（代理）已关闭。"
        exit 1
    fi
    LOGI "域名 ${domain} 解析到 IP: ${domain_ip}"
    echo ""

    if [ "$public_ip" != "$domain_ip" ]; then
        echo ""
        LOGE "域名 ${domain} 解析的 IP (${domain_ip}) 与本机 IP (${public_ip}) 不符！"
        echo ""
        LOGE "请检查您的 DNS 中 的〔A 记录〕设置是否正确。"
        echo ""
        LOGE "提示：如果您使用的是 Cloudflare，请确保【小黄云】（代理）已关闭。"
        exit 1
    fi

    LOGI "域名解析验证成功，继续执行证书申请......"
    echo ""

    # 检查是否已存在证书
    local currentCert=$(~/.acme.sh/acme.sh --list | tail -1 | awk '{print $1}')
    if [ "${currentCert}" == "${domain}" ]; then
        local certInfo=$(~/.acme.sh/acme.sh --list)
        LOGE "系统已存在此域名的证书。无法再次签发。当前证书详情:"
        LOGI "$certInfo"
        exit 1
    else
        LOGI "您的域名现在可以签发证书了......"
    fi

    # 为证书创建一个目录
    echo ""
    certPath="/root/cert/${domain}"
    if [ ! -d "$certPath" ]; then
        mkdir -p "$certPath"
    else
        rm -rf "$certPath"
        mkdir -p "$certPath"
    fi

    # 获取独立服务器的端口号
    local WebPort=80
    read -rp "请选择要使用的端口 (默认为 80): " WebPort
    if [[ ${WebPort} -gt 65535 || ${WebPort} -lt 1 ]]; then
        LOGE "您输入的 ${WebPort} 无效，将使用默认端口 80。"
        WebPort=80
    fi
    LOGI "将使用端口: ${WebPort} 来签发证书。请确保此端口已开放。"

    # 签发证书
    echo ""
    ~/.acme.sh/acme.sh --set-default-ca --server letsencrypt
    ~/.acme.sh/acme.sh --issue -d ${domain} --listen-v6 --standalone --httpport ${WebPort} --force
    if [ $? -ne 0 ]; then
        LOGE "签发证书失败，请检查日志。"
        rm -rf ~/.acme.sh/${domain}
        exit 1
    else
        LOGE "签发证书成功，正在安装证书..."
    fi

    # --- 自动设置 reloadCmd ---
    echo ""
    reloadCmd="x-ui restart"
    LOGI "ACME 的 --reloadcmd 已自动设置为: ${yellow}x-ui restart"
    
    # 安装证书
    echo ""
    ~/.acme.sh/acme.sh --installcert -d ${domain} \
        --key-file /root/cert/${domain}/privkey.pem \
        --fullchain-file /root/cert/${domain}/fullchain.pem \
        --reloadcmd "${reloadCmd}"

    echo ""
    if [ $? -ne 0 ]; then
        LOGE "安装证书失败，正在退出。"
        rm -rf ~/.acme.sh/${domain}
        exit 1
    else
        LOGI "安装证书成功，正在启用自动续订..."
    fi

    # 启用自动续订
    echo ""
    ~/.acme.sh/acme.sh --upgrade --auto-upgrade
    if [ $? -ne 0 ]; then
        LOGE "自动续订失败，证书详情："
        ls -lah cert/*
        chmod 755 $certPath/*
        exit 1
    else
        LOGI "自动续订成功，证书详情："
        ls -lah cert/*
        chmod 755 $certPath/*
    fi

    # ---  自动为面板设置证书路径  ---
    echo ""
    local webCertFile="/root/cert/${domain}/fullchain.pem"
    local webKeyFile="/root/cert/${domain}/privkey.pem"

    if [[ -f "$webCertFile" && -f "$webKeyFile" ]]; then
        /usr/local/x-ui/x-ui cert -webCert "$webCertFile" -webCertKey "$webKeyFile"
        LOGI "已为域名自动设置面板证书路径: $domain"
        echo ""
        LOGI "  - 证书文件: $webCertFile"
        LOGI "  - 私钥文件: $webKeyFile"
        echo ""
        echo -e "${green}登录访问面板URL: https://${domain}:${existing_port}${green}${existing_webBasePath}${plain}"
        echo ""
        echo -e "${green}PS：若您要登录访问面板，请复制上面的地址到浏览器打开即可${plain}"
        echo ""
        /usr/local/x-ui/x-ui cert-manager-record \
            -source "menu18-standalone" \
            -domain "${domain}" \
            -cert "${webCertFile}" \
            -key "${webKeyFile}" \
            -issuePort "${WebPort}" >/dev/null 2>&1 || LOGE "证书续期管理信息写入失败，但当前证书已正常安装。"
        restart
    else
        LOGE "错误：未找到域名的证书或私钥文件: $domain。"
    fi
}

ssl_cert_issue_CF() {
    local existing_webBasePath=$(/usr/local/x-ui/x-ui setting -show true | grep -Eo 'webBasePath（访问路径）: .+' | awk '{print $2}')
    local existing_port=$(/usr/local/x-ui/x-ui setting -show true | grep -Eo 'port（端口号）: .+' | awk '{print $2}')
    LOGI "****** 使用说明 ******"
    echo ""
    LOGI "请按照以下步骤完成操作："
    echo ""
    LOGI "1. 准备好在 Cloudflare 注册的电子邮箱。"
    echo ""
    LOGI "2. 准备好 Cloudflare Global API 密钥。"
    echo ""
    LOGI "3. 准备好一个可用的域名。"
    echo ""
    LOGI "4. 证书颁发后，系统将自动为您设置证书路径。"
    echo ""
    LOGI "5. 安装后，脚本还支持自动续订 SSL 证书。"
    echo ""

    confirm "您确认信息并希望继续吗？[y/n]" "y"

    if [ $? -eq 0 ]; then
        # 首先检查 acme.sh
        if ! command -v ~/.acme.sh/acme.sh &>/dev/null; then
            echo "未找到 acme.sh。我们将为您安装。"
            install_acme
            if [ $? -ne 0 ]; then
                LOGE "安装 acme 失败，请检查日志。"
                exit 1
            fi
        fi

        CF_Domain=""

        LOGD "请设置一个域名："
        # 强制从终端读取输入
        read -rp "在此输入您的域名: " CF_Domain </dev/tty
        LOGD "您的域名设置为：${CF_Domain}"

        # 设置 Cloudflare API 详细信息
        CF_GlobalKey=""
        CF_AccountEmail=""
        LOGD "请设置 API 密钥："
        # 强制从终端读取输入
        read -rp "在此输入您的密钥: " CF_GlobalKey </dev/tty
        LOGD "您的 API 密钥是：${CF_GlobalKey}"

        LOGD "请设置注册的电子邮箱："
        # 强制从终端读取输入
        read -rp "在此输入您的电子邮箱: " CF_AccountEmail </dev/tty
        LOGD "您注册的电子邮箱地址是：${CF_AccountEmail}"

        # 将默认 CA 设置为 Let's Encrypt
        ~/.acme.sh/acme.sh --set-default-ca --server letsencrypt
        if [ $? -ne 0 ]; then
            LOGE "设置默认 CA 为 Let's Encrypt 失败，脚本正在退出..."
            exit 1
        fi

        export CF_Key="${CF_GlobalKey}"
        export CF_Email="${CF_AccountEmail}"

        # 使用 Cloudflare DNS 颁发证书
        echo ""
        ~/.acme.sh/acme.sh --issue --dns dns_cf -d ${CF_Domain} -d *.${CF_Domain} --log --force
        if [ $? -ne 0 ]; then
            LOGE "证书颁发失败，脚本正在退出..."
            exit 1
        else
            LOGI "证书颁发成功，正在安装..."
        fi
        
        # 为证书创建一个目录
        echo ""
        certPath="/root/cert/${CF_Domain}"
        if [ -d "$certPath" ]; then
            rm -rf ${certPath}
        fi

        mkdir -p ${certPath}
        if [ $? -ne 0 ]; then
            LOGE "创建目录失败: ${certPath}"
            exit 1
        fi

        # --- 自动设置 reloadCmd ---
        echo ""
        reloadCmd="x-ui restart"
        LOGI "ACME 的 --reloadcmd 已自动设置为: ${yellow}x-ui restart"
        echo ""

        # 执行“安装证书”流程
        echo ""
        ~/.acme.sh/acme.sh --installcert -d ${CF_Domain} -d *.${CF_Domain} \
            --key-file ${certPath}/privkey.pem \
            --fullchain-file ${certPath}/fullchain.pem \
            --reloadcmd "${reloadCmd}"
        
        echo ""
        if [ $? -ne 0 ]; then
            LOGE "证书安装失败，脚本正在退出..."
            exit 1
        else
            LOGI "证书安装成功，正在开启自动更新..."
        fi

        # 启用自动续订
        echo ""
        ~/.acme.sh/acme.sh --upgrade --auto-upgrade
        if [ $? -ne 0 ]; then
            LOGE "自动续订设置失败，脚本正在退出..."
            exit 1
        else
            LOGI "证书已安装并开启自动续订。具体信息如下："
            ls -lah ${certPath}/*
            chmod 755 ${certPath}/*
        fi

        # --- 自动为面板设置证书路径 ---
        echo ""
        local webCertFile="${certPath}/fullchain.pem"
        local webKeyFile="${certPath}/privkey.pem"

        if [[ -f "$webCertFile" && -f "$webKeyFile" ]]; then
            /usr/local/x-ui/x-ui cert -webCert "$webCertFile" -webCertKey "$webKeyFile"
            echo ""
            LOGI "已为域名自动设置面板证书路径: $CF_Domain"
            echo ""
            LOGI "  - 证书文件: $webCertFile"
            LOGI "  - 私钥文件: $webKeyFile"
            echo ""
            echo -e "${green}登录访问面板URL: https://${CF_Domain}:${existing_port}${green}${existing_webBasePath}${plain}"
            echo ""
            echo -e "${green}PS：若您要登录访问面板，请复制上面的地址到浏览器打开即可${plain}"
            echo ""
            /usr/local/x-ui/x-ui cert-manager-record \
                -source "menu19-cloudflare" \
                -domain "${CF_Domain}" \
                -cert "${webCertFile}" \
                -key "${webKeyFile}" \
                -wildcard=true >/dev/null 2>&1 || LOGE "证书续期管理信息写入失败，但当前证书已正常安装。"
            restart    # 自动重启面板以应用证书
        else
            LOGE "错误：未找到域名的证书或私钥文件: $CF_Domain。"
        fi

    else
        show_menu
    fi
}

warp_cloudflare() {
    echo -e "${green}\t1.${plain} 安装 WARP socks5 代理"
    echo -e "${green}\t2.${plain} 账户类型 (free, plus, team)"
    echo -e "${green}\t3.${plain} 开启 / 关闭 WireProxy"
    echo -e "${green}\t4.${plain} 卸载 WARP"
    echo -e "${green}\t0.${plain} 返回主菜单"
    read -p "请输入选项: " choice
    case "$choice" in
    0)
        show_menu
        ;;
    1)
        bash <(curl -sSL https://raw.githubusercontent.com/hamid-gh98/x-ui-scripts/main/install_warp_proxy.sh)
        ;;
    2)
        warp a
        ;;
    3)
        warp y
        ;;
    4)
        warp u
        ;;
    *) echo "无效选项" ;;
    esac
}

# --------- 【订阅转换】模块 ---------- 
subconverter() {
echo ""
echo -e "${green}==============================================="
echo -e "〔订阅转换〕一键部署"
echo -e "1. 自动安装/部署 Nginx"
echo -e "2. 自动调用面板的证书"
echo -e "3. 自动部署 Sublink 服务"
echo -e "4. 自动配置 Nginx 反向代理"
echo -e "5. 可直观在前端页面配置订阅"
echo -e "Dui 集成功能"
echo -e "===============================================${plain}"
echo ""
    local existing_cert=$(/usr/local/x-ui/x-ui setting -getCert true | grep -Eo 'cert: .+' | awk '{print $2}')
    local existing_key=$(/usr/local/x-ui/x-ui setting -getCert true | grep -Eo 'key: .+' | awk '{print $2}')

    if [[ -n "$existing_cert" && -n "$existing_key" ]]; then
    echo -e "${green}面板已安装证书采用SSL保护${plain}"
    echo ""
    domain=$(basename "$(dirname "$existing_cert")")
    echo -e "${green}------------->>>>接下来进行sublink订阅转换服务的安装  ........${plain}"
    sleep 3
    echo ""
else
    echo -e "${red}警告：未找到证书和密钥，面板不安全！${plain}"
    echo ""
    echo -e "${green}------->>>>且不能安装sublink订阅转换服务<<<<-------${plain}"
    echo ""
    sleep 5
    exit 1
fi

# --------- 安装/部署sublink服务 ----------

if [[ -z "${SUBLINK_INSTALL_URL:-}" ]]; then
    echo -e "${yellow}Dui 不内置第三方 Sublink 安装源。${plain}"
    echo -e "${yellow}如需使用此功能，请先设置 SUBLINK_INSTALL_URL 后重试。${plain}"
    return 1
fi
bash <(curl -Ls "$SUBLINK_INSTALL_URL")


# --------- 安装 Nginx ----------
if ! command -v nginx &>/dev/null; then
    echo -e "${yellow}-------------->>>>>>>>未检测到 Nginx，正在安装...${plain}"
    apt update && apt install -y nginx
    systemctl enable nginx
    systemctl start nginx
else
    echo -e "${green}检测到 Nginx 已安装，跳过安装步骤${plain}"
fi

# --------- 拷贝Dui已有证书到 Nginx ----------
mkdir -p /etc/nginx/ssl
acme_path="/root/.acme.sh/${domain}_ecc"

cp "${acme_path}/fullchain.cer" "/etc/nginx/ssl/${domain}.cer"
cp "${acme_path}/${domain}.key" "/etc/nginx/ssl/${domain}.key"

# --------- 配置 Nginx 反向代理 ----------
NGINX_CONF="/etc/nginx/conf.d/sublink.conf"
cat > $NGINX_CONF <<EOF
server {
    listen 15268 ssl http2;
    server_name ${domain};

    # 证书路径（从 acme.sh 复制到 /etc/nginx/ssl/ 下）
    ssl_certificate     /etc/nginx/ssl/${domain}.cer;
    ssl_certificate_key /etc/nginx/ssl/${domain}.key;

    ssl_protocols TLSv1.2 TLSv1.3;

    location / {
        proxy_pass http://127.0.0.1:8000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    }
}
EOF

# 重载 nginx，让新证书生效
sleep 1
systemctl reload nginx
sleep 2

# --------- 使用 sed 替换 ExecStart 行，添加启动参数 ----------
sudo sed -i "/^ExecStart=/ s|$| run --port 8000|" "/etc/systemd/system/sublink.service"
# 重新加载 systemd 守护进程
sudo systemctl daemon-reload
# 重启 sublink 服务
sudo systemctl restart sublink


# --------- 开放防火墙端口 ----------
echo ""
echo -e "${yellow}请务必手动放行${plain}${red} 8000 和 15268 ${yellow}端口！！${plain}"
echo ""

# --------- 完成提示 ----------
echo ""
echo -e "${green}【订阅转换模块】安装完成！！！${plain}"
echo ""
echo -e "${green}登录用户名：admin，密码：123456，请进后台自行修改${plain}"
echo ""
echo -e "${green}Web 界面访问地址：https://${domain}:15268${plain}"
echo ""
echo -e "${green}若要登录前端网页使用【订阅转换】，请直接复制以上地址${plain}"
echo ""
echo -e "${green}接下来流程会进入〔Dui面板〕x-ui 菜单项${plain}"
sleep 8
echo ""
# --------- 返回菜单 ----------
show_menu
}

run_speedtest() {
    # Check if Speedtest is already installed
    if ! command -v speedtest &>/dev/null; then
        # If not installed, install it
        local pkg_manager=""
        local speedtest_install_script=""

        if command -v dnf &>/dev/null; then
            pkg_manager="dnf"
            speedtest_install_script="https://packagecloud.io/install/repositories/ookla/speedtest-cli/script.rpm.sh"
        elif command -v yum &>/dev/null; then
            pkg_manager="yum"
            speedtest_install_script="https://packagecloud.io/install/repositories/ookla/speedtest-cli/script.rpm.sh"
        elif command -v apt-get &>/dev/null; then
            pkg_manager="apt-get"
            speedtest_install_script="https://packagecloud.io/install/repositories/ookla/speedtest-cli/script.deb.sh"
        elif command -v apt &>/dev/null; then
            pkg_manager="apt"
            speedtest_install_script="https://packagecloud.io/install/repositories/ookla/speedtest-cli/script.deb.sh"
        fi

        if [[ -z $pkg_manager ]]; then
            echo "错误：找不到包管理器。 您可能需要手动安装 Speedtest"
            return 1
        else
            curl -s $speedtest_install_script | bash
            $pkg_manager install -y speedtest
        fi
    fi

    # Run Speedtest
    speedtest
}

open_all_firewall() {
    echo ""
    LOGI "正在根据系统类型放行全部防火墙规则..."

    case "${release}" in
    ubuntu|debian|armbian)
        if command -v ufw &>/dev/null; then
            LOGI "检测到 UFW 防火墙，正在关闭..."
            ufw --force disable >/dev/null 2>&1
            LOGI "UFW 防火墙已关闭，所有端口默认放行。"
        else
            LOGI "未检测到 UFW，改用 iptables 放行全部端口..."
            iptables -P INPUT ACCEPT  >/dev/null 2>&1
            iptables -P FORWARD ACCEPT >/dev/null 2>&1
            iptables -P OUTPUT ACCEPT >/dev/null 2>&1
            iptables -F >/dev/null 2>&1
            LOGI "iptables 已放行全部端口。"
        fi
        ;;
    centos|almalinux|rocky|oracle|fedora)
        if systemctl list-unit-files | grep -q firewalld; then
            LOGI "检测到 firewalld，正在停止并禁用..."
            systemctl stop firewalld >/dev/null 2>&1
            systemctl disable firewalld >/dev/null 2>&1
            LOGI "firewalld 已停止并禁用，所有端口默认放行。"
        else
            LOGI "未检测到 firewalld，改用 iptables 放行全部端口..."
            iptables -P INPUT ACCEPT  >/dev/null 2>&1
            iptables -P FORWARD ACCEPT >/dev/null 2>&1
            iptables -P OUTPUT ACCEPT >/dev/null 2>&1
            iptables -F >/dev/null 2>&1
            LOGI "iptables 已放行全部端口。"
        fi
        ;;
    arch|manjaro|alpine|opensuse*|*)
        LOGI "使用 iptables 放行全部端口..."
        iptables -P INPUT ACCEPT  >/dev/null 2>&1
        iptables -P FORWARD ACCEPT >/dev/null 2>&1
        iptables -P OUTPUT ACCEPT >/dev/null 2>&1
        iptables -F >/dev/null 2>&1
        LOGI "iptables 已放行全部端口。"
        ;;
    esac

    echo ""
    echo -e "${green}放行全部防火墙规则已执行完成！${plain}"
    echo ""
    before_show_menu
}

tcp_optimization() {
    echo ""
    LOGI "正在执行 TCP 网络调优脚本..."
    bash <(curl -sL https://raw.githubusercontent.com/yahuisme/network-optimization/main/script.sh)

    echo ""
    echo -e "${green}TCP 网络调优脚本已执行完毕，正在检测当前内核状态...${plain}"
    echo ""

    # 不看脚本退出码，直接根据内核参数判断是否启用成功
    local cc qdisc
    cc=$(sysctl -n net.ipv4.tcp_congestion_control 2>/dev/null)
    qdisc=$(sysctl -n net.core.default_qdisc 2>/dev/null)

    echo "当前拥塞控制算法: ${cc:-未知}"
    echo "当前队列调度算法: ${qdisc:-未知}"
    echo ""

    if [[ "$cc" == "bbr" && "$qdisc" == "fq" ]]; then
        echo -e "${green}检测结果：已成功启用 BBR + FQ 网络优化！${plain}"
    else
        echo -e "${red}检测结果：未检测到 BBR + FQ 生效，请检查上方脚本输出或手动检查配置。${plain}"
    fi

    echo ""
    before_show_menu
}

install_fail2ban2() {
    local fb_script="${XUI_RAW_BASE}/web/service/fb5.sh"
    LOGI "准备安装 Fail2ban（脚本：$fb_script）"

    read -rp "将执行远程脚本安装 Fail2ban（回车继续，输入 n 取消）: " _ans
    if [[ "${_ans}" =~ ^[nN]$ ]]; then
        LOGE "已取消"
        [[ $# == 0 ]] && before_show_menu
        return 0
    fi

    if bash <(curl -fsSL "$fb_script"); then
        LOGI "Fail2ban 安装脚本执行完成。"

        if command -v systemctl >/dev/null 2>&1; then
            systemctl enable --now fail2ban >/dev/null 2>&1 || true
            if systemctl is-active --quiet fail2ban; then
                LOGI "Fail2ban 服务：运行中"
            else
                LOGE "Fail2ban 服务：未运行（可执行：systemctl status fail2ban -l 查看原因）"
            fi
        else
            LOGI "未检测到 systemctl，已跳过服务自启/状态检查（可能不是 systemd 系统）。"
        fi

        [[ $# == 0 ]] && before_show_menu
        return 0
    else
        LOGE "Fail2ban 安装脚本执行失败。"
        [[ $# == 0 ]] && before_show_menu
        return 1
    fi
}





iplimit_main() {
    echo -e "\n${green}\t1.${plain} 安装 Fail2ban 并配置 IP 限制"
    echo -e "${green}\t2.${plain} 更改禁止期限"
    echo -e "${green}\t3.${plain} 解禁所有 IP"
    echo -e "${green}\t4.${plain} 查看日志"
    echo -e "${green}\t5.${plain} Fail2ban 状态"
    echo -e "${green}\t6.${plain} 重启 Fail2ban"
    echo -e "${green}\t7.${plain} 卸载 Fail2ban"
    echo -e "${green}\t0.${plain} 返回主菜单"
    read -p "请输入选项: " choice
    case "$choice" in
    0)
        show_menu
        ;;
    1)
        confirm "继续安装 Fail2ban 和 IP 限制?" "y"
        if [[ $? == 0 ]]; then
            install_iplimit
        else
            iplimit_main
        fi
        ;;
    2)
        read -rp "请输入新的禁令持续时间（以分钟为单位）[默认 30]: " NUM
        if [[ $NUM =~ ^[0-9]+$ ]]; then
            create_iplimit_jails ${NUM}
            systemctl restart fail2ban
        else
            echo -e "${red}${NUM} 不是一个数字！ 请再试一次.${plain}"
        fi
        iplimit_main
        ;;
    3)
        confirm "继续解除所有人的 IP 限制禁令?" "y"
        if [[ $? == 0 ]]; then
            fail2ban-client reload --restart --unban 3x-ipl
            truncate -s 0 "${iplimit_banned_log_path}"
            echo -e "${green}所有用户已成功解封${plain}"
            iplimit_main
        else
            echo -e "${yellow}已取消${plain}"
        fi
        iplimit_main
        ;;
    4)
        show_banlog
        ;;
    5)
        service fail2ban status
        ;;
    6)
        systemctl restart fail2ban
        ;;
    7)
        remove_iplimit
        ;;
    *) echo "无效选项" ;;
    esac
}

install_iplimit() {
    if ! command -v fail2ban-client &>/dev/null; then
        echo -e "${green}未安装 Fail2ban。正在安装...!${plain}\n"

        # Check the OS and install necessary packages
        case "${release}" in
        ubuntu)
            apt-get update
            if [[ "${os_version}" -ge 24 ]]; then
                apt-get install python3-pip -y
                python3 -m pip install pyasynchat --break-system-packages
            fi
            apt-get install fail2ban -y
            ;;
        debian)
            apt-get update
            if [ "$os_version" -ge 12 ]; then
                apt-get install -y python3-systemd
            fi
            apt-get install -y fail2ban
            ;;
        armbian)
            apt-get update && apt-get install fail2ban -y
            ;;
        centos | almalinux | rocky | oracle)
            yum update -y && yum install epel-release -y
            yum -y install fail2ban
            ;;
        fedora)
            dnf -y update && dnf -y install fail2ban
            ;;
        arch | manjaro | parch)
            pacman -Syu --noconfirm fail2ban
            ;;
        *)
            echo -e "${red}不支持的操作系统，请检查脚本并手动安装必要的软件包.${plain}\n"
            exit 1
            ;;
        esac

        if ! command -v fail2ban-client &>/dev/null; then
            echo -e "${red}Fail2ban 安装失败${plain}\n"
            exit 1
        fi

        echo -e "${green}Fail2ban 安装成功!${plain}\n"
    else
        echo -e "${yellow}Fail2ban 已安装${plain}\n"
    fi

    echo -e "${green}配置 IP 限制中...${plain}\n"

    # make sure there's no conflict for jail files
    iplimit_remove_conflicts

    # Check if log file exists
    if ! test -f "${iplimit_banned_log_path}"; then
        touch ${iplimit_banned_log_path}
    fi

    # Check if service log file exists so fail2ban won't return error
    if ! test -f "${iplimit_log_path}"; then
        touch ${iplimit_log_path}
    fi

    # Create the iplimit jail files
    # we didn't pass the bantime here to use the default value
    create_iplimit_jails

    # Launching fail2ban
    if ! systemctl is-active --quiet fail2ban; then
        systemctl start fail2ban
        systemctl enable fail2ban
    else
        systemctl restart fail2ban
    fi
    systemctl enable fail2ban

    echo -e "${green}IP 限制安装并配置成功!${plain}\n"
    before_show_menu
}

remove_iplimit() {
    echo -e "${green}\t1.${plain} 仅删除 IP 限制配置"
    echo -e "${green}\t2.${plain} 卸载 Fail2ban 和 IP 限制"
    echo -e "${green}\t0.${plain} 终止"
    read -p "请输入选项: " num
    case "$num" in
    1)
        rm -f /etc/fail2ban/filter.d/3x-ipl.conf
        rm -f /etc/fail2ban/action.d/3x-ipl.conf
        rm -f /etc/fail2ban/jail.d/3x-ipl.conf
        systemctl restart fail2ban
        echo -e "${green}IP 限制成功解除!${plain}\n"
        before_show_menu
        ;;
    2)
        rm -rf /etc/fail2ban
        systemctl stop fail2ban
        case "${release}" in
        ubuntu | debian | armbian)
            apt-get remove -y fail2ban
            apt-get purge -y fail2ban -y
            apt-get autoremove -y
            ;;
        centos | almalinux | rocky | oracle)
            yum remove fail2ban -y
            yum autoremove -y
            ;;
        fedora)
            dnf remove fail2ban -y
            dnf autoremove -y
            ;;
        arch | manjaro)
            pacman -Rns --noconfirm fail2ban
            ;;
        *)
            echo -e "${red}不支持的操作系统，请手动卸载 Fail2ban.${plain}\n"
            exit 1
            ;;
        esac
        echo -e "${green}Fail2ban 和 IP 限制已成功删除!${plain}\n"
        before_show_menu
        ;;
    0)
        echo -e "${yellow}已取消${plain}\n"
        iplimit_main
        ;;
    *)
        echo -e "${red}无效选项。 请选择一个有效的选项。${plain}\n"
        remove_iplimit
        ;;
    esac
}

show_banlog() {
    local system_log="/var/log/fail2ban.log"

    echo -e "${green}正在检查禁止日志...${plain}\n"

    if ! systemctl is-active --quiet fail2ban; then
        echo -e "${red}Fail2ban 服务未运行！${plain}\n"
        return 1
    fi

    if [[ -f "$system_log" ]]; then
        echo -e "${green}来自 fail2ban.log 的最近系统禁止活动:${plain}"
        grep "3x-ipl" "$system_log" | grep -E "Ban|Unban" | tail -n 10 || echo -e "${yellow}未发现近期系统禁止活动${plain}"
        echo ""
    fi

    if [[ -f "${iplimit_banned_log_path}" ]]; then
        echo -e "${green}3X-IPL禁止日志文件条目:${plain}"
        if [[ -s "${iplimit_banned_log_path}" ]]; then
            grep -v "INIT" "${iplimit_banned_log_path}" | tail -n 10 || echo -e "${yellow}未找到禁止条目${plain}"
        else
            echo -e "${yellow}禁止日志文件为空${plain}"
        fi
    else
        echo -e "${red}未找到禁止日志文件: ${iplimit_banned_log_path}${plain}"
    fi

    echo -e "\n${green}目前的限制情况:${plain}"
    fail2ban-client status 3x-ipl || echo -e "${yellow}无法获取限制状态${plain}"
}

create_iplimit_jails() {
    # Use default bantime if not passed => 30 minutes
    local bantime="${1:-30}"

    # Uncomment 'allowipv6 = auto' in fail2ban.conf
    sed -i 's/#allowipv6 = auto/allowipv6 = auto/g' /etc/fail2ban/fail2ban.conf

    # On Debian 12+ fail2ban's default backend should be changed to systemd
    if [[  "${release}" == "debian" && ${os_version} -ge 12 ]]; then
        sed -i '0,/action =/s/backend = auto/backend = systemd/' /etc/fail2ban/jail.conf
    fi

    cat << EOF > /etc/fail2ban/jail.d/3x-ipl.conf
[3x-ipl]
enabled=true
backend=auto
filter=3x-ipl
action=3x-ipl
logpath=${iplimit_log_path}
maxretry=2
findtime=32
bantime=${bantime}m
EOF

    cat << EOF > /etc/fail2ban/filter.d/3x-ipl.conf
[Definition]
datepattern = ^%%Y/%%m/%%d %%H:%%M:%%S
failregex   = \[LIMIT_IP\]\s*Email\s*=\s*<F-USER>.+</F-USER>\s*\|\|\s*SRC\s*=\s*<ADDR>
ignoreregex =
EOF

    cat << EOF > /etc/fail2ban/action.d/3x-ipl.conf
[INCLUDES]
before = iptables-allports.conf

[Definition]
actionstart = <iptables> -N f2b-<name>
              <iptables> -A f2b-<name> -j <returntype>
              <iptables> -I <chain> -p <protocol> -j f2b-<name>

actionstop = <iptables> -D <chain> -p <protocol> -j f2b-<name>
             <actionflush>
             <iptables> -X f2b-<name>

actioncheck = <iptables> -n -L <chain> | grep -q 'f2b-<name>[ \t]'

actionban = <iptables> -I f2b-<name> 1 -s <ip> -j <blocktype>
            echo "\$(date +"%%Y/%%m/%%d %%H:%%M:%%S")   BAN   [Email] = <F-USER> [IP] = <ip> banned for <bantime> seconds." >> ${iplimit_banned_log_path}

actionunban = <iptables> -D f2b-<name> -s <ip> -j <blocktype>
              echo "\$(date +"%%Y/%%m/%%d %%H:%%M:%%S")   UNBAN   [Email] = <F-USER> [IP] = <ip> unbanned." >> ${iplimit_banned_log_path}

[Init]
name = default
protocol = tcp
chain = INPUT
EOF

    echo -e "${green}创建的 IP Limit 限制文件禁止时间为 ${bantime} 分钟。${plain}"
}

iplimit_remove_conflicts() {
    local jail_files=(
        /etc/fail2ban/jail.conf
        /etc/fail2ban/jail.local
    )

    for file in "${jail_files[@]}"; do
        # Check for [3x-ipl] config in jail file then remove it
        if test -f "${file}" && grep -qw '3x-ipl' ${file}; then
            sed -i "/\[3x-ipl\]/,/^$/d" ${file}
            echo -e "${yellow}消除系统环境中 [3x-ipl] 的冲突 (${file})!${plain}\n"
        fi
    done
}

# 【中文注释】：这是新添加的“网页版SSH工具”安装函数
# 1. 自动从 GitHub API 获取最新版本
# 2. 完整移植了 .go 文件中的 UFW、Nginx 和 systemd 逻辑
install_sshwifty() {
    echo "=== 开始安装 Sshwifty (网页版SSH) ==="
    
    # 0.a 提取 Go 里的域名获取逻辑
    # 【中文注释】：首先检查面板是否已配置 SSL 证书，以便获取域名
    local existing_cert=$(/usr/local/x-ui/x-ui setting -getCert true | grep -Eo 'cert: .+' | awk '{print $2}')
    local existing_key=$(/usr/local/x-ui/x-ui setting -getCert true | grep -Eo 'key: .+' | awk '{print $2}')
    local domain=""
    local ACME_PATH=""

    if [[ -n "$existing_cert" && -n "$existing_key" ]]; then
        domain=$(basename "$(dirname "$existing_cert")")
        # 【中文注释】：从 .go 文件中获取 acme 路径逻辑
        ACME_PATH="/root/.acme.sh/${domain}_ecc"
        LOGI "获取到用于 Nginx 配置的域名: ${domain}"
        LOGI "将使用证书路径: ${ACME_PATH}"
    else
        LOGE "Sshwifty 安装失败：无法获取面板域名以配置 Nginx。"
        LOGE "请先为面板申请 SSL 证书 (菜单项 18 或 19)。"
        return 1 # 中止安装
    fi
    
    # 【中文注释】：检查证书文件是否存在，这是 .go 文件中的关键检查
    local CERT_FILE="${ACME_PATH}/fullchain.cer"
    local KEY_FILE="${ACME_PATH}/${domain}.key"
    if [ ! -f "$CERT_FILE" ] || [ ! -f "$KEY_FILE" ]; then
        LOGE "⚠️ **警告：Nginx 配置失败**"
        LOGE "错误: 找不到 Nginx 所需的 SSL 证书！"
        LOGE "检查路径: ${CERT_FILE}"
        LOGE "检查路径: ${KEY_FILE}"
        LOGE "Sshwifty 服务将安装，但 Nginx 反代将失败。"
        # 【中文注释】：我们继续安装，但 Nginx 会跳过
    fi

    # 0.b 提取 Go 里的 UFW (端口放行) 逻辑
    LOGI "正在为 Sshwifty 服务配置防火墙..."
    PORTS_TO_OPEN="8182 8188" # Sshwifty 自身端口和 Nginx 代理端口
    DEFAULT_PORTS="22 80 443 13688 8443" # 面板运行所需的基础端口
    
    if ! command -v ufw &>/dev/null; then
        LOGI "ufw 防火墙未安装，正在安装..."
        DEBIAN_FRONTEND=noninteractive apt-get update -qq >/dev/null
        DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ufw >/dev/null
        if [ $? -ne 0 ]; then LOGE "ufw 安装失败或权限不足。"; return 1; fi
    fi

    LOGI "正在检查并放行基础服务端口: $DEFAULT_PORTS"
    for p in $DEFAULT_PORTS; do
        if ! ufw status | grep -qw "$p/tcp"; then
            LOGI "端口 $p/tcp 未放行，正在添加规则..."
            ufw allow $p/tcp >/dev/null
            if [ $? -ne 0 ]; then LOGE "ufw 端口 $p 放行失败。"; return 1; fi
        fi
    done
    LOGI "基础服务端口检查完毕。"

    LOGI "正在检查并放行 Sshwifty 服务端口: $PORTS_TO_OPEN"
    for port in $PORTS_TO_OPEN; do
        if ! ufw status | grep -qw "$port"; then
            LOGI "正在执行 ufw allow $port..."
            ufw allow $port >/dev/null
            if [ $? -ne 0 ]; then LOGE "ufw 端口 $port 放行失败。"; return 1; fi
        fi
    done

    if ! ufw status | grep -q "Status: active"; then
        LOGI "ufw 状态：未激活。正在尝试激活..."
        ufw --force enable
        if [ $? -ne 0 ]; then LOGE "ufw 激活失败。"; return 1; fi
    fi
    LOGI "✅ 所有端口 ($DEFAULT_PORTS $PORTS_TO_OPEN) 已成功放行/检查。"

    # --- 从这里开始是 Go 脚本中的 shellCommand ---
    
    # 0. 检查权限和停止旧服务
    if [ "$EUID" -ne 0 ]; then
        LOGE "致命错误：安装脚本必须以 root 权限运行！"
        return 1
    fi

    if systemctl is-active --quiet sshwifty; then
        LOGI "检测到正在运行的 Sshwifty 服务，正在停止..."
        systemctl stop sshwifty
    fi

    # 1. 安装依赖
    LOGI "正在安装依赖: curl, wget, tar, jq..."
    # 【中文注释】：增加了 jq，用于更可靠地修改 json 配置文件
    DEPENDENCIES="curl wget tar jq"
    UNMET_DEPS=""
    for dep in $DEPENDENCIES; do
        if ! command -v $dep &>/dev/null; then 
            UNMET_DEPS="$UNMET_DEPS $dep"
        fi
    done

    if [ -n "$UNMET_DEPS" ]; then
        LOGI "【发现缺失依赖】: $UNMET_DEPS，尝试自动安装..."
        if command -v apt-get &>/dev/null; then
        DEBIAN_FRONTEND=noninteractive apt-get update -qq >/dev/null
            DEBIAN_FRONTEND=noninteractive apt-get install -y -qq $UNMET_DEPS
            if [ $? -ne 0 ]; then LOGE "错误：使用 apt-get 安装依赖 $UNMET_DEPS 失败！"; return 1; fi
        elif command -v yum &>/dev/null; then
            yum install -y -q $UNMET_DEPS
            if [ $? -ne 0 ]; then LOGE "错误：使用 yum 安装依赖 $UNMET_DEPS 失败！"; return 1; fi
        else
            LOGE "致命错误: 无法自动安装依赖 ($UNMET_DEPS)。请手动安装后再试。"
            return 1
        fi
    fi
    LOGI "依赖检查与安装完成。"

    # 1.b 安装 Nginx
    if ! command -v nginx &>/dev/null; then
        LOGI "未检测到 Nginx，正在安装..."
        if command -v apt-get &>/dev/null; then
            DEBIAN_FRONTEND=noninteractive apt-get update -qq >/dev/null
            DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nginx
            if [ $? -ne 0 ]; then LOGE "错误：使用 apt-get 安装 Nginx 失败！"; return 1; fi
        elif command -v yum &>/dev/null; then
            yum install -y -q nginx
            if [ $? -ne 0 ]; then LOGE "错误：使用 yum 安装 Nginx 失败！"; return 1; fi
        else
            LOGE "错误: 无法自动安装 nginx。请手动安装后再试。"
            return 1
        fi
        systemctl enable nginx
        systemctl start nginx
    else
        LOGI "检测到 Nginx 已安装，跳过安装步骤。"
    fi

    # 2. 【重大更新】：自动获取最新版本
    LOGI "正在从 GitHub API 获取 Sshwifty 最新版本..."
    DOWNLOAD_URL=$(curl -s "https://api.github.com/repos/nirui/sshwifty/releases/latest" | grep "browser_download_url" | grep "linux_amd64.tar.gz" | cut -d '"' -f 4 | head -n 1)

    if [ -z "$DOWNLOAD_URL" ]; then
        LOGE "致命错误: 无法自动获取最新的 Sshwifty 下载地址。"
        LOGE "请检查您的 VPS 是否能访问 GitHub API (api.github.com)。"
        return 1
    fi
    
    FILENAME=$(basename "$DOWNLOAD_URL")
    LOGI "获取到最新下载地址: $DOWNLOAD_URL"
    TEMP_DIR="/tmp/sshwifty_install_$$"

    # 3. 下载并解压
    LOGI "正在下载 $FILENAME..."
    rm -rf $TEMP_DIR
    mkdir -p $TEMP_DIR
    # 【中文注释】：使用 curl -fsL 替代 wget，-f 失败时静默退出，-s 静默，-L 跟随跳转
    curl -fsL -o "$TEMP_DIR/$FILENAME" $DOWNLOAD_URL
    if [ $? -ne 0 ]; then 
        LOGE "致命错误: 下载 Sshwifty 失败！"
        rm -rf $TEMP_DIR
        return 1
    fi

    LOGI "正在解压文件..."
    tar -xzvf "$TEMP_DIR/$FILENAME" -C $TEMP_DIR
    if [ $? -ne 0 ]; then 
        LOGE "致命错误: 解压 Sshwifty 失败。文件可能已损坏。"
        rm -rf $TEMP_DIR
        return 1
    fi

    # 4. 复制可执行文件并授权
    INSTALL_PATH="/usr/local/bin/sshwifty"
    LOGI "正在安装可执行文件到 $INSTALL_PATH..."
    # 【中文注释】：在解压目录中查找可执行文件
    BINARY_FILE=$(find $TEMP_DIR -type f \( -name "sshwifty_linux_amd64" -o -name "sshwifty" \) | head -n 1)

    if [ ! -f "$BINARY_FILE" ]; then
        LOGE "致命错误: 在解压目录中未找到 Sshwifty 可执行文件 ($TEMP_DIR)。"
        rm -rf $TEMP_DIR
        return 1
    fi

    cp "$BINARY_FILE" "$INSTALL_PATH"
    chmod +x "$INSTALL_PATH"
    LOGI "安装成功: $INSTALL_PATH"

    # 5. 生成默认配置文件 (保留 IP 修复逻辑)
    CONFIG_PATH="/etc/sshwifty.conf.json"
    if [ ! -f "$CONFIG_PATH" ]; then
        LOGI "正在创建默认配置文件: $CONFIG_PATH..."
        # 【中文注释】：在解压目录中查找示例配置文件
        EXAMPLE_CONFIG=$(find $TEMP_DIR -type f -name "*sshwifty.conf.example.json*" | head -n 1)
        if [ -f "$EXAMPLE_CONFIG" ]; then
            cp "$EXAMPLE_CONFIG" "$CONFIG_PATH"
        else
            # 【中文注释】：如果找不到，创建一个最小配置
            cat <<EOF > $CONFIG_PATH
{
    "Host": "127.0.0.1", 
    "Port": 8182,
    "Debug": false,
    "Title": "Sshwifty Web SSH"
}
EOF
        fi
        # 【中文注释】：确保 Host 正确设置为 127.0.0.1
        jq '.Host = "127.0.0.1"' $CONFIG_PATH > $CONFIG_PATH.tmp && mv $CONFIG_PATH.tmp $CONFIG_PATH
        LOGI "默认配置文件已创建并修正。"
    else
        LOGI "配置文件 $CONFIG_PATH 已存在，跳过创建。"
        # 【中文注释】：确保 Host 正确设置为 127.0.0.1
        jq '.Host = "127.0.0.1"' $CONFIG_PATH > $CONFIG_PATH.tmp && mv $CONFIG_PATH.tmp $CONFIG_PATH
        LOGI "确保 $CONFIG_PATH 监听 127.0.0.1 (使用 jq)"
    fi

    # 6. 创建 systemd 服务文件
    SERVICE_PATH="/etc/systemd/system/sshwifty.service"
    LOGI "正在创建 systemd 服务: $SERVICE_PATH..."
    cat <<EOF > $SERVICE_PATH
[Unit]
Description=Sshwifty - Web SSH Client
After=network.target

[Service]
Type=simple
ExecStart=$INSTALL_PATH --config $CONFIG_PATH
User=root
Restart=on-failure
RestartSec=5
KillMode=process

[Install]
WantedBy=multi-user.target
EOF

    # 7. 重新加载、启用并启动服务
    LOGI "正在配置 systemd 服务 (开机自启)..."
    systemctl daemon-reload
    systemctl enable sshwifty
    systemctl restart sshwifty

    # 8. 清理临时文件
    LOGI "正在清理临时文件..."
    rm -rf $TEMP_DIR

    # 9. 检查服务状态
    if ! systemctl is-active --quiet sshwifty; then
        LOGE "致命错误: Sshwifty 服务启动失败！请检查 8182 端口是否被占用，或查看以下日志："
        journalctl -u sshwifty -n 20 --no-pager
        return 1
    fi
    LOGI "✅ Sshwifty (8182 端口) 启动成功。"

    # 10. 配置 Nginx 反向代理
    LOGI "=== 开始配置 Nginx 反向代理 (8188 -> 8182) ==="
    DOMAIN="${domain}" # <--- 这里使用我们之前获取的变量

    if [ ! -f "$CERT_FILE" ] || [ ! -f "$KEY_FILE" ]; then
        LOGE "⚠️ **警告：Nginx 配置跳过**"
        LOGE "错误: 找不到 Nginx 所需的 SSL 证书！Sshwifty 服务已运行在 127.0.0.1:8182。"
    else
        # 【中文注释】：移植 Go 代码中的证书复制逻辑
        mkdir -p /etc/nginx/ssl
        cp "${CERT_FILE}" "/etc/nginx/ssl/${DOMAIN}.cer"
        cp "${KEY_FILE}" "/etc/nginx/ssl/${DOMAIN}.key"

        cat > /etc/nginx/conf.d/sshwifty-proxy.conf <<EOF
# --- Sshwifty (Web SSH) 反向代理配置 ---
server {
    listen 8188 ssl http2;
    server_name ${DOMAIN};
    # 【中文注释】：使用复制后的证书路径
    ssl_certificate       /etc/nginx/ssl/${DOMAIN}.cer;
    ssl_certificate_key /etc/nginx/ssl/${DOMAIN}.key;
    ssl_protocols TLSv1.2 TLSv1.3;
    location / {
        proxy_pass http://127.0.0.1:8182;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    }
}
EOF
        LOGI "正在重载 Nginx..."
        if ! systemctl reload nginx; then
            LOGE "错误: Nginx 重载失败。请检查配置。"
        fi
        LOGI "✅ Nginx 反向代理配置成功。"
    fi

    LOGI "=== Sshwifty 安装并启动成功! ==="
    
    # 【中文注释】：安装完成后向用户显示最终访问信息
    echo ""
    echo -e "${green}🎉 恭喜！【网页版SSH】模块已成功安装！${plain}"
    echo ""
    echo -e "${green}已自动为您配置 Nginx 反向代理。${plain}"
    echo ""
    echo -e "${green}您现在可以使用以下地址访问 Web 界面：${plain}"
    echo ""
    echo -e "🔗 ${yellow}登录地址: https://${DOMAIN}:8188${plain}"
    echo ""
    echo -e "${green}默认登录口令为： ${yellow}WEB_ACCESS_PASSWORD${plain}"
    echo ""
    echo -e "${green}(如果无法登录，请在 /etc/sshwifty.conf.json 中查看或设置口令)${plain}"
    echo ""
    
    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

# 【中文注释】：这是新添加的“线路和IP检测”相关函数
ip_check() {
    # 【中文注释】：功能 1：检测 IP
    bash <(curl -Ls https://Check.Place) -I
    before_show_menu
}

route_check() {
    # 【中文注释】：功能 2：检测线路
    bash <(curl -Ls https://Check.Place) -N
    before_show_menu
}

check_place_menu() {
    # 【中文注释】：这是选项 27 的子菜单
    echo -e "
  ${green}线路和IP检测 (Check.Place)${plain}
  
  ${green}1.${plain} 检测 IP 质量
  ${green}2.${plain} 检测 线路/路由
  ——————————————————————
  ${green}0.${plain} 返回主菜单
  "
    read -p "请输入选项 [0-2]: " num
    case "${num}" in
    0)
        show_menu
        ;;
    1)
        ip_check
        ;;
    2)
        route_check
        ;;
    *)
        LOGE "请输入正确的数字选项 [0-2]!"
        ;;
    esac
}

# 【中文注释】：这是新添加的“服务器DNS检测”函数
dns_check() {
    echo ""
    # 【中文注释】：执行 服务器 DNS 检测脚本
    bash <(curl -Ls https://raw.githubusercontent.com/shini74744/jj/refs/heads/main/dnsxz.sh)
    
    # 【中文注释】：检查是否是作为非交互式命令调用的
    # 如果 $# (参数个数) 为 0，说明是从菜单调用的，执行完后返回主菜单
    if [[ $# == 0 ]]; then
        before_show_menu
    fi
    # 【中文注释】：如果参数个数不为 0 (例如 '0')，说明是命令行调用的，函数结束
}

show_usage() {
    echo -e "         ---------------------"
    echo -e "         |${green}Dui 控制菜单用法 ${plain}|${plain}"
    echo -e "         |  ${yellow}一个更好的面板   ${plain}|${plain}"   
    echo -e "         | ${yellow}基于Xray Core构建 ${plain}|${plain}"  
    echo -e "--------------------------------------------"
    echo -e "x-ui              - 进入管理脚本"
    echo -e "x-ui start        - 启动 Dui 面板"
    echo -e "x-ui stop         - 关闭 Dui 面板"
    echo -e "x-ui restart      - 重启 Dui 面板"
    echo -e "x-ui status       - 查看 Dui 状态"
    echo -e "x-ui settings     - 查看当前设置信息"
    echo -e "x-ui enable       - 启用 Dui 开机启动"
    echo -e "x-ui disable      - 禁用 Dui 开机启动"
    echo -e "x-ui log          - 查看 Dui 运行日志"
    echo -e "x-ui banlog       - 检查 Fail2ban 禁止日志"
    echo -e "x-ui update       - 更新 Dui 面板"
    echo -e "x-ui custom       - 自定义 Dui 版本"
    echo -e "x-ui install      - 安装 Dui 面板"
    echo -e "x-ui uninstall    - 卸载 Dui 面板"
    echo -e "--------------------------------------------"
}

show_menu() {
    echo -e "
——————————————————————
  ${green}大大怪 面板管理脚本${plain}
  ${yellow}  一个更好的面板${plain}
  ${yellow} 基于Xray Core构建${plain}
——————————————————————
  ${green}0.${plain} 退出脚本
  ${green}1.${plain} 安装面板
  ${green}2.${plain} 更新面板
  ${green}3.${plain} 更新菜单项
  ${green}4.${plain} 自定义版本
  ${green}5.${plain} 卸载面板
——————————————————————
  ${green}6.${plain} 重置用户名、密码
  ${green}7.${plain} 修改访问路径
  ${green}8.${plain} 重置面板设置
  ${green}9.${plain} 修改面板端口
  ${green}10.${plain} 查看面板设置
——————————————————————
  ${green}11.${plain} 启动面板
  ${green}12.${plain} 关闭面板
  ${green}13.${plain} 重启面板
  ${green}14.${plain} 检查面板状态
  ${green}15.${plain} 检查面板日志
——————————————————————
  ${green}16.${plain} 启用开机启动
  ${green}17.${plain} 禁用开机启动
——————————————————————
  ${green}18.${plain} SSL 证书管理
  ${green}19.${plain} CF SSL 证书
  ${green}20.${plain} IP 限制管理
  ${green}21.${plain} 防火墙管理
——————————————————————
  ${green}22.${plain} 启用 BBR 
  ${green}23.${plain} 更新 Geo 文件
  ${green}24.${plain} Speedtest by Ookla
  ${green}25.${plain} 安装订阅转换
  ${green}26.${plain} 网页版SSH工具
  ${green}27.${plain} 线路和IP检测
  ${green}28.${plain} 服务器DNS检测
  ${green}29.${plain} 放行全部防火墙
  ${green}30.${plain} TCP 网络调优
  ${green}31.${plain} 安装 Fail2ban
  ${green}32.${plain} 流量消耗统计
——————————————————————
  ${green}若在使用过程中有任何问题请联系业务人员${plain}
  ${yellow}Dui${plain}
  ${red}仅限项目内部流通 ${pink}禁止外流使用${plain}
  ${green}〔Dui面板〕项目地址${plain}
  ${yellow}https://github.com/shini74744/Dui${plain}
  ${green}详细〔安装配置〕教程${plain}
  ${yellow}https://github.com/shini74744/Dui${plain}
——————————————————————

  ${green}探针监控面板地址${plain}
  ${red}https://nmsl.ma${plain}
----------------------------------------------
"
    show_status
    echo && read -p "请输入选项 [0-32]: " num


    case "${num}" in
    0)
        exit 0
        ;;
    1)
        check_uninstall && install
        ;;
    2)
        check_install && update
        ;;
    3)
        check_install && update_menu
        ;;
    4)
        check_install && custom_version
        ;;
    5)
        check_install && uninstall
        ;;
    6)
        check_install && reset_user
        ;;
    7)
        check_install && reset_webbasepath
        ;;
    8)
        check_install && reset_config
        ;;
    9)
        check_install && set_port
        ;;
    10)
        check_install && check_config
        ;;
    11)
        check_install && start
        ;;
    12)
        check_install && stop
        ;;
    13)
        check_install && restart
        ;;
    14)
        check_install && status
        ;;
    15)
        check_install && show_log
        ;;
    16)
        check_install && enable
        ;;
    17)
        check_install && disable
        ;;
    18)
        ssl_cert_issue_main
        ;;
    19)
        ssl_cert_issue_CF
        ;;
    20)
        iplimit_main
        ;;
    21)
        firewall_menu
        ;;
    22)
        bbr_menu
        ;;
    23)
        update_geo
        ;;
    24)
        run_speedtest
        ;;
    25)
        subconverter
        ;;
    26)
        check_install && install_sshwifty
        ;;
    27)
        check_install && check_place_menu
        ;;
    28)
        check_install && dns_check
        ;;
    29)
        open_all_firewall
        ;;
    30)
        tcp_optimization
        ;;
    31)
        install_fail2ban2
        ;;
    32)
        traffic_usage
        ;;
    *)
        LOGE "请输入正确的数字选项 [0-32]"
        ;;
    esac
}




# Keep release snapshots stable by default. Set XUI_AUTO_UPDATE_MENU=1 to opt in
# to tracking the branch copy of x-ui.sh automatically. Menu option 3 remains
# available for an explicit menu-only update.
if [[ "${XUI_AUTO_UPDATE_MENU:-0}" == "1" ]]; then
    update_shell || true
fi

if [[ $# > 0 ]]; then
    case $1 in
    "start")
        check_install 0 && start 0
        ;;
    "stop")
        check_install 0 && stop 0
        ;;
    "restart")
        check_install 0 && restart 0
        ;;
    "status")
        check_install 0 && status 0
        ;;
    "settings")
        check_install 0 && check_config 0
        ;;
    "enable")
        check_install 0 && enable 0
        ;;
    "disable")
        check_install 0 && disable 0
        ;;
    "log")
        check_install 0 && show_log 0
        ;;
    "banlog")
        check_install 0 && show_banlog 0
        ;;
    "update")
        check_install 0 && update 0
        ;;
    "custom")
        check_install 0 && custom_version 0
        ;;
    "install")
        check_uninstall 0 && install 0
        ;;
    "uninstall")
        check_install 0 && uninstall 0
        ;;
    "subconverter")
        check_install 0 && subconverter 0
        ;;
    "sshwifty")
        check_install 0 && install_sshwifty 0 
        ;;
    "dnscheck")
        check_install 0 && dns_check 0
        ;;    
    *) show_usage ;;
    esac
else
    show_menu
fi
