# Signing and Token Key Rotation

## Control Snapshot Ed25519 Rotation

Control snapshots contain `signingKeyId`. The Control Plane signs new revisions only with the active private key, while verifiers may retain retiring public keys. Gateway uses the same trust ring so old and new snapshots overlap without downtime.

```text
1. Generate a new Ed25519 keypair.
2. Register old and new public keys in Control and Gateway trust rings.
3. Switch the Control active private key and CONTROL_SIGNING_KEY_ID.
4. Publish a new revision.
5. Confirm every Gateway applied the snapshot signed by the new key.
6. Remove the old public key after the maximum LKG and snapshot lifetime.
```

Relevant environment variables:

```text
CONTROL_SIGNING_KEY_ID
CONTROL_TRUSTED_SIGNING_PUBLIC_KEYS
CONTROL_ALLOW_LEGACY_SIGNING_KEY_ID

GATEWAY_CONTROL_SIGNING_KEY_ID
GATEWAY_CONTROL_TRUSTED_SIGNING_PUBLIC_KEYS
GATEWAY_CONTROL_ALLOW_LEGACY_SIGNING_KEY_ID
```

Additional trusted keys accept a JSON array or `id=base64,id=base64`. Key IDs are 1-64 ASCII letters, digits, hyphens, or underscores. Reject trailing JSON values. Allow snapshots without `signingKeyId` only during migration, then disable both legacy switches after all snapshots and LKGs contain key IDs.

If Control verifies a persisted snapshot signed by a retiring key, publish the next revision with the active key. Never change only the signature at the same revision.

## Remote MCP HMAC Rotation

Remote MCP token v2 format:

```text
argt2.<key-id>.<base64url-claims>.<base64url-hmac>
```

Issue only with the active key and verify retiring keys from `EXPERT_MCP_TOKEN_VERIFICATION_KEYS_B64`. Values are base64 secrets. Reject normalized key-ID collisions, more than 64 verification keys, secrets over 4 KiB, and oversized subject, scope, identity, or claims payloads.

```powershell
$env:EXPERT_MCP_TOKEN_KEY_ID = "2026-q3"
$env:EXPERT_MCP_TOKEN_SECRET_B64 = "<new-secret>"
$env:EXPERT_MCP_TOKEN_VERIFICATION_KEYS_B64 = '{"2026-q2":"<old-secret>"}'
```

Rotation order:

```text
1. Generate a new HMAC secret and unique key ID.
2. Make it active and add the old key to the retiring map.
3. Issue new-key tokens with mcptokenctl.
4. Remove the old key after maximum token lifetime plus clock skew.
5. Set EXPERT_MCP_TOKEN_ALLOW_LEGACY=false after every argt1 token expires.
```

Because argt1 has no key ID, verify it against every retiring secret. This compatibility path is temporary.

## Virtual-Key Pepper Replacement

The virtual-key pepper is not an online rotation key. RelayDock stores only pepper-derived HMAC values and has no old-pepper verification ring, so changing `GATEWAY_VIRTUAL_KEY_PEPPER`, `GATEWAY_VIRTUAL_KEY_PEPPER_B64`, or the bytes resolved by `GATEWAY_VIRTUAL_KEY_PEPPER_REF` invalidates every existing virtual key.

Use an immutable external-secret version. To replace the pepper, create replacement virtual keys under the new pepper, distribute them through the consumer credential channel, switch every Gateway and `keyctl` deployment as one coordinated cutover, and then revoke the old records. Do not point the reference at `latest` or a mutable alias.

## Prohibitions

- Key IDs are not secrets; logging IDs is acceptable, logging secrets is not.
- Never replace the secret behind an active ID.
- Never distribute an old private key to Gateway; it needs only public keys.
- Do not depend on restarting every process simultaneously. Establish overlapping trust first.
