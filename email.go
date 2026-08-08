package simplemessage

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"github.com/medatechnology/goutil/encryption"
)

// emailProvider delivers messages via SMTP.
// Config: From = sender address, APIKey = SMTP username, SecretKey = SMTP
// password, BaseURL = "host:port" (default smtp.gmail.com:587).
func init() {
	Register("email", newEmailProvider)
}

type emailProvider struct {
	cfg ProviderConfig
}

func newEmailProvider(cfg ProviderConfig, _ *http.Client) (Provider, error) {
	if cfg.From == "" {
		return nil, fmt.Errorf("simplemessage: email requires From (sender address)")
	}
	if !strings.Contains(cfg.From, "@") {
		return nil, fmt.Errorf("simplemessage: email From must be a valid address, got %q", cfg.From)
	}
	return &emailProvider{cfg: cfg}, nil
}

func (p *emailProvider) Name() string { return "email" }

func (p *emailProvider) Send(ctx context.Context, req *SendRequest) (*SendResponse, error) {
	if req.To == "" {
		return nil, fmt.Errorf("simplemessage: email Send requires To")
	}
	code, subject, body := prepareMessage(req)
	// Attach the code to the response so the caller can store/verify it
	// without reading vendor responses.
	if err := p.sendEmail(ctx, req.To, subject, body); err != nil {
		return nil, err
	}
	return &SendResponse{
		MessageID: "eml_" + encryption.NewRandomToken()[:16],
		Status:    "sent",
		Channel:   ChannelEmail,
		CostCents: 0,
		Currency:  "USD",
		Code:      code,
	}, nil
}

func (p *emailProvider) sendEmail(ctx context.Context, to, subject, body string) error {
	hostPort := p.cfg.BaseURL
	if hostPort == "" {
		hostPort = "smtp.gmail.com:587"
	}
	host, _, err := splitHostPort(hostPort)
	if err != nil {
		return fmt.Errorf("simplemessage: email host %q: %w", hostPort, err)
	}

	msg := buildMIMEMessage(p.cfg.From, to, subject, body)

	var auth smtp.Auth
	if p.cfg.APIKey != "" || p.cfg.SecretKey != "" {
		auth = smtp.PlainAuth("", p.cfg.APIKey, p.cfg.SecretKey, host)
	}

	// Dial with a context-aware timeout; smtp.SendMail has no dial timeout
	// (blocks forever on unreachable hosts).
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", hostPort)
	if err != nil {
		return fmt.Errorf("simplemessage: email dial %s: %w", hostPort, err)
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("simplemessage: email smtp handshake: %w", err)
	}
	defer c.Close()

	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("simplemessage: email auth: %w", err)
		}
	}
	if err := c.Mail(p.cfg.From); err != nil {
		return fmt.Errorf("simplemessage: email MAIL: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("simplemessage: email RCPT: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("simplemessage: email DATA: %w", err)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("simplemessage: email write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("simplemessage: email data close: %w", err)
	}
	_ = c.Quit()
	return nil
}

func splitHostPort(hostPort string) (string, string, error) {
	idx := strings.LastIndex(hostPort, ":")
	if idx < 0 {
		return hostPort, "", nil
	}
	return hostPort[:idx], hostPort[idx+1:], nil
}

// buildMIMEMessage renders a minimal RFC 5322 message with UTF-8 content type.
func buildMIMEMessage(from, to, subject, body string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: text/plain; charset=UTF-8\r\n")
	fmt.Fprintf(&b, "Content-Transfer-Encoding: 8bit\r\n")
	fmt.Fprintf(&b, "\r\n%s\r\n", body)
	return b.String()
}

func (p *emailProvider) GetStatus(context.Context, string) (*MessageStatus, error) {
	return nil, fmt.Errorf("simplemessage: %w: email has no delivery status API", ErrNotSupported)
}

func (p *emailProvider) GetBalance(context.Context) (*BalanceResponse, error) {
	return nil, fmt.Errorf("simplemessage: %w: email has no balance", ErrNotSupported)
}

func (p *emailProvider) VerifyWebhook([]byte, http.Header) (*WebhookEvent, error) {
	return nil, fmt.Errorf("simplemessage: %w: email has no webhooks", ErrNotSupported)
}
