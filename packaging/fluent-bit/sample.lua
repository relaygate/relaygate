-- P2 sampling: keep anomaly flags and short sessions at 100%;
-- otherwise keep with probability LOG_SAMPLE_RATE (default 1.0 = keep all).
-- Env: LOG_SAMPLE_RATE (0.0–1.0], LOG_SHORT_SESSION_MS (default 2000)

math.randomseed(os.time() + (os.clock() * 1000000))

local function env_num(name, default)
  local v = os.getenv(name)
  if v == nil or v == "" then
    return default
  end
  local n = tonumber(v)
  if n == nil then
    return default
  end
  return n
end

function cb_sample(tag, timestamp, record)
  local sample_rate = env_num("LOG_SAMPLE_RATE", 1.0)
  if sample_rate >= 1.0 then
    return 1, timestamp, record
  end
  if sample_rate <= 0 then
    sample_rate = 0.01
  end

  local flags = record["flags"]
  if flags ~= nil and tostring(flags) ~= "-" and tostring(flags) ~= "" then
    return 1, timestamp, record
  end

  local short_ms = env_num("LOG_SHORT_SESSION_MS", 2000)
  local dur = tonumber(record["duration_ms"])
  if dur ~= nil and dur < short_ms then
    return 1, timestamp, record
  end

  if math.random() < sample_rate then
    return 1, timestamp, record
  end
  return -1, timestamp, record
end
