package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPTool 发起 HTTP GET 请求获取网页文本内容的工具
type HTTPTool struct {
	client *http.Client
}

// NewHTTPTool 创建 HTTP 工具
func NewHTTPTool() *HTTPTool {
	return &HTTPTool{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (t *HTTPTool) Name() string {
	return "http_get"
}

func (t *HTTPTool) Description() string {
	return "发起 HTTP GET 请求，获取指定 URL 的文本内容（截取前 4000 字符）"
}

func (t *HTTPTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {
				"type": "string",
				"description": "要请求的 URL 地址"
			}
		},
		"required": ["url"]
	}`)
}

type httpArgs struct {
	URL string `json:"url"`
}

func (t *HTTPTool) Execute(ctx context.Context, args string) (string, error) {
	var a httpArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	if a.URL == "" {
		return "", fmt.Errorf("URL 不能为空")
	}

	// 仅允许 http/https 协议
	if !strings.HasPrefix(a.URL, "http://") && !strings.HasPrefix(a.URL, "https://") {
		return "", fmt.Errorf("仅支持 http/https 协议")
	}

	// SSRF 防护：禁止访问内网地址
	if err := validateURL(a.URL); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "MyClaw-Agent/1.0")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 限制读取大小，避免内存溢出
	limited := io.LimitReader(resp.Body, 32*1024)
	body, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	content := string(body)
	if len(content) > 4000 {
		content = content[:4000] + "\n...(内容已截断)"
	}

	return fmt.Sprintf("HTTP %d\n\n%s", resp.StatusCode, content), nil
}

// validateURL 校验 URL，防止 SSRF 攻击（禁止访问内网/回环地址）
func validateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("无效的 URL: %w", err)
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL 缺少主机名")
	}

	// 禁止 localhost
	if strings.EqualFold(host, "localhost") {
		return fmt.Errorf("禁止访问本地地址")
	}

	// 如果已经是 IP 地址，直接校验，避免 DNS 查询
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("禁止访问内网地址 %s", host)
		}
		return nil
	}

	// 域名则需要解析后再校验
	ips, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("域名解析失败: %w", err)
	}

	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("禁止访问内网地址 %s", ipStr)
		}
	}

	return nil
}
