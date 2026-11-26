package wechat

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"devplatform/internal/core/channels"
)

const (
	messageTypeUser = 1

	itemTypeText  = 1
	itemTypeImage = 2
	itemTypeVoice = 3
	itemTypeFile  = 4
	itemTypeVideo = 5
)

type wireMessage struct {
	MessageID    int64      `json:"message_id"`
	FromUserID   string     `json:"from_user_id"`
	ToUserID     string     `json:"to_user_id"`
	GroupID      string     `json:"group_id"`
	CreateTimeMs int64      `json:"create_time_ms"`
	MessageType  int        `json:"message_type"`
	ContextToken string     `json:"context_token"`
	ItemList     []wireItem `json:"item_list"`
}

type wireItem struct {
	Type      int         `json:"type"`
	RefMsg    *wireRefMsg `json:"ref_msg,omitempty"`
	TextItem  *struct {
		Text string `json:"text"`
	} `json:"text_item,omitempty"`
	VoiceItem *struct {
		Text string `json:"text"`
	} `json:"voice_item,omitempty"`
	FileItem *struct {
		FileName string `json:"file_name"`
	} `json:"file_item,omitempty"`
}

type wireRefMsg struct {
	Title       string    `json:"title"`
	MessageItem *wireItem `json:"message_item,omitempty"`
}

// Listen runs the iLink getupdates long-poll loop until ctx is cancelled or
// the session expires (errcode = -14 -> channels.ErrSessionExpired).
// It filters out non-USER messages to avoid self-reply loops and persists the
// updated cursor via cb.OnCursor.
func (a *Adapter) Listen(ctx context.Context, creds channels.Credentials, cursor string, cb channels.ListenCallbacks) error {
	if strings.TrimSpace(creds.BotToken) == "" {
		return channels.ErrSessionExpired
	}
	base := a.resolveBase(creds)

	// Notify server bot online (best-effort, matching iLink protocol lifecycle).
	_ = a.notifyLifecycle(ctx, base, creds.BotToken, "/ilink/bot/msg/notifystart")
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = a.notifyLifecycle(stopCtx, base, creds.BotToken, "/ilink/bot/msg/notifystop")
	}()

	readyNotified := false
	backoff := time.Second
	const maxBackoff = 60 * time.Second

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		msgs, nextCursor, err := a.getUpdates(ctx, base, creds.BotToken, cursor)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if isSessionExpiredError(err) {
				return channels.ErrSessionExpired
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		backoff = time.Second
		if !readyNotified {
			readyNotified = true
			if cb.OnReady != nil {
				cb.OnReady()
			}
		}

		if nextCursor != "" && nextCursor != cursor {
			cursor = nextCursor
			if cb.OnCursor != nil {
				cb.OnCursor(cursor)
			}
		}

		for _, raw := range msgs {
			// Guard against self-reply loops: only process inbound USER messages.
			if raw.MessageType != 0 && raw.MessageType != messageTypeUser {
				continue
			}
			from := strings.TrimSpace(raw.FromUserID)
			if from == "" || (creds.AccountID != "" && from == creds.AccountID) {
				continue
			}

			text := extractInboundText(raw.ItemList)
			if strings.TrimSpace(text) == "" {
				continue
			}

			ts := time.Now().UTC()
			if raw.CreateTimeMs > 0 {
				ts = time.UnixMilli(raw.CreateTimeMs).UTC()
			}
			ctxToken := strings.TrimSpace(raw.ContextToken)
			a.cacheContextToken(from, ctxToken)

			inbound := channels.InboundMessage{
				ChannelType:  ChannelWeChat,
				MessageID:    strconv.FormatInt(raw.MessageID, 10),
				FromUserID:   from,
				ToUserID:     strings.TrimSpace(raw.ToUserID),
				GroupID:      strings.TrimSpace(raw.GroupID),
				Text:         text,
				ContextToken: ctxToken,
				Timestamp:    ts,
			}
			if cb.OnMessage != nil {
				if err := cb.OnMessage(ctx, inbound); err != nil {
					// Continue processing subsequent messages even if one handler fails.
					continue
				}
			}
		}
	}
}

func (a *Adapter) notifyLifecycle(ctx context.Context, base, botToken, path string) error {
	body := map[string]any{
		"base_info": map[string]string{"channel_version": channelVersion},
	}
	return a.doJSON(ctx, base, botToken, http.MethodPost, path, nil, body, nil)
}

func (a *Adapter) getUpdates(ctx context.Context, base, botToken, cursor string) ([]wireMessage, string, error) {
	body := map[string]any{
		"get_updates_buf": cursor,
		"base_info":       map[string]string{"channel_version": channelVersion},
	}
	var resp struct {
		Ret           int           `json:"ret"`
		ErrCode       int           `json:"errcode"`
		ErrMsg        string        `json:"errmsg"`
		Msgs          []wireMessage `json:"msgs"`
		GetUpdatesBuf string        `json:"get_updates_buf"`
	}
	if err := a.doJSON(ctx, base, botToken, http.MethodPost, "/ilink/bot/getupdates", nil, body, &resp); err != nil {
		return nil, "", err
	}
	if resp.ErrCode == errcodeSessionTimeout {
		return nil, "", channels.ErrSessionExpired
	}
	if resp.Ret != 0 || (resp.ErrCode != 0) {
		return nil, "", fmt.Errorf("wechat: getupdates failed (ret=%d errcode=%d errmsg=%s)", resp.Ret, resp.ErrCode, resp.ErrMsg)
	}
	return resp.Msgs, resp.GetUpdatesBuf, nil
}

func extractInboundText(items []wireItem) string {
	var parts []string
	for _, it := range items {
		if it.RefMsg != nil {
			refTitle := strings.TrimSpace(it.RefMsg.Title)
			refBody := ""
			if it.RefMsg.MessageItem != nil {
				refBody = extractItemText(*it.RefMsg.MessageItem)
			}
			if refTitle != "" && refBody != "" {
				parts = append(parts, fmt.Sprintf("[引用: %s | %s]", refTitle, refBody))
			} else if refTitle != "" {
				parts = append(parts, fmt.Sprintf("[引用: %s]", refTitle))
			} else if refBody != "" {
				parts = append(parts, fmt.Sprintf("[引用: %s]", refBody))
			}
		}
		if text := extractItemText(it); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func extractItemText(it wireItem) string {
	switch it.Type {
	case itemTypeText:
		if it.TextItem != nil {
			return strings.TrimSpace(it.TextItem.Text)
		}
	case itemTypeVoice:
		if it.VoiceItem != nil && strings.TrimSpace(it.VoiceItem.Text) != "" {
			return strings.TrimSpace(it.VoiceItem.Text)
		}
		return "[语音消息]"
	case itemTypeImage:
		return "[图片]"
	case itemTypeVideo:
		return "[视频]"
	case itemTypeFile:
		if it.FileItem != nil && strings.TrimSpace(it.FileItem.FileName) != "" {
			return fmt.Sprintf("[文件: %s]", strings.TrimSpace(it.FileItem.FileName))
		}
		return "[文件]"
	}
	return ""
}
