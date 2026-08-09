package distributedlease

// AcquireScript returns the lease expiry as Unix milliseconds, -1 when the
// account is at capacity, or -2 on the practically impossible member collision.
// Server time is used so gateway host clock skew cannot create overlapping
// leases.
const AcquireScript = `
local clock = redis.call('TIME')
local now_ms = (tonumber(clock[1]) * 1000) + math.floor(tonumber(clock[2]) / 1000)
local ttl_ms = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local member = ARGV[3]
local grace_ms = tonumber(ARGV[4])

redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now_ms)
if redis.call('ZSCORE', KEYS[1], member) then
  return -2
end
if redis.call('ZCARD', KEYS[1]) >= limit then
  return -1
end

local expires_ms = now_ms + ttl_ms
redis.call('ZADD', KEYS[1], expires_ms, member)
local tail = redis.call('ZREVRANGE', KEYS[1], 0, 0, 'WITHSCORES')
if #tail >= 2 then
  redis.call('PEXPIREAT', KEYS[1], tonumber(tail[2]) + grace_ms)
end
return expires_ms
`

// RenewScript returns the new expiry as Unix milliseconds or zero when the
// lease has expired, disappeared, or was released by another process.
const RenewScript = `
local clock = redis.call('TIME')
local now_ms = (tonumber(clock[1]) * 1000) + math.floor(tonumber(clock[2]) / 1000)
local ttl_ms = tonumber(ARGV[1])
local member = ARGV[2]
local grace_ms = tonumber(ARGV[3])

redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now_ms)
if not redis.call('ZSCORE', KEYS[1], member) then
  return 0
end

local expires_ms = now_ms + ttl_ms
redis.call('ZADD', KEYS[1], expires_ms, member)
local tail = redis.call('ZREVRANGE', KEYS[1], 0, 0, 'WITHSCORES')
if #tail >= 2 then
  redis.call('PEXPIREAT', KEYS[1], tonumber(tail[2]) + grace_ms)
end
return expires_ms
`

// ReleaseScript is idempotent. It removes the member and either deletes the
// empty sorted set or shortens the key lifetime to the latest remaining lease.
const ReleaseScript = `
local member = ARGV[1]
local grace_ms = tonumber(ARGV[2])
local removed = redis.call('ZREM', KEYS[1], member)
local tail = redis.call('ZREVRANGE', KEYS[1], 0, 0, 'WITHSCORES')
if #tail == 0 then
  redis.call('DEL', KEYS[1])
else
  redis.call('PEXPIREAT', KEYS[1], tonumber(tail[2]) + grace_ms)
end
return removed
`
