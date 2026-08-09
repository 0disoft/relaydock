# Repository Size and Source Release

## 40 KiB 정책

저장소의 regular file은 기본적으로 40 KiB 이하로 유지한다. 목적은 임의 숫자 맞추기가 아니라 다음 문제를 차단하는 데 있다.

- 서로 다른 책임이 한 파일에 쌓이는 god file
- 검토하기 어려운 거대한 generated manifest
- prompt와 code-review context를 불필요하게 점유하는 문서
- 저장소 안에 실수로 들어온 build artifact와 binary fixture

`config/file-size-exceptions.json`은 실제로 분할할 수 없는 파일만 허용한다. 여기에는 byte identity가 계약인 upstream fixture와 package manager가 단일 파일로 생성하는 canonical lockfile이 포함될 수 있다.

source scan은 루트 `build/`의 packaging source는 보존하지만 nested `dist/`, nested `build/`, `.svelte-kit/`, `.svelte-check/`과 `node_modules/` 같은 재생성 가능한 frontend output은 포함하지 않는다.

```json
{
  "version": 1,
  "maxBytes": 40960,
  "exceptions": [
    {
      "path": "tests/fixtures/vendor-binary.dat",
      "reason": "upstream conformance fixture; byte identity is part of the test"
    }
  ]
}
```

예외에는 경로와 구체적 이유가 모두 필요하다. 파일이 삭제됐는데 예외만 남거나, 같은 경로가 중복되거나, 이유가 비어 있으면 audit이 실패한다. symlink·socket·device 같은 비정규 파일은 조용히 archive에서 빠뜨리지 않고 audit 단계에서 실패한다.

```powershell
go run ./cmd/releasepack audit --root .
```

## Manifest 분할

전체 파일 목록을 한 `MANIFEST.json`에 넣으면 저장소가 커질수록 manifest 자체가 40 KiB를 넘는다. 그래서 root index와 정렬된 chunk로 나눈다.

```text
MANIFEST.json
manifest/chunks/files-0001.json
manifest/chunks/files-0002.json
...
```

root index에는 chunk 경로, 범위, 파일 수, 크기, SHA-256과 전체 record aggregate hash를 넣는다. 각 chunk는 기본 30 KiB를 목표로 하며 40 KiB를 넘지 않는다. verifier는 다음을 검사한다.

- root contract version과 hash algorithm
- chunk path 정규화와 중복·범위 겹침
- chunk byte size와 SHA-256
- record 정렬, 경로, mode, 크기, digest
- aggregate hash와 총 파일 수·총 바이트
- 실제 저장소 파일과 manifest record의 완전 일치
- manifest 자신까지 포함한 40 KiB 정책
- root index·chunk·policy JSON의 trailing value와 unknown field

## 재현 가능한 source ZIP

```powershell
go run ./cmd/releasepack build `
  --root . `
  --output ../relaydock-0.5.2-dev-source.zip `
  --generated-at 2026-08-08T12:00:00Z

go run ./cmd/releasepack verify --root .
```

archive output은 저장소 밖에 있어야 한다. 저장소 안에 두면 이전 ZIP이 다음 ZIP의 입력이 되는 자기포함 오류가 생기므로 명시적으로 거절한다.

ZIP entry는 path 순서로 정렬하고 모든 timestamp를 하나로 정규화한다. source를 archive에 복사하기 전후의 mode·size와 복사 중 SHA-256을 다시 확인해 scan 뒤 파일이 바뀌는 경쟁도 감지한다. temporary file은 매 실행마다 고유 이름으로 만들고 완성·flush한 뒤 backup swap으로 교체해 Windows의 rename 제약과 stale `.tmp` 충돌을 피한다.

`TREE.md`와 manifest는 releasepack이 생성한다. `TREE.md`는 자기 자신과 manifest metadata를 목록에서 제외한다. manifest도 자신의 hash를 포함하지 않는다. 이 제외는 순환 참조를 피하기 위한 계약이며 root index의 `excluded` 필드에 기록된다.
