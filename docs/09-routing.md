# 09. Routing

## 1단계: hard filter

- protocol capability
- context/output limit
- modality
- tool calling
- structured output
- reasoning preservation
- state continuation
- region
- tenant allowlist
- maximum estimated cost

## 2단계: deterministic score

```text
health
+ quota
+ latency
+ throughput
+ cache affinity
+ session affinity
+ configured weight
- estimated cost
- recent error
- queue pressure
```

## virtual model

사용자는 `code-fast`, `code-deep`, `vision-balanced` 같은 안정 ID를 요청한다. 실제 provider mapping은 revision을 가진다.

## Lease

route decision과 실제 호출 사이에 concurrency slot이 사라지지 않도록 짧은 lease를 획득한다.

Valkey 장애 시 lease 정책은 provider별로 fail-open 또는 fail-closed를 명시한다. customer balance와는 무관하다.
