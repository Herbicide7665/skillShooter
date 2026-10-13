package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	APIKey  string `yaml:"glm_api_key"`
	BaseURL string `yaml:"glm_base_url"`
	Model   string `yaml:"glm_model"`
}

func (c Config) Enabled() bool { return c.APIKey != "" }

func (c *Config) defaults() {
	if c.BaseURL == "" {
		c.BaseURL = "https://open.bigmodel.cn/api/paas/v4"
	}
	if c.Model == "" {
		c.Model = "glm-5.3-flash"
	}
}

type Client struct {
	cfg Config
	hc  *http.Client
}

func New(cfg Config) *Client {
	cfg.defaults()
	return &Client{
		cfg: cfg,
		hc:  &http.Client{Timeout: 90 * time.Second},
	}
}

func (c *Client) Config() Config { return c.cfg }

type chatReq struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Temperature float64    `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Chat 调 LLM：base_url 含 /anthropic 时走 Anthropic messages 协议，否则 OpenAI 兼容。
// 均带一次重试。
func (c *Client) Chat(ctx context.Context, system, user string) (string, error) {
	if !c.cfg.Enabled() {
		return "", fmt.Errorf("GLM 未配置")
	}
	if strings.Contains(c.cfg.BaseURL, "/anthropic") {
		return c.chatAnthropic(ctx, system, user)
	}
	return c.chatOpenAI(ctx, system, user)
}

type anthropicReq struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	System    string        `json:"system,omitempty"`
	Messages  []chatMessage `json:"messages"`
	Thinking  *struct {
		Type string `json:"type"`
	} `json:"thinking,omitempty"`
}

type anthropicResp struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) chatAnthropic(ctx context.Context, system, user string) (string, error) {
	body, _ := json.Marshal(anthropicReq{
		Model:     c.cfg.Model,
		MaxTokens: 4096,
		System:    system,
		Messages:  []chatMessage{{Role: "user", Content: user}},
		// GLM-5.3 思考块吃预算且拖慢响应，关闭直出（与 ST GLM-NO-THINK-PATCH 同理）
		Thinking: &struct {
			Type string `json:"type"`
		}{Type: "disabled"},
	})
	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/messages"
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", c.cfg.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		resp, err := c.hc.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("HTTP: %w", err)
			time.Sleep(2 * time.Second)
			continue
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(data), 300))
			if resp.StatusCode >= 500 || resp.StatusCode == 429 {
				time.Sleep(2 * time.Second)
				continue
			}
			return "", lastErr
		}
		var ar anthropicResp
		if err := json.Unmarshal(data, &ar); err != nil {
			return "", fmt.Errorf("解析响应失败: %w", err)
		}
		if ar.Error != nil {
			return "", fmt.Errorf("API 错误: %s", ar.Error.Message)
		}
		var sb strings.Builder
		for _, blk := range ar.Content {
			if blk.Type == "text" {
				sb.WriteString(blk.Text)
			}
		}
		out := strings.TrimSpace(sb.String())
		if out == "" {
			return "", fmt.Errorf("空响应（仅 thinking 块）")
		}
		return out, nil
	}
	return "", lastErr
}

func (c *Client) chatOpenAI(ctx context.Context, system, user string) (string, error) {
	body, _ := json.Marshal(chatReq{
		Model: c.cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0.3,
	})
	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
		resp, err := c.hc.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("HTTP: %w", err)
			time.Sleep(2 * time.Second)
			continue
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(data), 300))
			if resp.StatusCode >= 500 || resp.StatusCode == 429 {
				time.Sleep(2 * time.Second)
				continue
			}
			return "", lastErr
		}
		var cr chatResp
		if err := json.Unmarshal(data, &cr); err != nil {
			return "", fmt.Errorf("解析响应失败: %w", err)
		}
		if cr.Error != nil {
			return "", fmt.Errorf("API 错误 %s: %s", cr.Error.Code, cr.Error.Message)
		}
		if len(cr.Choices) == 0 {
			return "", fmt.Errorf("空响应")
		}
		return cr.Choices[0].Message.Content, nil
	}
	return "", lastErr
}

// Test 连通性测试
func (c *Client) Test(ctx context.Context) (string, error) {
	out, err := c.Chat(ctx, "你是连通性测试器。", "回复两个字：正常")
	if err != nil {
		return "", err
	}
	return out, nil
}

// JSON 帮助：让 LLM 只输出 JSON 并解析
func (c *Client) ChatJSON(ctx context.Context, system, user string, dst interface{}) error {
	raw, err := c.Chat(ctx, system+"\n\n只输出合法 JSON，不要 markdown 代码块，不要解释。", user)
	if err != nil {
		return err
	}
	s := strings.TrimSpace(raw)
	// 剥可能的 ```json ... ``` 包裹
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			s = s[i : j+1]
		}
	}
	return json.Unmarshal([]byte(s), dst)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
