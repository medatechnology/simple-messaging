# HISTORY — simple-messaging

Curated change log for this shared library. Consumers: sureauth (OTP delivery),
controlserver (notifications). Read `AGENTS.md` for contracts.

## Features
| Status | Change | Notes |
|---|---|---|
| ✅ | Telegram forum-topic support (`SendRequest.ThreadID` → `message_thread_id`) | Optional: empty = the chat's general stream, so normal groups are unaffected. Non-numeric thread ids error loudly. |
| ✅ | `ResolveTelegramTopic(ctx, token, chatID, httpClient)` | Returns the most recent `message_thread_id` seen for a chat from `getUpdates`, so setup can auto-fill a topic. Errors clearly when the chat is a normal group. |

## Consumers affected
- **controlserver** (v1.6+): `telegram.topic_id` config + settings + a “fetch” button; the default telegram notification carries `ThreadID`. Backward compatible — an empty topic sends to the chat as before.
- **sureauth**: API-compatible (field addition only); no action required.
