#!/usr/bin/env bash
set -euo pipefail

PROG="tokscale-dashboard"
DIR="$(cd "$(dirname "$0")" && pwd)"
BIN="${DIR}/${PROG}"
PIDFILE="${DIR}/${PROG}.pid"
LOGFILE="${DIR}/${PROG}.log"

red()   { printf '\033[31m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
yellow(){ printf '\033[33m%s\033[0m\n' "$*"; }

build() {
    yellow "[build] compiling ${PROG}..."
    (cd "${DIR}" && go build -o "${BIN}" .)
    green "[build] OK"
}

status() {
    if [ -f "${PIDFILE}" ]; then
        local pid
        pid=$(cat "${PIDFILE}")
        if kill -0 "${pid}" 2>/dev/null; then
            green "[status] running (pid=${pid})"
            return 0
        fi
    fi
    red "[status] not running"
    return 1
}

do_start() {
    if status >/dev/null 2>&1; then
        yellow "[start] already running"
        return 0
    fi

    build

    yellow "[start] launching ${PROG}..."
    nohup "${BIN}" > "${LOGFILE}" 2>&1 &
    local pid=$!
    echo "${pid}" > "${PIDFILE}"

    # wait up to 3s for readiness
    local i=0
    while [ $i -lt 30 ]; do
        if curl -sf http://127.0.0.1:8900/ -o /dev/null 2>/dev/null; then
            break
        fi
        sleep 0.1
        i=$((i+1))
    done

    if curl -sf http://127.0.0.1:8900/ -o /dev/null 2>/dev/null; then
        green "[start] OK  pid=${pid}  http://0.0.0.0:8900"
    else
        red "[start] failed to start, check ${LOGFILE}"
        cat "${LOGFILE}"
        return 1
    fi
}

do_stop() {
    if ! status >/dev/null 2>&1; then
        yellow "[stop] not running"
        return 0
    fi

    local pid
    pid=$(cat "${PIDFILE}")
    yellow "[stop] killing pid=${pid}..."
    kill "${pid}" 2>/dev/null || true

    local i=0
    while [ $i -lt 20 ]; do
        if ! kill -0 "${pid}" 2>/dev/null; then
            break
        fi
        sleep 0.1
        i=$((i+1))
    done

    if kill -0 "${pid}" 2>/dev/null; then
        kill -9 "${pid}" 2>/dev/null || true
    fi

    rm -f "${PIDFILE}"
    green "[stop] OK"
}

do_restart() {
    do_stop
    sleep 1
    do_start
}

do_test() {
    yellow "[test] running health checks..."
    local ok=true

    # 1) index page
    if curl -sf http://127.0.0.1:8900/ | grep -q "Tokscale"; then
        green "  [PASS] index page"
    else
        red   "  [FAIL] index page"
        ok=false
    fi

    # 2) summary API
    local summary
    summary=$(curl -sf "http://127.0.0.1:8900/api/summary?range=all" 2>/dev/null)
    if echo "${summary}" | python3 -c "import sys,json; d=json.load(sys.stdin); assert len(d.get('entries',[]))>0, 'no entries'" >/dev/null 2>&1; then
        green "  [PASS] /api/summary  ($(echo "${summary}" | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('entries',[])))" 2>/dev/null) entries)"
    else
        red   "  [FAIL] /api/summary"
        ok=false
    fi

    # 3) monthly API
    local monthly
    monthly=$(curl -sf "http://127.0.0.1:8900/api/monthly" 2>/dev/null)
    if echo "${monthly}" | python3 -c "import sys,json; d=json.load(sys.stdin); assert len(d.get('entries',[]))>0, 'no entries'" >/dev/null 2>&1; then
        green "  [PASS] /api/monthly  ($(echo "${monthly}" | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('entries',[])))" 2>/dev/null) months)"
    else
        red   "  [FAIL] /api/monthly"
        ok=false
    fi

    # 4) graph API
    local graph
    graph=$(curl -sf "http://127.0.0.1:8900/api/graph?range=all" 2>/dev/null)
    if echo "${graph}" | python3 -c "import sys,json; d=json.load(sys.stdin); assert 'summary' in d and 'contributions' in d" >/dev/null 2>&1; then
        green "  [PASS] /api/graph     (cost=$(echo "${graph}" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['summary'].get('totalCost'))" 2>/dev/null), days=$(echo "${graph}" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['summary'].get('activeDays'))" 2>/dev/null))"
    else
        red   "  [FAIL] /api/graph"
        ok=false
    fi

    # 5) client filter
    local filtered
    filtered=$(curl -sf "http://127.0.0.1:8900/api/summary?range=all&client=claude" 2>/dev/null)
    if echo "${filtered}" | python3 -c "
import sys,json
d=json.load(sys.stdin)
for e in d.get('entries',[]):
    assert e['client']=='claude', f'wrong client: {e[\"client\"]}'
print('ok')
" >/dev/null 2>&1; then
        green "  [PASS] client filter  (claude only)"
    else
        red   "  [FAIL] client filter"
        ok=false
    fi

    # 6) range filter
    local today
    today=$(curl -sf "http://127.0.0.1:8900/api/summary?range=today" 2>/dev/null)
    if echo "${today}" | python3 -c "import sys,json; d=json.load(sys.stdin); assert 'entries' in d" >/dev/null 2>&1; then
        green "  [PASS] range filter   (today)"
    else
        red   "  [FAIL] range filter"
        ok=false
    fi

    if ${ok}; then
        green "[test] ALL PASSED"
        return 0
    else
        red "[test] SOME FAILED"
        return 1
    fi
}

logs() {
    if [ -f "${LOGFILE}" ]; then
        tail -50 "${LOGFILE}"
    else
        yellow "[logs] no log file"
    fi
}

case "${1:-}" in
    start)   do_start   ;;
    stop)    do_stop    ;;
    restart) do_restart ;;
    status)  status     ;;
    test)    do_test    ;;
    build)   build     ;;
    logs)    logs      ;;
    *)
        echo "Usage: $0 {start|stop|restart|status|test|build|logs}"
        echo ""
        echo "  start    Build & start the dashboard (background)"
        echo "  stop     Stop the dashboard"
        echo "  restart  Stop + start"
        echo "  status   Check if running"
        echo "  test     Run health checks against all endpoints"
        echo "  build    Compile the binary"
        echo "  logs     Show recent log output"
        ;;
esac
