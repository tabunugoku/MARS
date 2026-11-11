#!/usr/bin/env bash
# Runs a built image against a synthetic library: first-run setup, a library
# scan, then reports image size and container memory to stdout (and to the
# job summary on GitHub Actions).
#
# usage: smoke-image.sh IMAGE
set -euo pipefail

image=$1
port=25600
base=http://localhost:$port
api=$base/api/v1
work=$(mktemp -d)
jar=$work/cookies
name=cominavi-smoke

cleanup() {
  docker logs "$name" >"$work/server.log" 2>&1 || true
  docker rm -f "$name" >/dev/null 2>&1 || true
}
trap cleanup EXIT

python3 "$(dirname "$0")/make-fixtures.py" "$work/comics"
# The container runs as uid 65532; the mount is read-only either way.
chmod -R a+rX "$work"

docker run -d --name "$name" -p $port:25600 -v "$work/comics:/comics:ro" "$image" >/dev/null

for _ in $(seq 60); do
  curl -fsS "$api/healthz" >/dev/null 2>&1 && break
  sleep 1
done
curl -fsS "$api/healthz"; echo
# The image's own health probe: the binary checks itself (no shell, no curl).
docker exec "$name" /usr/local/bin/cominavi healthcheck
echo "healthcheck subcommand: ok"

token=$(docker logs "$name" 2>&1 | grep -o 'token=[A-Za-z0-9_-]*' | head -1 | cut -d= -f2 || true)
if [ -z "$token" ]; then
  docker cp "$name:/data/setup-token" "$work/setup-token"
  token=$(tr -d '[:space:]' <"$work/setup-token")
fi

post() {
  curl -fsS -b "$jar" -c "$jar" -H "Origin: $base" -H 'Content-Type: application/json' -X POST "$api$1" -d "$2"
}

post /setup "{\"token\":\"$token\",\"name\":\"admin\",\"email\":\"admin@example.com\",\"password\":\"smoke-test-password-1\"}" >/dev/null
post /libraries '{"name":"Smoke","root":"/comics","kind":"manga"}' >/dev/null

start=$(date +%s)
for _ in $(seq 300); do
  sleep 2
  active=$(curl -fsS -b "$jar" "$api/tasks" | python3 -c 'import json,sys; print(sum(t["state"] in ("running","queued") for t in json.load(sys.stdin)))')
  [ "$active" = 0 ] && break
done
elapsed=$(( $(date +%s) - start ))

libs=$(curl -fsS -b "$jar" "$api/libraries")
books=$(python3 -c 'import json,sys; print(sum(l["bookCount"] for l in json.loads(sys.argv[1])))' "$libs")
series=$(python3 -c 'import json,sys; print(sum(l["seriesCount"] for l in json.loads(sys.argv[1])))' "$libs")
failed=$(curl -fsS -b "$jar" "$api/tasks" | python3 -c 'import json,sys; print(sum(t["state"]=="failed" for t in json.load(sys.stdin)))')

# A few page requests so the reader path is in the measurement too.
curl -fsS -b "$jar" "$api/series" >/dev/null

usage=$(docker stats --no-stream --format '{{.MemUsage}}' "$name")
id=$(docker inspect -f '{{.Id}}' "$name")
peak=""
for f in "/sys/fs/cgroup/system.slice/docker-$id.scope/memory.peak" "/sys/fs/cgroup/docker/$id/memory.peak"; do
  [ -r "$f" ] && peak=$(( $(cat "$f") / 1024 / 1024 ))MiB && break
done
size=$(docker image inspect -f '{{.Size}}' "$image")

# The image's HEALTHCHECK instruction itself (what TrueNAS shows): the first
# probe runs one interval (30 s) after start.
health=""
for _ in $(seq 30); do
  health=$(docker inspect -f '{{.State.Health.Status}}' "$name")
  [ "$health" = healthy ] && break
  sleep 3
done
if [ "$health" != healthy ]; then
  echo "container health is '$health', want healthy" >&2
  docker inspect -f '{{json .State.Health}}' "$name" >&2
  exit 1
fi

report=$(cat <<EOF
| measure | value |
| --- | --- |
| image size (linux/$(docker image inspect -f '{{.Architecture}}' "$image")) | $(( size / 1024 / 1024 )) MiB ($size bytes) |
| library | $series series, $books books |
| scan + thumbnails | ${elapsed}s, $failed failed tasks |
| memory after scan (docker stats) | $usage |
| peak memory (cgroup memory.peak) | ${peak:-n/a} |
EOF
)
echo "$report"
[ -n "${GITHUB_STEP_SUMMARY:-}" ] && echo "$report" >>"$GITHUB_STEP_SUMMARY"

if [ "$books" -lt 100 ]; then
  echo "expected at least 100 books, got $books" >&2
  docker logs "$name" >&2
  exit 1
fi
