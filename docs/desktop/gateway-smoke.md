# Desktop gateway smoke

Date: 2026-09-12. Version: `0.5.14-dev`. This is HTTP/runtime evidence,
not a native UI, installer, or production-readiness certification.

## Verified behavior

- The actual desktop `RuntimeService` binds an unused IPv4 loopback port.
- `local/echo` returns assistant text through non-streaming `/v1/responses`
  and streaming `/v1/chat/completions`.
- A controlled HTTP provider receives the selected upstream model rather than
  the public route name. The test failed before the runtime repair.
- A standard non-streaming Chat response retains its message text even when
  `finish_reason` is present. The test exposed an empty response before repair.
- Role-only Chat stream chunks do not terminate the response. A fixture with
  role, reasoning, answer, and terminal chunks failed before the decoder repair.
- Recognized `reasoning_content` and `reasoning` extensions from Chat upstreams
  can be forwarded to Chat streaming clients under their original field names,
  separately from assistant answer text. Untagged or cross-protocol reasoning
  and unsupported tool conversion still fail explicitly.
- With `ENTRIM_API_KEY`, Entrim `Qwen/Qwen3.6-35B-A3B` completed both the
  non-streaming Responses request and the streaming Chat request. HTTP 200 alone
  is insufficient: the smoke requires assistant text, semantic completion,
  and, for Chat streaming, a finish reason and `[DONE]`.
- Stopping the service clears readiness/address and closes its owned port.

The final live run passed in about three seconds. Earlier live runs exposed
incorrect model routing, then empty output and premature streaming completion.
A 256-token smoke budget also produced no final answer; the successful bounded
run used 1,024 output tokens per request. This does not establish a minimum
token budget for all Qwen prompts.

## Credential and call boundaries

The live command allows only `ENTRIM_API_KEY`, plus the host/cache variables
needed to run Go. It copies that key into the compatible-provider environment
inside the test process only. Other provider keys, persisted desktop settings,
OS credential stores, and existing gateway processes are not used or modified.
No prompt context from the workspace is sent. The fixed prompt requests a short
`RelayDock OK` reply. No credential or provider body is printed or stored.

Each live smoke performs at most two inference attempts, with automatic retries
disabled, 1,024 output tokens per attempt, a 60-second client timeout, and a
150-second test timeout. The dedicated command sets `RELAYDOCK_RUN_ENTRIM_SMOKE=1`;
key presence alone does not opt a normal test run into paid calls.
Missing credentials skip the Go test; the dedicated live command instead fails
before testing when the key is absent, preventing a skip from appearing as a
successful live validation.

The trusted endpoint and model come from [Entrim's Qwen documentation](https://entrim.ai/ai-models/qwen/qwen3-6-35b-a3b-api/).
RelayDock's adapter appends `/v1/chat/completions`, so its configured base is
`https://api.entrim.ai`, not the SDK-style `https://api.entrim.ai/v1`.

## Workspace verification intents

- `relaydock_gateway_local_smoke`: isolated loopback and routed-provider checks.
- `relaydock_gateway_regression_test`: desktop, composition, runtime, HTTP
  transport, conformance, and fault-injection tests without external secrets.
- `relaydock_gateway_entrim_smoke`: explicit network/secret approval, fixed
  Entrim origin, and the bounded live test described above.

These intents belong to the parent workspace command contract. A standalone
checkout does not automatically have them installed.

## Manual desktop connection

The smoke does not persist a provider configuration. In the PowerShell session
that will launch the desktop, map the already available key without printing it:

```powershell
$env:OPENAI_COMPATIBLE_BASE_URL = 'https://api.entrim.ai'
$env:OPENAI_COMPATIBLE_API_KEY = $env:ENTRIM_API_KEY
$env:GATEWAY_ROUTES_JSON = '{"qwen":[{"provider":"openai-compatible","model":"Qwen/Qwen3.6-35B-A3B"}]}'
```

This replaces the route JSON for that session. Preserve any existing custom
routes before using this example. Restart the desktop from that session, then
start its local gateway and use model `qwen`. Do not commit a populated `.env`
or place the key literal in shell history. The gateway port remains the user's
saved value; this work does not change the default or bind an existing port.

## Remaining boundaries

- OAuth fallback was not needed or exercised. This does not add Gemini,
  OpenAI, or xAI OAuth login/refresh adapters or reuse web-session credentials.
- Native button interaction, address copy, graceful tray Quit, and persisted
  settings verification remain separate from this headless runtime smoke.
- Live cancellation, authentication-error status mapping, structured tools,
  cross-protocol reasoning, usage/invoice reconciliation, and prolonged load
  remain separate acceptance gates. Local fault tests are not live-provider
  evidence for those behaviors.
- No remote push, release, or deployment is performed by these smoke commands.
