#!/bin/bash
# GoShell Linux 安装脚本
# 安装到 /usr/local/bin，并设置桌面图标

set -e

APP_NAME="goshell"
INSTALL_DIR="/usr/local/bin"
ICON_DIR="/usr/share/icons/hicolor/256x256/apps"
DESKTOP_DIR="/usr/share/applications"

# 检查是否为 root
if [ "$EUID" -ne 0 ]; then
  echo "请使用 sudo 运行此脚本"
  echo "Please run this script with sudo"
  exit 1
fi

echo "安装 ${APP_NAME}..."

# 复制二进制
cp "${APP_NAME}" "${INSTALL_DIR}/"
chmod +x "${INSTALL_DIR}/${APP_NAME}"

# 安装图标（如果存在）
if [ -f "assets/icon.png" ]; then
  mkdir -p "${ICON_DIR}"
  cp "assets/icon.png" "${ICON_DIR}/${APP_NAME}.png"
fi

# 安装 desktop entry
if [ -f "packaging/linux/goshell.desktop" ]; then
  cp "packaging/linux/goshell.desktop" "${DESKTOP_DIR}/"
fi

# 更新桌面数据库
if command -v update-desktop-database &> /dev/null; then
  update-desktop-database "${DESKTOP_DIR}"
fi

echo "安装完成！可以从应用菜单启动 ${APP_NAME}，或在终端运行 ${APP_NAME}"
echo "Installation complete! Launch ${APP_NAME} from app menu or terminal"
