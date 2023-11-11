# 使用Go 1.17.5 作为基础镜像
FROM golang:1.17.5

# 设置工作目录
WORKDIR /app

# 将本地的 Go 项目拷贝到容器中的工作目录
COPY . .

# 构建 Go 项目
RUN go build -o main main.go

# 暴露应用程序运行的端口
EXPOSE 9000

# 运行你的 Go 应用
CMD ["./main"]

