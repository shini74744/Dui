#!/bin/bash

red='\033[0;31m'
green='\033[0;32m'
blue='\033[0;34m'
yellow='\033[0;33m'
plain='\033[0m'

# Unified project source: installer, menu, service and release binary all live here.
XUI_REPO="${XUI_REPO:-shini74744/Dui}"
XUI_BRANCH="${XUI_BRANCH:-main}"
XUI_RAW_BASE="https://raw.githubusercontent.com/${XUI_REPO}/${XUI_BRANCH}"
XUI_RELEASE_BASE="https://github.com/${XUI_REPO}/releases"

cur_dir=$(pwd)

# 原子替换菜单脚本。安装器可能由正在运行的 /usr/bin/x-ui 菜单调用，
# 不能原地截断该文件，否则当前 bash 可能继续读取到被改写后的内容并发生执行流错乱。
install_menu_script_atomic() {
    local source="$1"
    local dest="/usr/bin/x-ui"
    local staged="/usr/bin/.x-ui.new.$$.${RANDOM}"

    rm -f "$staged"
    if ! install -m 0755 "$source" "$staged"; then
        rm -f "$staged"
        return 1
    fi
    if ! mv -f "$staged" "$dest"; then
        rm -f "$staged"
        return 1
    fi
    return 0
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
    echo ""
    echo -e "${red}检查服务器操作系统失败，请联系作者!${plain}" >&2
    exit 1
fi
echo ""
echo -e "${green}---------->>>>>目前服务器的操作系统为: $release${plain}"

dui_latest_version() {
    local tag
    tag=$(curl -fsSL --retry 2 --connect-timeout 10 --max-time 20 "${XUI_RELEASE_BASE}.atom" 2>/dev/null \
        | sed -n 's#.*<title>\(v[^<]*\)</title>.*#\1#p' \
        | head -n 1)
    if [[ "$tag" == v* ]]; then
        printf '%s\n' "$tag"
        return 0
    fi
    return 1
}

arch() {
    case "$(uname -m)" in
        x86_64 | x64 | amd64 ) echo 'amd64' ;;
        i*86 | x86 ) echo '386' ;;
        armv8* | armv8 | arm64 | aarch64 ) echo 'arm64' ;;
        armv7* | armv7 | arm ) echo 'armv7' ;;
        armv6* | armv6 ) echo 'armv6' ;;
        armv5* | armv5 ) echo 'armv5' ;;
        s390x) echo 's390x' ;;
        *) echo -e "${green}不支持的CPU架构! ${plain}" && rm -f install.sh && exit 1 ;;
    esac
}

echo ""
echo -e "${yellow}---------->>>>>当前系统的架构为: $(arch)${plain}"
echo ""

# 获取 GitHub 最新 Release 版本（读取公开 Releases Atom Feed，不依赖 API 配额）
last_version=$(dui_latest_version || true)

# 获取 x-ui 版本
xui_version=$(/usr/local/x-ui/x-ui -v 2>/dev/null)

# 检查 xui_version 是否为空
if [[ -z "$xui_version" ]]; then
    echo ""
    echo -e "${red}------>>>当前服务器没有安装任何 x-ui 系列代理面板${plain}"
    echo ""
    echo -e "${green}-------->>>>片刻之后脚本将会自动引导安装〔Dui面板〕${plain}"
else
    # 检查版本号中是否包含冒号
    if [[ "$xui_version" == *:* ]]; then
        echo -e "${green}---------->>>>>当前代理面板的版本为: ${red}其他 x-ui 分支版本${plain}"
        echo ""
        echo -e "${green}-------->>>>片刻之后脚本将会自动引导安装〔Dui面板〕${plain}"
    else
        echo -e "${green}---------->>>>>当前 Dui 版本：${red}v${xui_version}${plain}"
    fi
fi
echo ""
echo -e "${yellow}---------------------->>>>> Dui GitHub 最新 Release 版本：${red}${last_version}${plain}"
sleep 4

