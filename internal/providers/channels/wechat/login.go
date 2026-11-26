package wechat

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"devplatform/internal/core/channels"
)

// StartLogin initiates an iLink QR code login flow and returns a LoginSession
// whose Wait method emits intermediate scan statuses and returns full credentials
// (including ilink_bot_id and ilink_user_id).
func (a *Adapter) StartLogin(ctx context.Context) (*channels.LoginSession, error) {
	var resp struct {
		Ret              int    `json:"ret"`
		ErrCode          int    `json:"errcode"`
		ErrMsg           string `json:"errmsg"`
		QRCode           string `json:"qrcode"`
		QRCodeImgContent string `json:"qrcode_img_content"`
	}
	if err := a.doJSON(ctx, a.baseURL, "", http.MethodGet, "/ilink/bot/get_bot_qrcode?bot_type=3", nil, nil, &resp); err != nil {
		return nil, err
	}
	if resp.Ret != 0 || resp.ErrCode != 0 {
		return nil, fmt.Errorf("wechat: get_bot_qrcode failed (ret=%d errcode=%d errmsg=%s)", resp.Ret, resp.ErrCode, resp.ErrMsg)
	}
	if strings.TrimSpace(resp.QRCode) == "" {
		return nil, fmt.Errorf("wechat: empty qrcode returned")
	}

	qrRaw := strings.TrimSpace(resp.QRCode)
	qrImg := strings.TrimSpace(resp.QRCodeImgContent)
	// When qrcode_img_content is an HTTPS image URL, keep it as QRURL; when it
	// is a weixin:// or raw QR payload, expose both so UI can render locally.
	qrContent := qrRaw
	if qrImg != "" && !strings.HasPrefix(qrImg, "http://") && !strings.HasPrefix(qrImg, "https://") {
		qrContent = qrImg
	}

	return &channels.LoginSession{
		QRCode: qrContent,
		QRURL:  qrImg,
		Wait: func(waitCtx context.Context, onStatus func(channels.LoginEvent)) (*channels.Credentials, error) {
			return a.pollQRLogin(waitCtx, qrRaw, qrContent, qrImg, onStatus)
		},
	}, nil
}

func (a *Adapter) pollQRLogin(
	ctx context.Context,
	qrcode string,
	qrContent string,
	qrImg string,
	onStatus func(channels.LoginEvent),
) (*channels.Credentials, error) {
	path := buildQueryURL("/ilink/bot/get_qrcode_status", map[string]string{"qrcode": qrcode})
	headers := map[string]string{"iLink-App-ClientVersion": "1"}

	ticker := time.NewTicker(a.pollInterval)
	defer ticker.Stop()

	lastStatus := "wait"
	emit := func(ev channels.LoginEvent) {
		if onStatus != nil {
			onStatus(ev)
		}
	}

	emit(channels.LoginEvent{
		Status: channels.LoginPending,
		QRCode: qrContent,
		QRURL:  qrImg,
	})

	for {
		select {
		case <-ctx.Done():
			return nil, channels.ErrLoginTimeout
		case <-ticker.C:
		}

		var resp struct {
			Ret         int    `json:"ret"`
			ErrCode     int    `json:"errcode"`
			ErrMsg      string `json:"errmsg"`
			Status      string `json:"status"`
			BotToken    string `json:"bot_token"`
			BaseURL     string `json:"baseurl"`
			ILinkBotID  string `json:"ilink_bot_id"`
			ILinkUserID string `json:"ilink_user_id"`
		}
		if err := a.doJSON(ctx, a.baseURL, "", http.MethodGet, path, headers, nil, &resp); err != nil {
			// Transient network glitch; keep polling until ctx expires.
			continue
		}
		if resp.Ret != 0 || resp.ErrCode != 0 {
			errMsg := fmt.Errorf("wechat: get_qrcode_status failed (ret=%d errcode=%d errmsg=%s)", resp.Ret, resp.ErrCode, resp.ErrMsg)
			emit(channels.LoginEvent{Status: channels.LoginError, Error: errMsg.Error()})
			return nil, errMsg
		}

		switch strings.ToLower(strings.TrimSpace(resp.Status)) {
		case "wait", "pending", "":
			if lastStatus != "wait" {
				lastStatus = "wait"
				emit(channels.LoginEvent{Status: channels.LoginPending, QRCode: qrContent, QRURL: qrImg})
			}
		case "scaned", "scanned":
			if lastStatus != "scanned" {
				lastStatus = "scanned"
				emit(channels.LoginEvent{Status: channels.LoginScanned, QRCode: qrContent, QRURL: qrImg})
			}
		case "confirmed":
			if strings.TrimSpace(resp.BotToken) == "" {
				err := fmt.Errorf("wechat: confirmed login missing bot_token")
				emit(channels.LoginEvent{Status: channels.LoginError, Error: err.Error()})
				return nil, err
			}
			creds := &channels.Credentials{
				BotToken:  strings.TrimSpace(resp.BotToken),
				BaseURL:   strings.TrimSpace(resp.BaseURL),
				AccountID: strings.TrimSpace(resp.ILinkBotID),
				UserID:    strings.TrimSpace(resp.ILinkUserID),
			}
			emit(channels.LoginEvent{Status: channels.LoginConfirmed, Credentials: creds})
			return creds, nil
		case "expired":
			emit(channels.LoginEvent{Status: channels.LoginExpired})
			return nil, channels.ErrQRCodeExpired
		case "cancelled", "canceled":
			emit(channels.LoginEvent{Status: channels.LoginCancelled})
			return nil, channels.ErrQRCodeCancelled
		}
	}
}
