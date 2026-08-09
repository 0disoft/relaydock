# 04. Request Lifecycle

```text
authenticate
→ budget authorization
→ decode ingress
→ capability requirement extraction
→ candidate hard filter
→ route decision
→ account/provider lease
→ upstream request
→ stream translation
→ usage finalization
→ capture/release
→ reconciliation
```

## Retry 경계

### Pre-semantic retry

사용자에게 text, reasoning, tool call, image chunk 중 아무것도 전달하지 않은 상태다. 동일 요청을 다른 attempt로 재시도할 수 있다.

### Post-semantic failure

첫 semantic event가 전달된 뒤다. 다른 provider로 조용히 갈아타지 않는다.

가능한 처리:

- 공급자가 공식 resume를 지원하면 같은 attempt를 resume
- 클라이언트에 partial failure를 노출
- 사용자가 승인한 explicit continuation 생성

## 취소

클라이언트 취소는 다음 계층으로 전파한다.

```text
client context
→ ingress handler
→ router lease
→ provider request
→ expert job
→ accounting provisional state
```

취소됐다고 이미 발생한 provider 비용이 사라지는 건 아니다. usage와 charge 정책은 별도로 정산한다.
