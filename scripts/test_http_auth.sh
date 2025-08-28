#!/usr/bin/env bash
set -euo pipefail

# Simple test for HTTP auth: requires curl and timeout

BIN=./mcp-service
ADDR=:8081

if [ ! -x "$BIN" ]; then
  echo "Building binary..."
  go build -o mcp-service .
fi

if [ ! -f config.json ]; then
  if [ -f config.example.json ]; then
    cp config.example.json config.json
  else
    echo "config.json not found and example missing" >&2
    exit 1
  fi
fi

echo "Starting server with HTTP auth..."
HTTP_API_KEY=secret123 timeout 8s $BIN -config config.json -http $ADDR >/tmp/mcp-http.log 2>&1 &
PID=$!
sleep 1

fail=0

code=$(curl -s -o /dev/null -w "%{http_code}" http://localhost${ADDR#:*}/status || true)
if [ "$code" != "401" ]; then
  echo "❌ Expected 401 without auth, got $code"; fail=1
else
  echo "✅ Unauthorized blocked"
fi

code=$(curl -s -o /dev/null -w "%{http_code}" -H 'Authorization: Bearer wrong' http://localhost${ADDR#:*}/status || true)
if [ "$code" != "401" ]; then
  echo "❌ Expected 401 with wrong auth, got $code"; fail=1
else
  echo "✅ Wrong token blocked"
fi

code=$(curl -s -o /dev/null -w "%{http_code}" -H 'Authorization: Bearer secret123' http://localhost${ADDR#:*}/status || true)
if [ "$code" != "200" ]; then
  echo "❌ Expected 200 with correct token, got $code"; fail=1
else
  echo "✅ Correct token authorized"
fi

kill $PID 2>/dev/null || true
wait $PID 2>/dev/null || true

if [ "$fail" = "1" ]; then
  exit 1
fi
echo "🎉 HTTP auth tests passed"

