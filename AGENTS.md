# AGENTS.md — simple-messaging

## Purpose
Vendor-agnostic messaging/OTP delivery wrapper (interface + implementations):
`SendOTP` now, `SendMessage` for confirmations/registration links/welcome
messages. Email (SMTP), WhatsApp (Meta Cloud API), Indonesian vendor (Fonnte:
WhatsApp + SMS), Telegram.

## Ownership
- Library tier of the Meda ecosystem. Consumers: sureauth (all OTP delivery +
  deposit metering), later surepayment-saas (notifications).
- **OTP codes are ALWAYS generated with `goutil/encryption`**
  (`GenerateOTP` / `GenerateSecureRandomNumber` / `GenerateDefaultOTP`) —
  never reimplemented. See `otp.go` and DESCRIPTION.md.

## Local Contracts
- **Single package** (`simplemessage`; module path keeps the hyphen):
  implementations in `email.go` / `whatsapp.go` / `indonesia.go` /
  `telegram.go`, self-registered via `init()`.
- **Master-set interface**: `SendOTP`, `SendMessage`, `GetStatus`,
  `GetBalance` (ErrNotSupported where N/A), `VerifyWebhook(body, headers)`.
- `SendResponse` carries `CostCents`/`Currency` when the vendor reports cost,
  and `Code` (the sent OTP) so callers can store/verify it server-side.
- No stdout noise; no tokens/credentials in logs.
- SMTP dial is context-aware with a timeout (`net.Dialer`) — never
  `smtp.SendMail` directly (it hangs on unreachable hosts).

## Work Guidance
- New channel: implement `Provider`, `Register` it, add httptest fixtures
  (send, balance where applicable, webhook verify, unsupported).
- Channel→provider mapping is by name (`email`, `whatsapp`, `sms`,
  `telegram`); a provider may register multiple names (fonnte registers both
  `sms` and `fonnte`).

## Verification
- `go build ./...`, `go vet ./...`, `go test ./...` (httptest fixtures; no
  live provider keys).

## Child DOX Index
Flat library — none.