os_version=$(grep -i version_id /etc/os-release | cut -d \" -f2 | cut -d . -f1)

if [[ "${release}" == "arch" ]]; then
    echo "您的操作系统是 ArchLinux"
elif [[ "${release}" == "manjaro" ]]; then
    echo "您的操作系统是 Manjaro"
elif [[ "${release}" == "armbian" ]]; then
    echo "您的操作系统是 Armbian"
elif [[ "${release}" == "alpine" ]]; then
    echo "您的操作系统是 Alpine Linux"
elif [[ "${release}" == "opensuse-tumbleweed" ]]; then
    echo "您的操作系统是 OpenSUSE Tumbleweed"
elif [[ "${release}" == "centos" ]]; then
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
    echo "- Manjaro"
    echo "- Armbian"
    echo "- Alpine Linux"
    echo "- AlmaLinux 9+"
    echo "- Rocky Linux 9+"
    echo "- Oracle Linux 8+"
    echo "- OpenSUSE Tumbleweed"
    exit 1
fi

install_base() {
    case "${release}" in
    ubuntu | debian | armbian)
        apt-get update && apt-get install -y -q wget curl sudo tar tzdata
        ;;
    centos | rhel | almalinux | rocky | ol)
        yum -y --exclude=kernel* update && yum install -y -q wget curl sudo tar tzdata
        ;;
    fedora | amzn | virtuozzo)
        dnf -y --exclude=kernel* update && dnf install -y -q wget curl sudo tar tzdata
        ;;
    arch | manjaro | parch)
        pacman -Sy && pacman -S --noconfirm wget curl sudo tar tzdata
        ;;
    alpine)
        apk update && apk add --no-cache wget curl sudo tar tzdata
        ;;
    opensuse-tumbleweed)
        zypper refresh && zypper -q install -y wget curl sudo tar timezone
        ;;
    *)
        apt-get update && apt-get install -y -q wget curl sudo tar tzdata
        ;;
    esac
}

gen_random_string() {
    local length="$1"
    local random_string=$(LC_ALL=C tr -dc 'a-zA-Z0-9' </dev/urandom | fold -w "$length" | head -n 1)
    echo "$random_string"
}

# 去除面板 CLI 输出中的 ANSI 颜色码，避免读取路径时混入转义字符。
strip_ansi() {
    sed $'s/\033\\[[0-9;]*[[:alpha:]]//g'
}

