#!/usr/bin/env bash
set -euo pipefail

BIN=./mcp-service
CFG=config.json

echo "[codex] Build binary"
go build -o "$BIN" .

run_rpc() {
  local payload="$1"
  (
    printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}'
    sleep 0.2
    printf '%s\n' "$payload"
    # keep stdin open briefly to avoid EOF before server reads
    sleep 0.5
  ) | timeout 20s "$BIN" -config "$CFG"
}

decode_resource_link() {
  local uri="$1"
  echo "$uri" | sed 's/^data:application\/json;base64,//' | base64 -d
}

assert_eq() { [ "$1" = "$2" ] || { echo "Assertion failed: $1 != $2"; exit 1; }; }

echo "[codex] Test: tools/status_get"
out=$(run_rpc '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"status_get","arguments":{"fast_only":true}}}')
json=$(echo "$out" | grep -E '^\{')
ctype=$(echo "$json" | jq -r 'select(.id==2).result.content[1].type')
assert_eq "$ctype" "resource_link"
status_uri=$(echo "$json" | jq -r 'select(.id==2).result.content[1].uri')
status_json=$(decode_resource_link "$status_uri")
echo "$status_json" | jq '.' >/dev/null
qhealth=$(echo "$status_json" | jq -r '.qdrant.health')
echo "[codex] Qdrant health: $qhealth"

if [ "$qhealth" = "ok" ]; then
  echo "[codex] Test: tools/rag_index"
  out=$(run_rpc '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"rag_index","arguments":{"dir":"./docs","include_code":false}}}')
  json=$(echo "$out" | grep -E '^\{')
  ctype=$(echo "$json" | jq -r 'select(.id==3).result.content[1].type')
  assert_eq "$ctype" "resource_link"
  idx_uri=$(echo "$json" | jq -r 'select(.id==3).result.content[1].uri')
  idx_json=$(decode_resource_link "$idx_uri")
  echo "$idx_json" | jq '.' >/dev/null
  echo "[codex] rag_index: $(echo "$idx_json" | jq -r '.indexed') chunks"

  echo "[codex] Test: tools/rag_search"
  out=$(run_rpc '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"rag_search","arguments":{"query":"vector databases","k":3}}}')
  json=$(echo "$out" | grep -E '^\{')
  ctype=$(echo "$json" | jq -r 'select(.id==4).result.content[1].type')
  assert_eq "$ctype" "resource_link"
  s_uri=$(echo "$json" | jq -r 'select(.id==4).result.content[1].uri')
  s_json=$(decode_resource_link "$s_uri")
  echo "$s_json" | jq '.' >/dev/null
  echo "[codex] rag_search total_chunks: $(echo "$s_json" | jq -r '.total_chunks')"

  echo "[codex] Test: tools/rag_projects"
  out=$(run_rpc '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"rag_projects","arguments":{"prefix":"","offset":0,"limit":10}}}')
  json=$(echo "$out" | grep -E '^\{')
  ctype=$(echo "$json" | jq -r 'select(.id==5).result.content[1].type')
  assert_eq "$ctype" "resource_link"
  p_uri=$(echo "$json" | jq -r 'select(.id==5).result.content[1].uri')
  p_json=$(decode_resource_link "$p_uri")
  echo "$p_json" | jq '.' >/dev/null
  echo "[codex] rag_projects count: $(echo "$p_json" | jq -r '.count')"

  # Optional delete test: delete by first project name if exists
  proj=$(echo "$p_json" | jq -r '.projects[0].project // empty')
  if [ -n "$proj" ]; then
    echo "[codex] Test: tools/rag_delete (project=$proj)"
    out=$(run_rpc '{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"rag_delete","arguments":{"project":"'$proj'"}}}')
    json=$(echo "$out" | grep -E '^\{')
    ctype=$(echo "$json" | jq -r 'select(.id==6).result.content[1].type')
    assert_eq "$ctype" "resource_link"
    d_uri=$(echo "$json" | jq -r 'select(.id==6).result.content[1].uri')
    d_json=$(decode_resource_link "$d_uri")
    echo "$d_json" | jq '.' >/dev/null
    echo "[codex] rag_delete deleted: $(echo "$d_json" | jq -r '.deleted')"
  fi
else
  echo "[codex] Qdrant not healthy; skipping rag_index/search/projects"
fi

echo "[codex] All checks completed"
