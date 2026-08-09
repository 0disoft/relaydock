# 13. Observability

## 기본 신호

- request ID
- tenant/project
- ingress protocol
- virtual model
- provider/model
- route revision
- attempt
- TTFT
- first semantic event
- inter-token latency
- output TPS
- cancellation latency
- usage dimension
- estimated/provider cost
- loss report
- retry phase

## 민감정보

prompt, response, tool argument, ContextPack content는 기본 trace/log에 넣지 않는다.

## 중요 metric

- auth latency
- snapshot lookup
- route decision
- provider queue wait
- connection latency
- TTFT
- partial stream rate
- retry-before-semantic rate
- post-semantic failure rate
- usage mismatch
- capture conflict
- stale ContextPack result
