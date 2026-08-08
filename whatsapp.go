package simplemessage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// whatsappProvider delivers messages via the Meta WhatsApp Business Cloud API.
// Config: From = phone-number-id, APIKey = permanent access token,
// SecretKey = app secret (webhook signature), BaseURL = graph host override.
func init() {
	Register("whatsapp", newWhatsAppProvider)
}

type whatsappProvider struct {
	cfg    ProviderConfig
	client *http.Client
}

func newWhatsAppProvider(cfg ProviderConfig, httpClient *http.Client) (Provider, error) {
	if cfg.From == "" {
		return nil, fmt.Errorf("simplemessage: whatsapp requires From (phone-number-id)")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("simplemessage: whatsapp requires APIKey (permanent access token)")
	}
	return &whatsappProvider{cfg: cfg, client: httpClient}, nil
}

func (p *whatsappProvider) Name() string { return "whatsapp" }

func (p *whatsappProvider) graphBase() string {
	if p.cfg.BaseURL != "" {
		return p.cfg.BaseURL
	}
	return "https://graph.facebook.com/v21.0"
}

func (p *whatsappProvider) Send(ctx context.Context, req *SendRequest) (*SendResponse, error) {
	if req.To == "" {
		return nil, fmt.Errorf("simplemessage: whatsapp Send requires To (phone number)")
	}
	code, _, body := prepareMessage(req)

	var payload map[string]interface{}
	if req.TemplateName != "" {
		components := []map[string]interface{}{}
		if len(req.TemplateParams) > 0 {
			params := []map[string]interface{}{}
			for _, v := range req.TemplateParams {
				params = append(params, map[string]interface{}{"type": "text", "text": v})
			}
			components = append(components, map[string]interface{}{"type": "body", "parameters": params})
		}
		payload = map[string]interface{}{
			"messaging_product": "whatsapp",
			"to":                req.To,
			"type":              "template",
			"template": map[string]interface{}{
				"name":       req.TemplateName,
				"language":   map[string]interface{}{"code": "id"},
				"components": components,
			},
		}
	} else {
		payload = map[string]interface{}{
			"messaging_product": "whatsapp",
			"to":                req.To,
			"type":              "text",
			"text":              map[string]interface{}{"preview_url": false, "body": body},
		}
	}

	var out struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
		Error *struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	if err := doJSON(ctx, p.client, http.MethodPost, p.graphBase()+"/"+p.cfg.From+"/messages", payload, &out, &map[string]interface{}{}, p.bearerAuth); err != nil {
		return nil, err
	}
	if out.Error != nil {
		return nil, fmt.Errorf("simplemessage: whatsapp: %s (code %d)", out.Error.Message, out.Error.Code)
	}
	if len(out.Messages) == 0 {
		return nil, fmt.Errorf("simplemessage: whatsapp: empty message response")
	}
	return &SendResponse{
		MessageID: out.Messages[0].ID,
		Status:    "sent",
		Channel:   ChannelWhatsApp,
		Code:      code,
	}, nil
}

func (p *whatsappProvider) bearerAuth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
}

func (p *whatsappProvider) GetStatus(context.Context, string) (*MessageStatus, error) {
	return nil, fmt.Errorf("simplemessage: %w: whatsapp status requires app-level webhooks", ErrNotSupported)
}

func (p *whatsappProvider) GetBalance(context.Context) (*BalanceResponse, error) {
	return nil, fmt.Errorf("simplemessage: %w: whatsapp has no balance API", ErrNotSupported)
}

// VerifyWebhook validates the X-Hub-Signature-256 header (HMAC-SHA256 of the
// body with the app secret) and parses a WhatsApp webhook payload.
func (p *whatsappProvider) VerifyWebhook(body []byte, headers http.Header) (*WebhookEvent, error) {
	if p.cfg.SecretKey != "" {
		sig := headers.Get("X-Hub-Signature-256")
		if !strings.HasPrefix(sig, "sha256=") {
			return nil, fmt.Errorf("simplemessage: whatsapp: missing X-Hub-Signature-256")
		}
		mac := hmac.New(sha256.New, []byte(p.cfg.SecretKey))
		mac.Write(body)
		expected := hex.EncodeToString(mac.Sum(nil))
		if subtle.ConstantTimeCompare([]byte(expected), []byte(strings.TrimPrefix(sig, "sha256="))) != 1 {
			return nil, fmt.Errorf("simplemessage: whatsapp: webhook signature mismatch")
		}
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("simplemessage: whatsapp webhook: %w", err)
	}
	entry, _ := payload["entry"].([]interface{})
	eventType := "message.status"
	id := ""
	for _, e := range entry {
		emap, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		changes, _ := emap["changes"].([]interface{})
		for _, c := range changes {
			cmap, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			field, _ := cmap["field"].(string)
			if field == "messages" {
				eventType = "message.received"
			}
			value, _ := cmap["value"].(map[string]interface{})
			if v, ok := value["id"].(string); ok && v != "" {
				id = v
			}
			if id == "" {
				if msgs, ok := value["messages"].([]interface{}); ok && len(msgs) > 0 {
					if m, ok := msgs[0].(map[string]interface{}); ok {
						if mid, ok := m["id"].(string); ok {
							id = mid
						}
					}
				}
			}
		}
	}
	return &WebhookEvent{Type: eventType, ID: id, Data: payload, Raw: body}, nil
}
