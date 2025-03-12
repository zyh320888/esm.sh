# 使用 custom runtime 作为基础镜像
FROM registry.cn-beijing.aliyuncs.com/aliyunfc/runtime-custom:build-latest

# 设置环境变量
ENV PATH /opt/bin:$PATH
ENV LD_LIBRARY_PATH /opt/lib
WORKDIR /tmp

# 安装 Go 123
RUN wget https://go.dev/dl/go1.23.0.linux-amd64.tar.gz \
    && tar -C /usr/local -xzf go1.23.0.linux-amd64.tar.gz \
    && rm go1.23.0.linux-amd64.tar.gz \
    && mkdir -p /opt/bin \
    && ln -s /usr/local/go/bin/go /opt/bin/go

# 将 /opt 目录打包成 layer.zip
RUN cd /opt \
    && zip -ry layer.zip * .[^.]*

CMD ["bash"]