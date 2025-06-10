package utils

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// IsPortInUse 检查指定端口是否被占用
func IsPortInUse(port int) bool {
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return true // 如果无法监听，说明端口被占用
	}
	listener.Close()
	return false
}

// GetPortFromAddress 从地址字符串中提取端口号
func GetPortFromAddress(address string) (int, error) {
	// 如果地址包含冒号，取最后一个冒号后面的部分作为端口
	parts := strings.Split(address, ":")
	if len(parts) < 2 {
		return 0, fmt.Errorf("地址格式无效: %s", address)
	}

	portStr := parts[len(parts)-1]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("无效的端口号: %s", portStr)
	}

	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("端口号超出范围(1-65535): %d", port)
	}

	return port, nil
}

// ValidateAddress 验证地址格式是否正确
func ValidateAddress(address string) error {
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("无效的地址格式: %w", err)
	}

	// 验证端口
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("无效的端口号: %s", portStr)
	}

	// 如果主机部分不为空，验证主机名或IP地址
	if host != "" && host != "localhost" {
		if ip := net.ParseIP(host); ip == nil {
			// 不是有效的IP地址，检查是否是有效的主机名
			if !isValidHostname(host) {
				return fmt.Errorf("无效的主机名: %s", host)
			}
		}
	}

	return nil
}

// isValidHostname 检查主机名是否有效
func isValidHostname(hostname string) bool {
	// 主机名规则：
	// 1. 只能包含字母、数字、连字符和点
	// 2. 不能以连字符或点开始或结束
	// 3. 连字符不能连续出现
	// 4. 总长度不能超过253个字符
	if len(hostname) > 253 {
		return false
	}

	// 分割成标签
	labels := strings.Split(hostname, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if !(c >= 'a' && c <= 'z' ||
				c >= 'A' && c <= 'Z' ||
				c >= '0' && c <= '9' ||
				c == '-') {
				return false
			}
		}
	}

	return true
}
