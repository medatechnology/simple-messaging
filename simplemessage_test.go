package simplemessage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

// testClient builds a Client with one provider backed by an httptest server.
func newTestClient(t *testing.T, cfg ProviderConfig, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(srv.Close)
	cfg.BaseURL = srv.URL
	c, err := New(Config{Providers: map[string]ProviderConfig{cfg.Channel: cfg}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

var digitCode = regexp.MustCompile(`^\d{6}$`)

func TestPrepareMessageGeneratesGoutilOTP(t *testing.T) {
	code, subject, body := prepareMessage(&SendRequest{MessageType: MessageTypeOTP, ExpiresIn: 5 * time.Minute})
	if !digitCode.MatchString(code) {
		t.Fatalf("generated code %q is not 6 digits", code)
	}
	if subject == "" || !strings.Contains(body, code) {
		t.Fatalf("subject=%q body=%q", subject, body)
	}

	// Provided code is preserved and {code} is substituted.
	code2, _, body2 := prepareMessage(&SendRequest{MessageType: MessageTypeOTP, Code: "123456", Body: "Enter {code} now"})
	if code2 != "123456" || body2 != "Enter 123456 now" {
		t.Fatalf("code2=%q body2=%q", code2, body2)
	}

	// Custom length.
	code3, _, _ := prepareMessage(&SendRequest{MessageType: MessageTypeOTP, CodeLength: 4})
	if !regexp.MustCompile(`^\d{4}$`).MatchString(code3) {
		t.Fatalf("code3=%q not 4 digits", code3)
	}
}

func TestNewNoProviders(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected error with no providers")
	}
}

func TestUnconfiguredChannel(t *testing.T) {
	c, err := New(Config{Providers: map[string]ProviderConfig{
		"email": {Channel: "email", From: "a@b.c"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.SendOTP(context.Background(), &SendRequest{Channel: ChannelSMS, To: "+628"}); err == nil {
		t.Fatal("expected error for unconfigured sms channel")
	}
}

func TestEmailProviderValidation(t *testing.T) {
	if _, err := New(Config{Providers: map[string]ProviderConfig{
		"email": {Channel: "email"},
	}}); err == nil {
		t.Fatal("expected From required")
	}
}

func TestEmailSendUnreachableSMTP(t *testing.T) {
	c, _ := New(Config{Providers: map[string]ProviderConfig{
		"email": {Channel: "email", From: "noreply@sureauth.app", BaseURL: "127.0.0.1:1"}, // closed port
	}})
	if _, err := c.SendOTP(context.Background(), &SendRequest{Channel: ChannelEmail, To: "x@y.z"}); err == nil {
		t.Fatal("expected SMTP connection error")
	}
}

func TestEmailMIMEBuild(t *testing.T) {
	msg := buildMIMEMessage("a@b.c", "x@y.z", "Sub", "Body")
	for _, want := range []string{"From: a@b.c", "To: x@y.z", "Subject: Sub", "text/plain", "Body"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message missing %q:\n%s", want, msg)
		}
	}
}

func TestTelegramSend(t *testing.T) {
	var captured map[string]interface{}
	c := newTestClient(t, ProviderConfig{Channel: "telegram", From: "bot-token"}, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sendMessage" {
			t.Errorf("path = %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&captured)
		w.Write([]byte(`{"ok":true,"result":{"message_id":42}}`))
	})
	resp, err := c.SendOTP(context.Background(), &SendRequest{Channel: ChannelTelegram, To: "12345"})
	if err != nil {
		t.Fatalf("SendOTP: %v", err)
	}
	if resp.MessageID != "42" || !digitCode.MatchString(resp.Code) {
		t.Fatalf("resp = %+v", resp)
	}
	if captured["chat_id"] != "12345" || !strings.Contains(captured["text"].(string), resp.Code) {
		t.Fatalf("payload = %v", captured)
	}
}

func TestTelegramWebhook(t *testing.T) {
	c := newTestClient(t, ProviderConfig{Channel: "telegram", From: "bot-token", SecretKey: "wh-secret"}, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no call expected")
	})
	headers := http.Header{}
	headers.Set("X-Telegram-Bot-Api-Secret-Token", "wh-secret")
	ev, err := c.VerifyWebhook(ChannelTelegram, []byte(`{"update_id":7,"message":{"text":"hi"}}`), headers)
	if err != nil {
		t.Fatalf("VerifyWebhook: %v", err)
	}
	if ev.Type != "message.received" || ev.ID != "7" {
		t.Fatalf("event = %+v", ev)
	}
	if _, err := c.VerifyWebhook(ChannelTelegram, []byte(`{"update_id":8}`), http.Header{}); err == nil {
		t.Fatal("expected secret token mismatch")
	}
	if _, err := c.GetBalance(context.Background(), ChannelTelegram); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("GetBalance: expected ErrNotSupported, got %v", err)
	}
}

func TestWhatsAppTextSend(t *testing.T) {
	var captured map[string]interface{}
	c := newTestClient(t, ProviderConfig{Channel: "whatsapp", From: "123456", APIKey: "wa-token"}, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/123456/messages" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer wa-token" {
			t.Errorf("auth = %q", got)
		}
		json.NewDecoder(r.Body).Decode(&captured)
		w.Write([]byte(`{"messaging_product":"whatsapp","contacts":[{"wa_id":"62812"}],"messages":[{"id":"wamid.1"}]}`))
	})
	resp, err := c.SendOTP(context.Background(), &SendRequest{Channel: ChannelWhatsApp, To: "62812"})
	if err != nil {
		t.Fatalf("SendOTP: %v", err)
	}
	if resp.MessageID != "wamid.1" {
		t.Fatalf("resp = %+v", resp)
	}
	if captured["to"] != "62812" || captured["type"] != "text" {
		t.Fatalf("payload = %v", captured)
	}
	text := captured["text"].(map[string]interface{})
	if !strings.Contains(text["body"].(string), resp.Code) {
		t.Fatalf("text = %v", text)
	}
}

func TestWhatsAppTemplateSend(t *testing.T) {
	var captured map[string]interface{}
	c := newTestClient(t, ProviderConfig{Channel: "whatsapp", From: "123456", APIKey: "wa-token"}, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&captured)
		w.Write([]byte(`{"messages":[{"id":"wamid.2"}]}`))
	})
	_, err := c.SendMessage(context.Background(), &SendRequest{
		Channel: ChannelWhatsApp, To: "62812", MessageType: MessageTypeWelcome,
		TemplateName: "welcome_msg", TemplateParams: map[string]string{"1": "John"},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if captured["type"] != "template" {
		t.Fatalf("payload = %v", captured)
	}
	tmpl := captured["template"].(map[string]interface{})
	if tmpl["name"] != "welcome_msg" {
		t.Fatalf("template = %v", tmpl)
	}
}

func TestWhatsAppWebhook(t *testing.T) {
	c := newTestClient(t, ProviderConfig{Channel: "whatsapp", From: "123456", APIKey: "wa-token", SecretKey: "appsecret"}, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no call expected")
	})
	body := []byte(`{"object":"whatsapp_business_account","entry":[{"changes":[{"field":"messages","value":{"id":"wamid.9","messages":[{"id":"wamid.9","from":"62812"}]}}]}]}`)
	mac := hmac.New(sha256.New, []byte("appsecret"))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	headers := http.Header{}
	headers.Set("X-Hub-Signature-256", sig)
	ev, err := c.VerifyWebhook(ChannelWhatsApp, body, headers)
	if err != nil {
		t.Fatalf("VerifyWebhook: %v", err)
	}
	if ev.Type != "message.received" || ev.ID == "" {
		t.Fatalf("event = %+v", ev)
	}
	bad := http.Header{}
	bad.Set("X-Hub-Signature-256", "sha256=deadbeef")
	if _, err := c.VerifyWebhook(ChannelWhatsApp, body, bad); err == nil {
		t.Fatal("expected signature mismatch")
	}
}

func TestFonnteSend(t *testing.T) {
	var formBody string
	c := newTestClient(t, ProviderConfig{Channel: "sms", APIKey: "fonnte-token"}, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/send" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "fonnte-token" {
			t.Errorf("auth = %q", got)
		}
		r.ParseForm()
		formBody = r.Form.Get("target") + "|" + r.Form.Get("message")
		w.Write([]byte(`{"status":true,"detail":"sent","id":"fx-1"}`))
	})
	resp, err := c.SendOTP(context.Background(), &SendRequest{Channel: ChannelSMS, To: "62812345678"})
	if err != nil {
		t.Fatalf("SendOTP: %v", err)
	}
	if resp.MessageID != "fx-1" || resp.Status != "sent" {
		t.Fatalf("resp = %+v", resp)
	}
	if !strings.HasPrefix(formBody, "62812345678|") || !strings.Contains(formBody, resp.Code) {
		t.Fatalf("form = %q (code %q)", formBody, resp.Code)
	}
}

func TestFonnteBalance(t *testing.T) {
	c := newTestClient(t, ProviderConfig{Channel: "sms", APIKey: "fonnte-token"}, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/getbalance" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Write([]byte(`{"status":true,"balance":"12500"}`))
	})
	bal, err := c.GetBalance(context.Background(), ChannelSMS)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if bal.Balance != 12500 || bal.Currency != "IDR" {
		t.Fatalf("bal = %+v", bal)
	}
}

func TestFonnteWebhook(t *testing.T) {
	c := newTestClient(t, ProviderConfig{Channel: "sms", APIKey: "fonnte-token", SecretKey: "wh-token"}, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no call expected")
	})
	headers := http.Header{}
	headers.Set("Authorization", "wh-token")
	ev, err := c.VerifyWebhook(ChannelSMS, []byte(`{"id":"fx-9","status":"delivered"}`), headers)
	if err != nil {
		t.Fatalf("VerifyWebhook: %v", err)
	}
	if ev.Type != "message.status" || ev.ID != "fx-9" {
		t.Fatalf("event = %+v", ev)
	}
	if _, err := c.VerifyWebhook(ChannelSMS, []byte(`{"id":"fx-9"}`), http.Header{}); err == nil {
		t.Fatal("expected token mismatch")
	}
}
