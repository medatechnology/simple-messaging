package simplemessage

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
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
	// Hide a sensitive substring (e.g. the OTP code) behind a spoiler entity.
	if req.Spoiler != "" {
		if idx := strings.Index(body, req.Spoiler); idx >= 0 {
			payload["message_entities"] = []map[string]interface{}{
				{"type": "spoiler", "offset": idx, "length": len(req.Spoiler)},
			}
		}
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

// ResolveTelegramChatID returns the chat id of the most recent update for a
// bot token, so callers can auto-fill the target chat during setup. The
// operator must have messaged the bot at least once. Generic and reusable —
// setup wizards should use this instead of guessing chat ids.
func ResolveTelegramChatID(ctx context.Context, token string, httpClient *http.Client) (int64, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	var out struct {
		OK     bool `json:"ok"`
		Result []struct {
			Message struct {
				Chat struct {
					ID int64 `json:"id"`
				} `json:"chat"`
			} `json:"message"`
		} `json:"result"`
	}
	if err := doJSON(ctx, httpClient, http.MethodGet, "https://api.telegram.org/bot"+token+"/getUpdates", nil, &out, &map[string]interface{}{}, nil); err != nil {
		return 0, err
	}
	if !out.OK || len(out.Result) == 0 {
		return 0, fmt.Errorf("simplemessage: telegram has no updates yet — message your bot first")
	}
	return out.Result[len(out.Result)-1].Message.Chat.ID, nil
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
