package simplemessage

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// indonesiaProvider delivers WhatsApp + SMS messages through Fonnte
// (api.fonnte.com), an Indonesian vendor with a balance API used by the
// sureauth deposit metering.
// Config: APIKey = Fonnte token, SecretKey = optional webhook token,
// BaseURL = API host override.
func init() {
	Register("sms", newIndonesiaProvider) // SMS channel
	Register("fonnte", newIndonesiaProvider)
}

type indonesiaProvider struct {
	cfg    ProviderConfig
	client *http.Client
}

func newIndonesiaProvider(cfg ProviderConfig, httpClient *http.Client) (Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("simplemessage: fonnte requires APIKey (token)")
	}
	return &indonesiaProvider{cfg: cfg, client: httpClient}, nil
}

func (p *indonesiaProvider) Name() string { return "fonnte" }

func (p *indonesiaProvider) apiBase() string {
	if p.cfg.BaseURL != "" {
		return p.cfg.BaseURL
	}
	return "https://api.fonnte.com"
}

func (p *indonesiaProvider) Send(ctx context.Context, req *SendRequest) (*SendResponse, error) {
	if req.To == "" {
		return nil, fmt.Errorf("simplemessage: fonnte Send requires To (phone number)")
	}
	code, _, body := prepareMessage(req)

	form := url.Values{}
	form.Set("target", req.To)
	form.Set("message", body)
	if len(req.TemplateParams) > 0 {
		for k, v := range req.TemplateParams {
			form.Set(k, v)
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiBase()+"/send", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Authorization", p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("simplemessage: fonnte send: %w", err)
	}
	defer resp.Body.Close()

	var out struct {
		Status bool   `json:"status"`
		Detail string `json:"detail"`
		ID     string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("simplemessage: fonnte decode: %w", err)
	}
	if !out.Status {
		return nil, fmt.Errorf("simplemessage: fonnte: %s", out.Detail)
	}
	messageID := out.ID
	if messageID == "" {
		messageID = out.Detail
	}
	channel := req.Channel
	if channel == "" {
		channel = ChannelWhatsApp
	}
	return &SendResponse{
		MessageID: messageID,
		Status:    "sent",
		Channel:   channel,
		CostCents: 0, // rate applied by consuming app's rate table
		Currency:  "IDR",
		Code:      code,
	}, nil
}

func (p *indonesiaProvider) GetStatus(context.Context, string) (*MessageStatus, error) {
	return nil, fmt.Errorf("simplemessage: %w: fonnte status via device webhooks", ErrNotSupported)
}

// GetBalance returns the Fonnte account balance (IDR).
func (p *indonesiaProvider) GetBalance(ctx context.Context) (*BalanceResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, p.apiBase()+"/getbalance", nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("simplemessage: fonnte balance: %w", err)
	}
	defer resp.Body.Close()

	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("simplemessage: fonnte balance decode: %w", err)
	}
	bal := &BalanceResponse{Currency: "IDR", Raw: out}
	switch v := out["balance"].(type) {
	case float64:
		bal.Balance = int64(v)
	case string:
		bal.Balance = strToInt64(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			bal.Balance = n
		}
	}
	return bal, nil
}

// VerifyWebhook parses a Fonnte device webhook. When SecretKey is set it must
// match the Authorization header.
func (p *indonesiaProvider) VerifyWebhook(body []byte, headers http.Header) (*WebhookEvent, error) {
	if p.cfg.SecretKey != "" {
		got := headers.Get("Authorization")
		if subtle.ConstantTimeCompare([]byte(p.cfg.SecretKey), []byte(got)) != 1 {
			return nil, fmt.Errorf("simplemessage: fonnte webhook token mismatch")
		}
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("simplemessage: fonnte webhook: %w", err)
	}
	id := ""
	if v, ok := payload["id"].(string); ok {
		id = v
	}
	eventType := "message.status"
	if _, ok := payload["message"]; ok {
		eventType = "message.received"
	}
	return &WebhookEvent{Type: eventType, ID: id, Data: payload, Raw: body}, nil
}

func strToInt64(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}
