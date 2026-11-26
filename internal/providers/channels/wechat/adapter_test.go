package wechat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"devplatform/internal/core/channels"
)

func TestQRLoginAndScannedStatus(t *testing.T) {
	var polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ilink/bot/get_bot_qrcode":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ret":                0,
				"qrcode":             "qr-123",
				"qrcode_img_content": "https://weixin.qq.com/x/qr-123",
			})
		case "/ilink/bot/get_qrcode_status":
			n := polls.Add(1)
			switch {
			case n == 1:
				_ = json.NewEncoder(w).Encode(map[string]any{"ret": 0, "status": "wait"})
			case n == 2:
				_ = json.NewEncoder(w).Encode(map[string]any{"ret": 0, "status": "scaned"})
			default:
				_ = json.NewEncoder(w).Encode(map[string]any{
					"ret":           0,
					"status":        "confirmed",
					"bot_token":     "tok-abc",
					"baseurl":       "https://ilink.example.test",
					"ilink_bot_id":  "bot@im.bot",
					"ilink_user_id": "owner@im.wechat",
				})
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	adapter := New(Options{
		BaseURL:      srv.URL,
		PollInterval: 10 * time.Millisecond,
	})
	sess, err := adapter.StartLogin(context.Background())
	if err != nil {
		t.Fatalf("StartLogin: %v", err)
	}
	if sess.QRURL != "https://weixin.qq.com/x/qr-123" {
		t.Fatalf("unexpected QRURL: %s", sess.QRURL)
	}

	var statuses []channels.LoginStatus
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	creds, err := sess.Wait(ctx, func(ev channels.LoginEvent) {
		statuses = append(statuses, ev.Status)
	})
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if creds.BotToken != "tok-abc" || creds.AccountID != "bot@im.bot" || creds.UserID != "owner@im.wechat" {
		t.Fatalf("unexpected creds: %+v", creds)
	}
	if len(statuses) < 3 || statuses[1] != channels.LoginScanned || statuses[len(statuses)-1] != channels.LoginConfirmed {
		t.Fatalf("unexpected status sequence: %+v", statuses)
	}
}

func TestListenFiltersBotMessagesPersistsCursorAndDetectsExpiry(t *testing.T) {
	var call atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ilink/bot/msg/notifystart", "/ilink/bot/msg/notifystop":
			_ = json.NewEncoder(w).Encode(map[string]any{"ret": 0})
		case "/ilink/bot/getupdates":
			n := call.Add(1)
			if n == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"ret":             0,
					"get_updates_buf": "cursor-1",
					"msgs": []map[string]any{
						{
							"message_id":   100,
							"from_user_id": "bot@im.bot",
							"message_type": 2, // BOT self message -> must be skipped
							"item_list":    []map[string]any{{"type": 1, "text_item": map[string]any{"text": "self"}}},
						},
						{
							"message_id":    101,
							"from_user_id":  "user1@im.wechat",
							"to_user_id":    "bot@im.bot",
							"message_type":  1, // USER
							"context_token": "ctx-tok-1",
							"item_list": []map[string]any{
								{
									"type":      1,
									"text_item": map[string]any{"text": "帮我创建会话"},
									"ref_msg":   map[string]any{"title": "项目A"},
								},
							},
						},
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ret":     0,
				"errcode": -14,
				"errmsg":  "session timeout",
			})
		}
	}))
	defer srv.Close()

	adapter := New(Options{BaseURL: srv.URL})
	var received []channels.InboundMessage
	var cursors []string
	var ready atomic.Bool

	err := adapter.Listen(context.Background(), channels.Credentials{
		BotToken:  "tok",
		AccountID: "bot@im.bot",
	}, "", channels.ListenCallbacks{
		OnReady:  func() { ready.Store(true) },
		OnCursor: func(c string) { cursors = append(cursors, c) },
		OnMessage: func(ctx context.Context, msg channels.InboundMessage) error {
			received = append(received, msg)
			return nil
		},
	})
	if !errors.Is(err, channels.ErrSessionExpired) {
		t.Fatalf("expected ErrSessionExpired, got %v", err)
	}
	if !ready.Load() {
		t.Fatal("expected OnReady callback")
	}
	if len(cursors) != 1 || cursors[0] != "cursor-1" {
		t.Fatalf("unexpected cursors: %+v", cursors)
	}
	if len(received) != 1 || received[0].FromUserID != "user1@im.wechat" || !strings.Contains(received[0].Text, "帮我创建会话") {
		t.Fatalf("unexpected inbound messages: %+v", received)
	}
}