# 覆盖旧程序前读取当前面板访问路径。
# "/" 表示直接使用 IP/域名 + 端口；只有真正读取失败时才不输出内容。
get_existing_web_base_path() {
    [[ -x /usr/local/x-ui/x-ui ]] || return 1

    local line base_path
    line=$(/usr/local/x-ui/x-ui setting -show true 2>/dev/null \
        | strip_ansi \
        | grep -m1 'webBasePath（访问路径）:' || true)
    [[ -n "$line" ]] || return 1

    base_path="${line#*webBasePath（访问路径）:}"
    base_path="${base_path#"${base_path%%[![:space:]]*}"}"
    base_path="${base_path%"${base_path##*[![:space:]]}"}"

    # 老版本若显示空路径，其实际含义就是根路径。
    [[ -n "$base_path" ]] || base_path="/"
    [[ "$base_path" == /* ]] || base_path="/$base_path"
    [[ "$base_path" == */ ]] || base_path="$base_path/"
    printf '%s' "$base_path"
}

# 安装/更新后配置
config_after_install() {
    local is_upgrade="${1:-false}"

    # 覆盖升级不再进入重新设置路径流程，所有旧设置原样保留。
    if [[ "$is_upgrade" == "true" ]]; then
        echo ""
        echo -e "${green}检测到已有 Dui/x-ui 数据库：本次为覆盖升级。${plain}"
        echo -e "${green}保留原账号、密码、端口、证书和面板访问路径，不添加 /shlii/。${plain}"
        echo ""
        /usr/local/x-ui/x-ui migrate
        return
    fi

    echo -e "${yellow}全新安装完成！ 为了您的面板安全，建议修改面板设置 ${plain}"
    echo ""
    read -p "$(echo -e "${green}想继续修改吗？${plain} [y/n]？--->>请输入：")" config_confirm
    if [[ "${config_confirm}" == "y" || "${config_confirm}" == "Y" ]]; then
        read -p "请设置您的用户名: " config_account
        echo -e "${yellow}您的用户名将是: ${config_account}${plain}"
        read -p "请设置您的密码: " config_password
        echo -e "${yellow}您的密码将是: ${config_password}${plain}"
        read -p "请设置面板端口: " config_port
        echo -e "${yellow}您的面板端口号为: ${config_port}${plain}"
        read -p "请设置面板登录访问路径（回车默认 shlii）: " config_webBasePath
        [[ -n "${config_webBasePath}" ]] || config_webBasePath="shlii"
        echo -e "${yellow}您的面板访问路径为: ${config_webBasePath}${plain}"
        echo -e "${yellow}正在初始化，请稍候...${plain}"
        /usr/local/x-ui/x-ui setting -username "${config_account}" -password "${config_password}"
        echo -e "${yellow}用户名和密码设置成功!${plain}"
        /usr/local/x-ui/x-ui setting -port "${config_port}"
        echo -e "${yellow}面板端口号设置成功!${plain}"
        /usr/local/x-ui/x-ui setting -webBasePath "${config_webBasePath}"
        echo -e "${yellow}面板登录访问路径设置成功!${plain}"
        echo ""
    else
        echo ""
        sleep 1
        local usernameTemp=$(head -c 10 /dev/urandom | base64)
        local passwordTemp=$(head -c 10 /dev/urandom | base64)
        local webBasePathTemp="shlii"
        /usr/local/x-ui/x-ui setting -username "${usernameTemp}" -password "${passwordTemp}" -webBasePath "${webBasePathTemp}"
        echo ""
        echo -e "${yellow}检测到为全新安装：用户名和密码随机生成，默认访问路径使用 shlii:${plain}"
        echo -e "###############################################"
        echo -e "${green}用户名: ${usernameTemp}${plain}"
        echo -e "${green}密  码: ${passwordTemp}${plain}"
        echo -e "${green}访问路径: ${webBasePathTemp}${plain}"
        echo -e "###############################################"
        echo -e "${green}如果您忘记了登录信息，可以在安装后通过 x-ui 命令然后输入${red}数字 10 选项${green}进行查看${plain}"
    fi
    sleep 1
    echo -e ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
    echo ""
    /usr/local/x-ui/x-ui migrate
}
# ssh 转发提示
ssh_forwarding() {
    # 获取 IPv4 和 IPv6 地址
    v4=$(curl -s4m8 http://ip.sb -k)
    v6=$(curl -s6m8 http://ip.sb -k)
    local setting_output cert_output existing_webBasePath existing_port existing_cert existing_key
    setting_output=$(/usr/local/x-ui/x-ui setting -show true 2>/dev/null | strip_ansi)
    cert_output=$(/usr/local/x-ui/x-ui setting -getCert true 2>/dev/null | strip_ansi)
    existing_webBasePath=$(printf '%s\n' "$setting_output" | sed -n 's/.*webBasePath（访问路径）:[[:space:]]*//p' | head -n1)
    existing_port=$(printf '%s\n' "$setting_output" | sed -n 's/.*port（端口号）:[[:space:]]*//p' | head -n1)
    existing_cert=$(printf '%s\n' "$cert_output" | sed -n 's/.*cert:[[:space:]]*//p' | head -n1)
    existing_key=$(printf '%s\n' "$cert_output" | sed -n 's/.*key:[[:space:]]*//p' | head -n1)
    [[ -n "$existing_webBasePath" ]] || existing_webBasePath="/"

    if [[ -n "$existing_cert" && -n "$existing_key" ]]; then
        echo -e "${green}面板已安装证书采用SSL保护${plain}"
        echo ""
        domain=$(basename "$(dirname "$existing_cert")")
        echo -e "${green}登录访问面板URL: https://${domain}:${existing_port}${green}${existing_webBasePath}${plain}"
    fi
    echo ""
    if [[ -z "$existing_cert" && -z "$existing_key" ]]; then
        echo -e "${yellow}当前未配置证书：面板将使用 HTTP，仍可直接通过 IP + 端口访问。${plain}"
        echo ""

        # Do not enable a firewall here. If UFW is already active, only add the
        # configured panel port so a fresh HTTP install remains reachable.
        if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q "Status: active"; then
            ufw allow "${existing_port}/tcp" >/dev/null 2>&1 || true
        fi

        if [[ -n "$v4" ]]; then
            echo -e "${green}IPv4 登录地址: ${blue}http://${v4}:${existing_port}${existing_webBasePath}${plain}"
        fi
        if [[ -n "$v6" ]]; then
            echo -e "${green}IPv6 登录地址: ${blue}http://[${v6}]:${existing_port}${existing_webBasePath}${plain}"
        fi
        echo ""
        echo -e "${yellow}HTTP 不提供传输加密；如在公网长期管理，可后续自行配置 HTTPS。${plain}"
        echo -e "${green}如不希望直接暴露管理端口，也可以继续使用 SSH 本地转发。${plain}"
        if [[ -n "$v4" ]]; then
            echo -e "${green}SSH 转发示例: ${blue}ssh -L 15208:127.0.0.1:${existing_port} root@${v4}${plain}"
            echo -e "${green}转发后访问: ${blue}http://127.0.0.1:15208${existing_webBasePath}${plain}"
        fi
    fi
}

echo ""
install_x-ui() {
    cd /usr/local/ || exit 1

    if [[ $# == 0 ]]; then
        last_version=$(dui_latest_version || true)
        if [[ -z "$last_version" ]]; then
            echo -e "${red}获取 Dui 最新 Release 失败，请稍后再试。${plain}"
            return 1
        fi
    else
        last_version="$1"
        [[ "$last_version" != v* ]] && last_version="v${last_version}"
    fi

    local platform asset package_url tmpdir staged backup_dir="" etc_backup="" release_mode="指定" is_upgrade=false preserved_web_base_path=""

    # 必须在覆盖旧二进制之前判断是否为升级，并读取旧面板真实访问路径。
    # 旧面板若使用根路径 /，升级后仍保持 IP/域名 + 端口直接访问。
    if [[ -f /etc/x-ui/x-ui.db ]]; then
        is_upgrade=true
        preserved_web_base_path=$(get_existing_web_base_path || true)
        if [[ -n "$preserved_web_base_path" ]]; then
            echo -e "${green}检测到已有面板，升级后将保留访问路径：${yellow}${preserved_web_base_path}${plain}"
        else
            echo -e "${yellow}检测到已有面板；未能从旧程序读取访问路径，将保留数据库现有值。${plain}"
        fi
    fi

    platform=$(arch)
    asset="x-ui-linux-${platform}.tar.gz"
    package_url="${XUI_RELEASE_BASE}/download/${last_version}/${asset}"
    [[ $# == 0 ]] && release_mode="最新"

    echo ""
    echo -e "${green}正在从 Dui GitHub ${release_mode} Release 安装：${plain}"
    echo -e "${green}版本：${yellow}${last_version}${plain}"
    echo -e "${green}架构：${yellow}${platform}${plain}"

    tmpdir=$(mktemp -d) || return 1
    staged="${tmpdir}/x-ui"

    # 先下载、检查、解压和自检；在这些步骤全部通过前绝不停止旧面板。
    if ! curl -fL --retry 3 --connect-timeout 15 -o "${tmpdir}/${asset}" "$package_url"; then
        echo -e "${red}下载 Release 包失败：${package_url}${plain}"
        rm -rf "$tmpdir"
        return 1
    fi
    if ! curl -fsSL --retry 3 -o "${tmpdir}/${asset}.sha256" "${package_url}.sha256" \
        || ! (cd "$tmpdir" && sha256sum -c "${asset}.sha256" >/dev/null); then
        echo -e "${red}Release 包 SHA-256 校验失败，已取消安装。${plain}"
        rm -rf "$tmpdir"
        return 1
    fi
    if ! tar -tzf "${tmpdir}/${asset}" | grep -qx 'x-ui/x-ui' \
        || ! tar -tzf "${tmpdir}/${asset}" | grep -qx 'x-ui/x-ui.sh' \
        || ! tar -tzf "${tmpdir}/${asset}" | grep -qx 'x-ui/x-ui.service' \
        || ! tar -tzf "${tmpdir}/${asset}" | grep -qx 'x-ui/fb5.sh'; then
        echo -e "${red}Release 包缺少 x-ui、x-ui.sh、x-ui.service 或 fb5.sh，已取消安装。${plain}"
        rm -rf "$tmpdir"
        return 1
    fi
    if ! tar -xzf "${tmpdir}/${asset}" -C "$tmpdir"; then
        echo -e "${red}Release 包解压失败，已取消安装。${plain}"
        rm -rf "$tmpdir"
        return 1
    fi
    chmod +x "${staged}/x-ui" "${staged}/x-ui.sh" "${staged}/fb5.sh"
    if ! bash -n "${staged}/x-ui.sh"; then
        echo -e "${red}Release 中菜单脚本语法检查失败，已取消安装。${plain}"
        rm -rf "$tmpdir"
        return 1
    fi
    local staged_version
    staged_version=$("${staged}/x-ui" -v 2>/dev/null | head -n1)
    if [[ -z "$staged_version" ]]; then
        echo -e "${red}Release 中面板二进制无法运行，已取消安装。${plain}"
        rm -rf "$tmpdir"
        return 1
    fi
    echo -e "${green}面板二进制自检通过：${yellow}${staged_version}${plain}"

    # 到这里才开始修改系统文件。先停服务，再完整备份 /etc/x-ui，
    # 避免只复制 SQLite 主文件而漏掉 WAL/SHM 或在写入过程中得到不一致快照。
    systemctl stop x-ui 2>/dev/null || true
    if [[ -d /etc/x-ui ]]; then
        etc_backup="${tmpdir}/etc-x-ui.bak"
        cp -a /etc/x-ui "$etc_backup"
    fi
    if [[ -d /usr/local/x-ui ]]; then
        backup_dir="/usr/local/x-ui.backup.$$.${RANDOM}"
        mv /usr/local/x-ui "$backup_dir"
    fi
    mkdir -p /usr/local
    cp -a "$staged" /usr/local/x-ui
    chmod +x /usr/local/x-ui/x-ui /usr/local/x-ui/x-ui.sh

    # CLI 菜单和 systemd 服务都来自同一个 Release 快照。
    [[ -e /usr/bin/x-ui ]] && cp -aL /usr/bin/x-ui "${tmpdir}/x-ui-menu.bak"
    [[ -f /etc/systemd/system/x-ui.service ]] && cp -a /etc/systemd/system/x-ui.service "${tmpdir}/x-ui.service.bak"
    local had_fb5=false
    if [[ -e /usr/local/bin/fb5 ]]; then
        had_fb5=true
        cp -aL /usr/local/bin/fb5 "${tmpdir}/fb5.bak"
    fi
    if ! install_menu_script_atomic /usr/local/x-ui/x-ui.sh; then
        echo -e "${red}安装菜单脚本失败，正在自动回滚。${plain}"
        systemctl stop x-ui 2>/dev/null || true
        rm -rf /usr/local/x-ui
        [[ -n "$backup_dir" && -d "$backup_dir" ]] && mv "$backup_dir" /usr/local/x-ui
        rm -rf /etc/x-ui
        [[ -n "$etc_backup" && -d "$etc_backup" ]] && cp -a "$etc_backup" /etc/x-ui
        systemctl daemon-reload
        systemctl restart x-ui 2>/dev/null || true
        rm -rf "$tmpdir"
        return 1
    fi
    ln -sfn /usr/bin/x-ui /usr/local/x-ui/x-ui.sh
    if [[ "$had_fb5" == true ]]; then
        install -m 0755 "$staged/fb5.sh" /usr/local/bin/fb5
    fi
    install -m 0644 "$staged/x-ui.service" /etc/systemd/system/x-ui.service

    # 覆盖升级时，在新程序首次读取默认值前明确恢复旧 webBasePath。
    # 这样旧面板使用根路径 / 时，绝不会被新版默认 /shlii/ 覆盖。
    if [[ "$is_upgrade" == "true" && -n "$preserved_web_base_path" ]]; then
        if ! /usr/local/x-ui/x-ui setting -webBasePath "$preserved_web_base_path" >/dev/null 2>&1; then
            echo -e "${red}恢复原面板访问路径失败，正在自动回滚。${plain}"
            systemctl stop x-ui 2>/dev/null || true
            rm -rf /usr/local/x-ui
            [[ -n "$backup_dir" && -d "$backup_dir" ]] && mv "$backup_dir" /usr/local/x-ui
            if [[ -f "${tmpdir}/x-ui-menu.bak" ]]; then
                install_menu_script_atomic "${tmpdir}/x-ui-menu.bak"
            else
                rm -f /usr/bin/x-ui
            fi
            if [[ -f "${tmpdir}/x-ui.service.bak" ]]; then
                install -m 0644 "${tmpdir}/x-ui.service.bak" /etc/systemd/system/x-ui.service
            else
                rm -f /etc/systemd/system/x-ui.service
            fi
            if [[ -f "${tmpdir}/fb5.bak" ]]; then
                install -m 0755 "${tmpdir}/fb5.bak" /usr/local/bin/fb5
            fi
            rm -rf /etc/x-ui
            [[ -n "$etc_backup" && -d "$etc_backup" ]] && cp -a "$etc_backup" /etc/x-ui
            systemctl daemon-reload
            systemctl restart x-ui 2>/dev/null || true
            rm -rf "$tmpdir"
            return 1
        fi
    fi

    config_after_install "$is_upgrade"

    systemctl daemon-reload
    systemctl enable x-ui >/dev/null 2>&1 || true
    if ! systemctl restart x-ui || ! sleep 2 || ! systemctl is-active --quiet x-ui; then
        echo -e "${red}新版本启动失败，正在自动回滚。${plain}"
        systemctl stop x-ui 2>/dev/null || true
        rm -rf /usr/local/x-ui
        if [[ -n "$backup_dir" && -d "$backup_dir" ]]; then
            mv "$backup_dir" /usr/local/x-ui
        fi
        if [[ -f "${tmpdir}/x-ui-menu.bak" ]]; then
            install_menu_script_atomic "${tmpdir}/x-ui-menu.bak"
        else
            rm -f /usr/bin/x-ui
        fi
        if [[ -f "${tmpdir}/x-ui.service.bak" ]]; then
            install -m 0644 "${tmpdir}/x-ui.service.bak" /etc/systemd/system/x-ui.service
        else
            rm -f /etc/systemd/system/x-ui.service
        fi
        if [[ -f "${tmpdir}/fb5.bak" ]]; then
            install -m 0755 "${tmpdir}/fb5.bak" /usr/local/bin/fb5
        fi
        rm -rf /etc/x-ui
        if [[ -n "$etc_backup" && -d "$etc_backup" ]]; then
            cp -a "$etc_backup" /etc/x-ui
        fi
        systemctl daemon-reload
        systemctl restart x-ui 2>/dev/null || true
        rm -rf "$tmpdir"
        return 1
    fi

    [[ -n "$backup_dir" && -d "$backup_dir" ]] && rm -rf "$backup_dir"

    # 无证书时直接给出 HTTP IP+端口访问地址；有证书则输出 HTTPS。
    ssh_forwarding

    # warp 相关（保持原有逻辑）
    systemctl stop warp-go >/dev/null 2>&1
    wg-quick down wgcf >/dev/null 2>&1
    ipv4=$(curl -s4m8 ip.p3terx.com -k | sed -n 1p)
    ipv6=$(curl -s6m8 ip.p3terx.com -k | sed -n 1p)
    systemctl start warp-go >/dev/null 2>&1
    wg-quick up wgcf >/dev/null 2>&1

    rm -rf "$tmpdir"

    echo ""
    echo -e "------->>>>${green}Dui ${last_version}${plain}<<<< 安装/升级成功"
    echo ""
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
    echo -e "x-ui update       - 同步更新面板、菜单和服务文件"
    echo -e "x-ui custom       - 安装指定 Release 版本"
    echo -e "x-ui install      - 安装 Dui 面板"
    echo -e "x-ui uninstall    - 卸载 Dui 面板"
    echo -e "--------------------------------------------"
    echo ""
    echo -e "${yellow}----->>>Dui 面板和 Xray 启动成功<<<-----${plain}"
}

# 设置VPS中的时区/时间为【上海时间】
sudo timedatectl set-timezone Asia/Shanghai

install_base
install_x-ui $1
echo ""
echo -e "----------------------------------------------"
sleep 4
info=$(/usr/local/x-ui/x-ui setting -show true)
echo -e "${info}${plain}"
echo ""
echo -e "若您忘记了上述面板信息，后期可通过x-ui命令进入脚本${red}输入数字〔10〕选项获取${plain}"
echo ""
echo -e "----------------------------------------------"
echo ""
sleep 2
echo -e "${green}安装/更新完成，若在使用过程中有任何问题${plain}"
echo -e "${yellow}请先描述清楚所遇问题加〔Dui面板〕交流群${plain}"
echo -e "${yellow}在TG群中${red} https://t.me/IncuShliidlb ${yellow}截图进行反馈${plain}"
echo ""
echo -e "----------------------------------------------"
echo ""
echo -e "${green}〔Dui面板〕项目地址：${yellow}https://github.com/${XUI_REPO}${plain}"
echo ""
echo -e "${green} 详细安装教程：${yellow}https://github.com/shini74744/Dui${plain}"
echo ""
echo -e "----------------------------------------------"
echo ""
echo -e "-------------->>>>>>>探 针 地 址<<<<<<<<-------------------"
echo ""
echo -e "${green}探针地址：${yellow}shli.io${plain}"
echo ""
echo -e "----------------------------------------------"
echo ""
