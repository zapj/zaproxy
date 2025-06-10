# ZAProxy

ZAProxy 是一个用 Go 语言编写的高性能 HTTP/HTTPS 代理服务器。它提供了简单但强大的代理功能，支持基本认证、自定义超时设置和守护进程模式。

## 特性

- 支持 HTTP 和 HTTPS 代理
- 内置基本认证机制
- 可配置的超时设置
- 支持守护进程模式
- 详细的错误日志记录
- 高性能的数据传输
- 支持 X-Forwarded-For 头部
- 自动处理 hop-by-hop 头部
- 灵活的缓冲区大小配置

## 安装

```bash
go install github.com/zapj/zaproxy@latest
```

## 使用方法

### 基本用法

启动代理服务器（默认端口 12828）：

```bash
zaproxy http
```

### 自定义配置

```bash
# 默认端口 12828
# 默认用户名密码 zaproxy:zaproxy

# 指定端口
zaproxy http -l :8080

# 以守护进程模式运行
zaproxy http -l :8080 -d
```

### 命令行参数

- `-l, --listen`: 设置代理服务器端口（默认：:12828）
- `--daemon`: 以守护进程模式运行
- `--auth-file` : 认证文件路径 (格式：username:password)

### 使用代理

#### curl 示例

```bash
# HTTP 代理
curl -x http://localhost:12828 -U zaproxy:zaproxy http://example.com

# HTTPS 代理
curl -x http://localhost:12828 -U zaproxy:zaproxy https://example.com
```

#### 环境变量设置

```bash
export http_proxy=http://zaproxy:zaproxy@localhost:12828
export https_proxy=http://zaproxy:zaproxy@localhost:12828
```


### PowerShell

```bash
$env:HTTP_PROXY=http://zaproxy:zaproxy@your_ip_address:12828
$env:HTTPS_PROXY=http://zaproxy:zaproxy@your_ip_address:12828
```
## 特性详解

### 基本认证

ZAProxy 默认启用基本认证，默认凭据：
- 用户名：zaproxy
- 密码：zaproxy

### 超时设置

- 默认连接超时：60 秒
- 默认请求超时：10 分钟
- 支持 Keep-Alive 连接

### 性能优化

- 使用高效的缓冲区管理
- 支持配置缓冲区大小
- 自动处理连接关闭和错误情况
- 支持分块传输编码

### 安全特性

- 自动移除敏感的 hop-by-hop 头部
- 支持 TLS 连接
- 请求和响应完整性验证
- 详细的错误日志记录

## 开发

### 构建项目

```bash
git clone https://github.com/zapj/zaproxy.git
cd zaproxy
go build
```

### 项目结构

```
zaproxy/
├── cmd/                    # 命令行接口
│   ├── commands/          # 命令实现
│   └── zaproxy.go        # 主入口
├── http_proxy/           # 代理核心实现
│   ├── http_proxy.go     # HTTP/HTTPS 代理
│   └── http_proxy_auth.go # 认证实现
└── utils/                # 工具函数
```

## 贡献

欢迎提交 Pull Requests 和 Issues！

## 发布

本项目使用 GitHub Actions 自动构建和发布。当推送新的版本标签（如 `v1.0.0`）时，将自动触发构建流程：

- 自动构建 Linux (amd64/arm64) 和 Windows (amd64/arm64) 平台的二进制文件
- 自动创建 GitHub Release
- 自动上传构建的二进制文件到 Release

### 发布新版本

```bash
# 标记新版本
git tag v1.0.0

# 推送标签到 GitHub，这将触发自动构建和发布
git push origin v1.0.0
```

## 许可证

[License Apache 2.0]