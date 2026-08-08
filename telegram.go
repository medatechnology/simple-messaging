package simplemessage

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// telegramProvider delivers messages to Telegram chats via the Bot API.
// Config: From = bot token, SecretKey = optional webhook secret token.
func init() {
	Register("telegram", newTelegramProvider)
}

type telegramProvider struct {
	cfg    ProviderConfig
	client *http.Client
}

func newTelegramProvider(cfg ProviderConfig, httpClient *http.Client) (Provider, error) {
	if cfg.From == "" {
		return nil, fmt.Errorf("simplemessage: telegram requires From (bot token)")
	}
	return &telegramProvider{cfg: cfg, client: httpClient}, nil
}

func (p *telegramProvider) Name() string { return "telegram" }

func (p *telegramProvider) Send(ctx context.Context, req *SendRequest) (*SendResponse, error) {
	if req.To == "" {
		return nil, fmt.Errorf("simplemessage: telegram Send requires To (chat id)")
	}
	code, _, body := prepareMessage(req)

	payload := map[string]interface{}{
		"chat_id": req.To,
		"text":    body,
	}
	var out struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
	}
	if err := doJSON(ctx, p.client, http.MethodPost, p.apiBase()+"/sendMessage", payload, &out, &map[string]interface{}{}, nil); err != nil {
		return nil, err
	}
	if !out.OK {
		return nil, fmt.Errorf("simplemessage: telegram send failed")
	}
	return &SendResponse{
		MessageID: strconv.FormatInt(out.Result.MessageID, 10),
		Status:    "sent",
		Channel:   ChannelTelegram,
		Code:      code,
	}, nil
}

func (p *telegramProvider) apiBase() string {
	if p.cfg.BaseURL != "" {
		return p.cfg.BaseURL
	}
	return "https://api.telegram.org/bot" + p.cfg.From
}

func (p *telegramProvider) GetStatus(context.Context, string) (*MessageStatus, error) {
	return nil, fmt.Errorf("simplemessage: %w: telegram has no delivery status API", ErrNotSupported)
}

func (p *telegramProvider) GetBalance(context.Context) (*BalanceResponse, error) {
	return nil, fmt.Errorf("simplemessage: %w: telegram has no balance", ErrNotSupported)
}

// VerifyWebhook parses a Bot API update. When SecretKey is set it must match
// the X-Telegram-Bot-Api-Secret-Token header.
func (p *telegramProvider) VerifyWebhook(body []byte, headers http.Header) (*WebhookEvent, error) {
	if p.cfg.SecretKey != "" {
		got := headers.Get("X-Telegram-Bot-Api-Secret-Token")
		if subtle.ConstantTimeCompare([]byte(p.cfg.SecretKey), []byte(got)) != 1 {
			return nil, fmt.Errorf("simplemessage: telegram webhook secret token mismatch")
		}
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("simplemessage: telegram webhook: %w", err)
	}
	id := ""
	if v, ok := payload["update_id"]; ok {
		id = fmt.Sprintf("%v", v)
	}
	return &WebhookEvent{Type: "message.received", ID: id, Data: payload, Raw: body}, nil
}
