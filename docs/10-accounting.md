# 10. Accounting

## 분리할 객체

```text
Request
  └─ ProviderAttempt*
       └─ UsageEvent*

CustomerCharge
  ├─ AuthorizationHold
  ├─ Capture
  ├─ Release
  └─ Adjustment
```

provider attempt가 여러 개여도 customer request는 하나다.

## 흐름

```text
quote
→ authorize hold
→ provisional usage
→ final usage
→ capture
→ release remainder
→ reconciliation
→ adjustment
```

## usage dimension

- uncached input
- cache write
- cache read
- visible output
- reasoning output
- image input/output
- audio seconds
- tool fees
- web search
- service tier surcharge
- provider reported total
- locally estimated total

## 불변식

- 같은 idempotency key는 한 번만 capture
- price revision은 request 시작 시 고정
- 원본 ledger entry 수정 금지
- provider invoice 차이는 adjustment로 기록
- Valkey 삭제가 balance를 바꾸면 안 됨
