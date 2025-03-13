#!/bin/bash

# 设置esmd服务的路径和工作目录
esmd_path="/home/admin/esmsh/esmd"
working_dir="/home/admin/esmsh"

# 创建并配置esmd服务文件
servicefile=/etc/systemd/system/esmd.service
if [ -f $servicefile ]; then
  rm -f $servicefile
fi

echo "[Unit]" >> $servicefile
echo "Description=esm.sh service" >> $servicefile
echo "After=network.target" >> $servicefile
echo "StartLimitIntervalSec=0" >> $servicefile

echo "[Service]" >> $servicefile
echo "Type=simple" >> $servicefile
echo "ExecStart=$esmd_path --config=config.json" >> $servicefile
echo "WorkingDirectory=$working_dir" >> $servicefile
echo "Restart=always" >> $servicefile
echo "RestartSec=5" >> $servicefile

echo "[Install]" >> $servicefile
echo "WantedBy=multi-user.target" >> $servicefile

# 设置服务启动和停止的逻辑
systemctl daemon-reload
systemctl enable esmd.service
systemctl start esmd.service

echo "esmd服务已成功配置并启动。"