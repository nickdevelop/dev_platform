// Package wechat implements core/channels.ChannelAdapter for personal WeChat
// via Tencent's iLink Bot protocol (using github.com/lib-x/ilink for outbound
// messaging, typing indicators and token structures).
package wechat

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lib-x/ilink"

	"devplatform/internal/core/channels"
)

const (
	// ChannelWeChat identifies the personal WeChat iLink channel adapter.
	ChannelWeChat channels.ChannelType = "wechat"

	defaultBaseURL        = "https://ilinkai.weixin.qq.com"
	channelVersion        = "2.0.0"
	errcodeSessionTimeout = -14
	defaultMaxReplyLength = 2000
	chunkSendDelay        = 100 * time.Millisecond
)

// Descriptor declares metadata for the personal WeChat channel.
var Descriptor = channels.ChannelDescriptor{
	Type:            ChannelWeChat,
	Label:           "个人微信",
	Description:     "基于腾讯 iLink Bot 协议扫码接入个人微信，支持图文问答与异步任务结果通知",
	SupportsQRLogin: true,
}

// Options customizes the WeChat adapter (mainly for unit testing).
type Options struct {
	BaseURL      string
	HTTPClient   *http.Client
	PollInterval time.Duration
	ChunkDelay   time.Duration
}

// Adapter implements channels.ChannelAdapter for personal WeChat.
type Adapter struct {
	baseURL       string
	httpClient    *http.Client
	pollInterval  time.Duration
	chunkDelay    time.Duration
	contextTokens sync.Map // map[userID string]contextToken string
}

// New creates a WeChat iLink channel adapter.
func New(opts ...Options) *Adapter {
	var cfg Options
	if len(opts) > 0 {
		cfg = opts[0]
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = defaultBaseURL
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 45 * time.Second}
	}
	pollInterval := cfg.PollInterval
	if pollInterval <= 0 {
		pollInterval = 1500 * time.Millisecond
	}
	chunkDelay := cfg.ChunkDelay
	if chunkDelay < 0 {
		chunkDelay = 0
	} else if chunkDelay == 0 && len(opts) == 0 {
		chunkDelay = chunkSendDelay
	} else if chunkDelay == 0 && cfg.BaseURL == "" {
		chunkDelay = chunkSendDelay
	}
	return &Adapter{
		baseURL:      base,
		httpClient:   hc,
		pollInterval: pollInterval,
		chunkDelay:   chunkDelay,
	}
}

func (a *Adapter) Descriptor() channels.ChannelDescriptor {
	return Descriptor
}

func (a *Adapter) resolveBase(creds channels.Credentials) string {
	if b := strings.TrimRight(strings.TrimSpace(creds.BaseURL), "/"); b != "" {
		return b
	}
	return a.baseURL
}

func (a *Adapter) cacheContextToken(userID, token string) {
	u := strings.TrimSpace(userID)
	t := strings.TrimSpace(token)
	if u != "" && t != "" {
		a.contextTokens.Store(u, t)
	}
}

func (a *Adapter) cachedContextToken(userID string) string {
	if v, ok := a.contextTokens.Load(strings.TrimSpace(userID)); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func newClientID() string {
	var buf [16]byte
	if _, err := cryptorand.Read(buf[:]); err == nil {
		buf[6] = (buf[6] & 0x0f) | 0x40
		buf[8] = (buf[8] & 0x3f) | 0x80
		return fmt.Sprintf("%s-%s-%s-%s-%s",
			hex.EncodeToString(buf[0:4]),
			hex.EncodeToString(buf[4:6]),
			hex.EncodeToString(buf[6:8]),
			hex.EncodeToString(buf[8:10]),
			hex.EncodeToString(buf[10:16]),
		)
	}
	return strconv.FormatInt(time.Now().UnixNano(), 16)
}

// SendMessage sanitizes outbound text, splits long replies into WeChat-safe
// chunks, and sends them directly to /ilink/bot/sendmessage with unique client_id,
// base_info, and strict ret/errcode error checking.
func (a *Adapter) SendMessage(ctx context.Context, creds channels.Credentials, msg channels.OutboundMessage, maxLen int) error {
	if strings.TrimSpace(msg.ToUserID) == "" {
		return errors.New("wechat: target user id is required")
	}
	if strings.TrimSpace(creds.BotToken) == "" {
		return channels.ErrSessionExpired
	}
	sanitized := SanitizeOutput(msg.Text)
	if strings.TrimSpace(sanitized) == "" {
		return nil
	}
	if maxLen <= 0 {
		maxLen = defaultMaxReplyLength
	}
	chunks := SplitMessage(sanitized, maxLen)
	base := a.resolveBase(creds)
	ctxToken := strings.TrimSpace(msg.ContextToken)
	if ctxToken != "" {
		a.cacheContextToken(msg.ToUserID, ctxToken)
	} else {
		ctxToken = a.cachedContextToken(msg.ToUserID)
	}

	for i, chunk := range chunks {
		if i > 0 && a.chunkDelay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(a.chunkDelay):
			}
		}
		outboundMsg := map[string]any{
			"from_user_id":  "",
			"to_user_id":    msg.ToUserID,
			"client_id":     newClientID(),
			"message_type":  2,
			"message_state": 2,
			"context_token": ctxToken,
			"item_list": []map[string]any{
				{
					"type":      itemTypeText,
					"text_item": map[string]any{"text": chunk},
				},
			},
		}
		if strings.TrimSpace(msg.GroupID) != "" {
			outboundMsg["group_id"] = msg.GroupID
		}
		reqBody := map[string]any{
			"msg":       outboundMsg,
			"base_info": map[string]string{"channel_version": channelVersion},
		}
		var resp struct {
			Ret     int    `json:"ret"`
			ErrCode int    `json:"errcode"`
			ErrMsg  string `json:"errmsg"`
		}
		if err := a.doJSON(ctx, base, creds.BotToken, http.MethodPost, "/ilink/bot/sendmessage", nil, reqBody, &resp); err != nil {
			if isSessionExpiredError(err) {
				return channels.ErrSessionExpired
			}
			return fmt.Errorf("wechat: send chunk %d/%d: %w", i+1, len(chunks), err)
		}
		if resp.ErrCode == errcodeSessionTimeout {
			return channels.ErrSessionExpired
		}
		if resp.Ret != 0 || resp.ErrCode != 0 {
			return fmt.Errorf("wechat: send chunk %d/%d rejected (ret=%d errcode=%d errmsg=%s)", i+1, len(chunks), resp.Ret, resp.ErrCode, resp.ErrMsg)
		}
	}
	return nil
}

