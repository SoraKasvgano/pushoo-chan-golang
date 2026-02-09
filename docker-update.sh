#!/bin/bash
# Docker update script for pushoo-chan-gover
# 适配 dist 子目录存放二进制文件的场景（无 frontend、无 buildx）

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# 配置项
CONTAINER_NAME="pushoo-chan-gover"
IMAGE_NAME="pushoo-chan-gover:latest"
# 二进制文件路径：脚本执行目录（docker-compose.yml 目录）下的 dist 子目录
BINARY_PATH="./dist/pushoo-chan-gover-linux-amd64"
PORT="8084"

# 输出函数
echo_info() { echo -e "${YELLOW}[INFO]${NC} $1"; }
echo_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
echo_error() { echo -e "${RED}[ERROR]${NC} $1"; }

echo "========================================"
echo "Docker Update Script（适配 dist 子目录）"
echo "========================================"

# 前置检查：确认二进制文件在 dist 目录下
if ! command -v docker &> /dev/null; then
    echo_error "Docker 未安装！"
    exit 1
fi
if [ ! -f "$BINARY_PATH" ]; then
    echo_error "二进制文件不存在：$BINARY_PATH"
    echo_error "请确认文件在 docker-compose.yml 目录的 dist 子目录下"
    exit 1
fi

# 1. 停止容器
echo_info "[1/5] 停止容器..."
if docker ps -q -f name="^/${CONTAINER_NAME}$" | grep -q .; then
    docker stop "$CONTAINER_NAME"
    echo_success "容器已停止"
else
    echo_info "容器未运行"
fi

# 2. 删除容器
echo_info "[2/5] 删除容器..."
if docker ps -a -q -f name="^/${CONTAINER_NAME}$" | grep -q .; then
    docker rm "$CONTAINER_NAME"
    echo_success "容器已删除"
else
    echo_info "容器不存在"
fi

# 3. 删除旧镜像
echo_info "[3/5] 删除旧镜像..."
if docker images -q "$IMAGE_NAME" | grep -q .; then
    docker rmi -f "$IMAGE_NAME"
    echo_success "镜像已删除"
else
    echo_info "镜像不存在"
fi

# 4. 构建新镜像（上下文为当前目录，即 dist 上级目录）
echo_info "[4/5] 构建新镜像..."
docker build --no-cache -t "$IMAGE_NAME" .
echo_success "镜像构建完成"

# 5. 启动容器（用 docker compose，适配 docker-compose.yml 配置）
echo_info "[5/5] 启动新容器..."
COMPOSE_DOCKER_CLI_BUILD=0 docker compose up -d
echo_success "容器启动成功"

# 完成信息
echo -e "\n========================================"
echo_success "更新完成！"
echo "========================================"
echo "二进制文件路径：$BINARY_PATH"
echo "容器名：$CONTAINER_NAME"
echo "访问地址：http://localhost:$PORT"
echo "查看日志：docker logs -f $CONTAINER_NAME"
echo "========================================"