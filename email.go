package simplemessage

import (
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/mail"
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
	if address, err := mail.ParseAddress(cfg.From); err != nil || address.Address != cfg.From || strings.ContainsAny(cfg.From, "\r\n") {
		return nil, fmt.Errorf("simplemessage: email From must be a valid address, got %q", cfg.From)
	}
	return &emailProvider{cfg: cfg}, nil
}

func (p *emailProvider) Name() string { return "email" }

// TLSRequired reports that this SMTP implementation refuses unencrypted delivery.
func (p *emailProvider) TLSRequired() bool { return true }

func (p *emailProvider) Send(ctx context.Context, req *SendRequest) (*SendResponse, error) {
	if address, err := mail.ParseAddress(req.To); err != nil || address.Address != req.To || strings.ContainsAny(req.To+req.Subject, "\r\n") {
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
	host, port, err := net.SplitHostPort(hostPort)
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
	defer conn.Close()
	deadline := time.Now().Add(30 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("simplemessage: email deadline: %w", err)
	}
	rawConn := conn
	stop := context.AfterFunc(ctx, func() { rawConn.Close() })
	defer stop()
	tlsConfig := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	if port == "465" {
		secure := tls.Client(conn, tlsConfig)
		if err := secure.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("simplemessage: email TLS: %w", err)
		}
		conn = secure
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("simplemessage: email smtp handshake: %w", err)
	}
	defer c.Close()

	if port != "465" {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return fmt.Errorf("simplemessage: email requires STARTTLS")
		}
		if err := c.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("simplemessage: email STARTTLS: %w", err)
		}
	}
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

// buildMIMEMessage renders a minimal RFC 5322 message with UTF-8 content type.
func buildMIMEMessage(from, to, subject, body string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
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
