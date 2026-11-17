# DESCRIPTION — simple-messaging (messaging/OTP wrapper)

> **Read this first.** The shared messaging library of the Meda ecosystem.

## What It Is

`simple-messaging` delivers messages through one interface:
`SendOTP` (one-time passwords) today, `SendMessage` for confirmations,
registration links and welcome messages. Channels: **email** (SMTP),
**WhatsApp** (Meta Cloud API), **Indonesian vendor** (Fonnte: WhatsApp +
SMS), **Telegram**.

## Reuse (mandatory)

- **OTP codes are ALWAYS generated with `goutil/encryption`
  (`GenerateOTP` / `GenerateSecureRandomNumber` / `GenerateDefaultOTP`) —
  never reimplemented** (see `otp.go`). This rule applies to every project
  in the ecosystem that sends OTPs (sureauth, surepayment-saas).
- **Interface + implementations**: master-set `Provider` interface; missing
  capabilities return `ErrNotSupported`.
- **SMTP**: context-aware dial with timeout (`net.Dialer`) — never
  `smtp.SendMail` directly (hangs on unreachable hosts).
- **Metering**: `SendResponse` reports `CostCents`/`Currency` when the
  vendor reports cost; consumers apply their own rate table otherwise
  (sureauth: prepaid deposit metering).
- **Reused by**: sureauth (all OTP delivery), later surepayment-saas
  (notifications).

## Channels

| Channel | Provider | Balance | Status | Webhook |
|---------|----------|---------|--------|---------|
| email | SMTP (From/APIKey=user/SecretKey=pass, BaseURL=host:port) | — | — | — |
| whatsapp | Meta Cloud API (From=phone-number-id, APIKey=token, SecretKey=app secret) | — | — | X-Hub-Signature-256 |
| sms / fonnte | Fonnte (APIKey=token; WhatsApp + SMS, balance API) | ✓ | — | Authorization token |
| telegram | Bot API (From=bot token, SecretKey=webhook token) | — | — | X-Telegram-Bot-Api-Secret-Token |

## Repo Facts

- Module `github.com/medatechnology/simple-messaging`, root package
  `simplemessage`. Deps: `medatechnology/goutil` only.
- Verification: `go build ./...`, `go vet ./...`, `go test ./...` (httptest
  fixtures; no live provider keys).

## SMTP transport security

Email requires encrypted transport: implicit TLS on port 465, STARTTLS on other
ports. Server certificates are verified and TLS 1.2 is the minimum. Servers that
do not advertise STARTTLS are rejected. SMTP username is `APIKey`, password is
`SecretKey`, sender address is `From`, and `BaseURL` is `host:port`.

Connections have a 10-second dial timeout and a 30-second total SMTP deadline,
shortened by the caller's context deadline. Context cancellation closes the
connection during SMTP operations. From/To must be single mailbox addresses;
CR/LF in header fields is rejected. Subjects are UTF-8 MIME encoded when needed.
The email provider exposes `TLSRequired() bool` for consumers that must reject
older implementations lacking mandatory encryption.

For an existing verification link, call `SendMessage` with
`MessageTypeNotification`, a subject, and the complete text body. The verification
message type currently formats an OTP and can replace a body without `{code}`.
The caller owns token generation, storage, expiry, and validation. SMTP acceptance
is not confirmation of inbox delivery and retries can cause duplicate email.

The module resolves its published `goutil` dependency without a sibling checkout.
For local consumer development, use `go work` instead of committing local replace
paths. Publish a library version before consumers pin transport fixes in go.mod.
