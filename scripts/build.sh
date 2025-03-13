#!/bin/bash

# 设置构建环境变量
read -p "? 目标操作系统 (默认是 \"linux\"): " goos
read -p "? 目标架构 (默认是 \"amd64\"): " goarch
if [ "$goos" == "" ]; then
  goos="linux"
fi
if [ "$goarch" == "" ]; then
  goarch="amd64"
fi

# 构建Go程序
echo "--- 构建(${goos}_$goarch)..."
export GOOS=$goos
export GOARCH=$goarch
go build -ldflags="-s -w" -o esmd $(dirname $0)/../server/cmd/main.go
if [ "$?" != "0" ]; then
  exit
fi

# 打包为tar.gz格式
echo "--- 打包..."
tar -czf esmd.tar.gz esmd
if [ "$?" != "0" ]; then
  rm -f esmd
  exit
fi

echo "构建和打包完成，生成文件: esmd.tar.gz"