// SendTyping toggles the typing indicator for userID using getconfig + sendtyping.
func (a *Adapter) SendTyping(ctx context.Context, creds channels.Credentials, userID string, typing bool) error {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(creds.BotToken) == "" {
		return nil
	}
	base := a.resolveBase(creds)
	cfgReq := map[string]any{
		"ilink_user_id": userID,
		"base_info":     map[string]string{"channel_version": channelVersion},
	}
	if ctxTok := a.cachedContextToken(userID); ctxTok != "" {
		cfgReq["context_token"] = ctxTok
	}
	var cfgResp struct {
		Ret          int    `json:"ret"`
		ErrCode      int    `json:"errcode"`
		TypingTicket string `json:"typing_ticket"`
	}
	if err := a.doJSON(ctx, base, creds.BotToken, http.MethodPost, "/ilink/bot/getconfig", nil, cfgReq, &cfgResp); err != nil {
		return err
	}
	if cfgResp.Ret != 0 || cfgResp.ErrCode != 0 || strings.TrimSpace(cfgResp.TypingTicket) == "" {
		return nil
	}
	status := 2
	if typing {
		status = 1
	}
	typingReq := map[string]any{
		"ilink_user_id": userID,
		"typing_ticket": cfgResp.TypingTicket,
		"status":        status,
		"base_info":     map[string]string{"channel_version": channelVersion},
	}
	var typingResp struct {
		Ret     int    `json:"ret"`
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := a.doJSON(ctx, base, creds.BotToken, http.MethodPost, "/ilink/bot/sendtyping", nil, typingReq, &typingResp); err != nil {
		return err
	}
	if typingResp.Ret != 0 || typingResp.ErrCode != 0 {
		return fmt.Errorf("wechat: sendtyping failed (ret=%d errcode=%d errmsg=%s)", typingResp.Ret, typingResp.ErrCode, typingResp.ErrMsg)
	}
	return nil
}

func (a *Adapter) doJSON(ctx context.Context, base, botToken, method, path string, extraHeaders map[string]string, body, dst any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("wechat: marshal request: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+path, reader)
	if err != nil {
		return fmt.Errorf("wechat: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	uin := base64.StdEncoding.EncodeToString([]byte(strconv.FormatUint(uint64(rand.Uint32()), 10)))
	req.Header.Set("X-WECHAT-UIN", uin)
	if botToken != "" {
		req.Header.Set("AuthorizationType", "ilink_bot_token")
		req.Header.Set("Authorization", "Bearer "+botToken)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("wechat: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("wechat: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(raw)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return fmt.Errorf("wechat: %s %s: status %d: %s", method, path, resp.StatusCode, snippet)
	}
	if dst != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, dst); err != nil {
			snippet := string(raw)
			if len(snippet) > 200 {
				snippet = snippet[:200]
			}
			return fmt.Errorf("wechat: decode %s response: %w (body: %s)", path, err, snippet)
		}
	}
	return nil
}

func isSessionExpiredError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ilink.ErrSessionTimeout) || errors.Is(err, channels.ErrSessionExpired) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "-14") && strings.Contains(strings.ToLower(msg), "errcode")
}

func buildQueryURL(path string, params map[string]string) string {
	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	return path + "?" + q.Encode()
}
