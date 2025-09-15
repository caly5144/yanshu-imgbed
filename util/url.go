package util

import (
	"net/url"
	"strings"
)

// ExtractEndpointHost 从完整的URL中提取主机名部分
// 例如: "https://oss-cn-hangzhou.aliyuncs.com" -> "oss-cn-hangzhou.aliyuncs.com"
func ExtractEndpointHost(endpointURL string) string {
	// 确保URL有协议前缀，以便url.Parse能正确工作
	if !strings.HasPrefix(endpointURL, "http://") && !strings.HasPrefix(endpointURL, "https://") {
		endpointURL = "https://" + endpointURL
	}

	parsedURL, err := url.Parse(endpointURL)
	if err != nil {
		// 如果解析失败，返回原始字符串作为备用
		return endpointURL
	}
	return parsedURL.Host
}