func TestSendMessageSplitsAndSanitizes(t *testing.T) {
	var sentBodies []string
	var clientIDs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ilink/bot/sendmessage" {
			var body struct {
				Msg struct {
					FromUserID   *string `json:"from_user_id"`
					ToUserID     string  `json:"to_user_id"`
					ClientID     string  `json:"client_id"`
					ContextToken string  `json:"context_token"`
					MessageType  int     `json:"message_type"`
					MessageState int     `json:"message_state"`
					ItemList     []struct {
						TextItem *struct {
							Text string `json:"text"`
						} `json:"text_item"`
					} `json:"item_list"`
				} `json:"msg"`
				BaseInfo map[string]string `json:"base_info"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode sendmessage: %v", err)
			}
			if body.Msg.ClientID == "" {
				t.Fatal("expected non-empty client_id")
			}
			if body.Msg.FromUserID == nil || *body.Msg.FromUserID != "" {
				t.Fatalf("expected explicit empty from_user_id, got %+v", body.Msg.FromUserID)
			}
			if body.BaseInfo["channel_version"] == "" {
				t.Fatalf("expected base_info.channel_version, got %+v", body.BaseInfo)
			}
			clientIDs = append(clientIDs, body.Msg.ClientID)
			if len(body.Msg.ItemList) > 0 && body.Msg.ItemList[0].TextItem != nil {
				sentBodies = append(sentBodies, body.Msg.ItemList[0].TextItem.Text)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ret": 0})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	adapter := New(Options{BaseURL: srv.URL, ChunkDelay: 0})
	longText := "密钥 sk-abcdefghijklmnopqrstuvwxyz123456 不可泄露。\n\n" + strings.Repeat("测试中文分段内容。", 300)
	err := adapter.SendMessage(context.Background(), channels.Credentials{BotToken: "tok"}, channels.OutboundMessage{
		ToUserID:     "u1@im.wechat",
		ContextToken: "ctx-1",
		Text:         longText,
	}, 200)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if len(sentBodies) < 2 {
		t.Fatalf("expected multi-chunk delivery, got %d chunk(s)", len(sentBodies))
	}
	seenIDs := make(map[string]bool)
	for _, cid := range clientIDs {
		if seenIDs[cid] {
			t.Fatalf("duplicate client_id %q across chunks", cid)
		}
		seenIDs[cid] = true
	}
	joined := strings.Join(sentBodies, "\n")
	if strings.Contains(joined, "sk-abcdef") || !strings.Contains(joined, "[REDACTED]") {
		t.Fatalf("secret was not sanitized: %s", sentBodies[0])
	}
	for i, chunk := range sentBodies {
		if utf8.RuneCountInString(chunk) > 200 {
			t.Fatalf("chunk %d exceeds 200 runes: %d", i, utf8.RuneCountInString(chunk))
		}
	}
}

func TestSendMessageChecksErrcodeEvenWhenRetZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ret":     0,
			"errcode": -2,
			"errmsg":  "rejected",
		})
	}))
	defer srv.Close()

	adapter := New(Options{BaseURL: srv.URL, ChunkDelay: 0})
	err := adapter.SendMessage(context.Background(), channels.Credentials{BotToken: "tok"}, channels.OutboundMessage{
		ToUserID:     "u1@im.wechat",
		ContextToken: "ctx-1",
		Text:         "hello",
	}, 2000)
	if err == nil || !strings.Contains(err.Error(), "errcode=-2") {
		t.Fatalf("expected explicit errcode=-2 error, got %v", err)
	}
}
