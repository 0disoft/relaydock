# Local Expert Store

## 목적

데스크톱과 headless 실행은 PostgreSQL 없이도 consultation과 ContextPack을 잃지 않아야 한다. 동시에 큰 source excerpt를 consultation 상태가 바뀔 때마다 하나의 JSON에 다시 쓰면 안 된다. 로컬 저장소 v3는 **작은 원자 metadata 파일**과 **불변 content-addressed chunk**를 분리한다.

## 파일 배치

```text
expert-state.json
expert-state.json.chunks/
└── ab/
    └── cdef...        # 전체 digest는 sha256:abcdef...
```

metadata에는 consultation, idempotency record, ContextPack manifest, 결과와 각 source content의 chunk reference만 들어간다. source content는 최대 32 KiB 단위로 나누고 SHA-256으로 주소를 정한다. 동일 내용은 여러 ContextPack이 재사용한다.

## 쓰기 계약

ContextPack chunk는 먼저 불변 파일로 기록한다. 그 뒤 ContextPack reference와 consultation을 하나의 metadata transaction으로 원자 교체한다. 중간에 프로세스가 죽으면 metadata가 가리키지 않는 chunk만 남을 수 있으며, 이 chunk는 compaction grace period 이후 제거된다. 반대로 metadata가 존재하는데 chunk가 없거나 크기·digest가 맞지 않으면 store open과 ContextPack read가 실패한다.

동일 ContextPack ID에 동일 payload를 다시 쓰는 것은 멱등 성공이다. 같은 ID에 다른 payload를 쓰면 conflict다. consultation 생성도 idempotency scope와 fingerprint를 함께 검사한다.

ContextPack write와 read는 chunk lifecycle의 shared lock을 잡고, destructive compaction은 exclusive lock을 잡는다. 따라서 오래전에 만들어졌지만 아직 metadata에 연결되기 직전인 deduplicated chunk를 compactor가 orphan으로 오인해 지우거나, read 도중 live chunk가 사라지는 경쟁을 허용하지 않는다.

## v1·v2 migration

예전 store는 ContextPack source를 metadata JSON 안에 직접 넣었다. v3 open은 다음 순서로 migration한다.

```text
legacy JSON decode
→ ContextPack content chunk 작성
→ 전체 reference 검증
→ 원본을 .v2.bak으로 보존
→ v3 metadata 원자 교체
```

backup이 만들어지지 않거나 v3 metadata publish가 실패하면 open은 성공으로 처리하지 않는다. migration 이후 backup은 자동 삭제하지 않는다. v3 open은 reference의 크기만 보는 게 아니라 각 chunk를 읽어 SHA-256을 다시 계산하므로, 같은 크기로 덮어쓴 손상도 시작 단계에서 거절한다.

## Compaction

`expertstorectl compact`는 두 계층을 순서대로 정리한다.

1. retention보다 오래된 terminal consultation을 제거한다.
2. consultation이 가리키지 않는 idempotency record를 제거한다.
3. 옵션이 켜져 있으면 orphan ContextPack과 result를 grace period 이후 제거한다.
4. 최종 live chunk 집합을 계산하고 orphan chunk를 grace period 이후 제거한다.

```powershell
go run ./cmd/expertstorectl stats
go run ./cmd/expertstorectl compact --dry-run
go run ./cmd/expertstorectl compact `
  --terminal-retention 720h `
  --orphan-grace 24h `
  --remove-orphans=true
```

`--dry-run`은 metadata와 chunk를 바꾸지 않는다. grace period를 0으로 설정하면 방금 발생한 중단 쓰기의 orphan chunk까지 바로 제거할 수 있으므로 운영 기본값으로 사용하지 않는다.

## 장애 경계

- metadata 파일과 chunk directory는 같은 로컬 파일시스템에 둔다.
- store 파일과 chunk는 사용자 전용 권한으로 만든다.
- 네트워크 공유 폴더의 rename·flush 의미는 보장하지 않으므로 지원 경로로 간주하지 않는다.
- antivirus가 chunk rename을 일시적으로 막으면 consultation 생성은 실패해야 하며 빈 성공 결과를 반환하면 안 된다.
- 사용자가 source file을 수정해도 기존 ContextPack chunk는 바뀌지 않는다. 새 작업은 새 digest와 ContextPack ID를 만든다.
