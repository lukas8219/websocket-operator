#!/bin/sh
# Runs entirely inside the pinned stock image, with no external network.
set -eu
wstest -m echoserver -w ws://127.0.0.1:9001 -u 0 > /reports/echo.log 2>&1 &
/fixture/proxy > /reports/proxy.log 2>&1 &
python - <<'PY'
import json, socket, time
from autobahntestsuite.case import Cases, CaseCategories, CaseSubCategories, CaseSetname, CaseBasename
from autobahntestsuite.caseset import CaseSet
cases = CaseSet(CaseSetname, CaseBasename, Cases, CaseCategories, CaseSubCategories)
expected = cases.parseSpecCases(json.load(open('/fixture/fuzzingclient.json')))
json.dump(expected, open('/reports/expected.json', 'w'))
for port in [9000, 9001]:
    for attempt in range(100):
        try:
            sock = socket.create_connection(('127.0.0.1', port), .1)
            sock.close()
            break
        except socket.error:
            time.sleep(.1)
    else:
        raise RuntimeError('server did not start')
PY
wstest -m fuzzingclient -s /fixture/fuzzingclient.json > /reports/client.log 2>&1
