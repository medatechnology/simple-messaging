package simplemessage

import (
	"context"
	"fmt"
	"net/http"
)

// ProviderConfig carries the credentials for one provider instance. The
// Channel field selects which provider a ProviderConfig describes.
type ProviderConfig struct {
	Channel   string // "email" | "whatsapp" | "sms" | "telegram"
	APIKey    string // vendor token / SMTP password / Meta access token
	SecretKey string // extra secret (Meta app secret, webhook token)
	From      string // email sender address / WhatsApp phone-number-id / bot token
	Sandbox   bool
	// BaseURL overrides the default API base URL (mainly for tests/proxies).
	BaseURL string
}

// Config configures the simplemessage Client.
type Config struct {
	// Providers maps channel name -> provider config. At least one entry.
	Providers map[string]ProviderConfig
	HTTPClient *http.Client
}

// Client is the facade over the configured messaging providers.
type Client struct {
	providers map[string]Provider
}

// providerRegistry maps provider keys to constructors; implementations
// register themselves in init().
var providerRegistry = map[string]func(cfg ProviderConfig, httpClient *http.Client) (Provider, error){}

// Register adds a provider constructor to the registry (called by vendor
// implementations in their init()).
func Register(name string, ctor func(cfg ProviderConfig, httpClient *http.Client) (Provider, error)) {
	providerRegistry[name] = ctor
}

// New builds a Client from cfg, constructing one provider per configured
// channel.
func New(cfg Config) (*Client, error) {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	c := &Client{providers: map[string]Provider{}}
	for channel, pcfg := range cfg.Providers {
		if pcfg.Channel == "" {
			pcfg.Channel = channel
		}
		ctor, ok := providerRegistry[pcfg.Channel]
		if !ok {
			return nil, fmt.Errorf("simplemessage: unknown channel %q (registered: email, whatsapp, sms, telegram)", pcfg.Channel)
		}
		p, err := ctor(pcfg, httpClient)
		if err != nil {
			return nil, fmt.Errorf("simplemessage: init %q: %w", pcfg.Channel, err)
		}
		c.providers[channel] = p
	}
	if len(c.providers) == 0 {
		return nil, fmt.Errorf("simplemessage: no providers configured")
	}
	return c, nil
}

// Provider returns the provider for a channel, or nil.
func (c *Client) Provider(channel Channel) Provider { return c.providers[string(channel)] }

// Channels returns the configured channel names.
func (c *Client) Channels() []string {
	out := make([]string, 0, len(c.providers))
	for k := range c.providers {
		out = append(out, k)
	}
	return out
}

// SendOTP delivers a one-time password to the requested channel, generating
// the code via goutil/encryption when the request does not carry one.
func (c *Client) SendOTP(ctx context.Context, req *SendRequest) (*SendResponse, error) {
	if req.MessageType == "" {
		req.MessageType = MessageTypeOTP
	}
	p, err := c.providerFor(req.Channel)
	if err != nil {
		return nil, err
	}
	return p.Send(ctx, req)
}

// SendMessage delivers a non-OTP message (confirmation, registration link,
// welcome message, notification...).
func (c *Client) SendMessage(ctx context.Context, req *SendRequest) (*SendResponse, error) {
	if req.MessageType == "" {
		req.MessageType = MessageTypeNotification
	}
	p, err := c.providerFor(req.Channel)
	if err != nil {
		return nil, err
	}
	return p.Send(ctx, req)
}

// Send routes by channel without overriding the message type.
func (c *Client) Send(ctx context.Context, req *SendRequest) (*SendResponse, error) {
	p, err := c.providerFor(req.Channel)
	if err != nil {
		return nil, err
	}
	return p.Send(ctx, req)
}

// GetStatus delegates to the channel's provider.
func (c *Client) GetStatus(ctx context.Context, channel Channel, messageID string) (*MessageStatus, error) {
	p, err := c.providerFor(channel)
	if err != nil {
		return nil, err
	}
	return p.GetStatus(ctx, messageID)
}

// GetBalance delegates to the channel's provider (ErrNotSupported where N/A).
func (c *Client) GetBalance(ctx context.Context, channel Channel) (*BalanceResponse, error) {
	p, err := c.providerFor(channel)
	if err != nil {
		return nil, err
	}
	return p.GetBalance(ctx)
}

// VerifyWebhook delegates to the channel's provider.
func (c *Client) VerifyWebhook(channel Channel, body []byte, headers http.Header) (*WebhookEvent, error) {
	p, err := c.providerFor(channel)
	if err != nil {
		return nil, err
	}
	return p.VerifyWebhook(body, headers)
}

func (c *Client) providerFor(channel Channel) (Provider, error) {
	p, ok := c.providers[string(channel)]
	if !ok {
		return nil, fmt.Errorf("simplemessage: channel %q is not configured (configured: %v)", channel, c.Channels())
	}
	return p, nil
}
