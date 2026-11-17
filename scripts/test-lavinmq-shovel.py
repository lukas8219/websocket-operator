#!/usr/bin/env python3
"""Check HTTP shovel wire compatibility against an isolated LavinMQ broker."""
import base64
import http.server
import json
import os
import queue
import threading
import time
import urllib.error
import urllib.request
import uuid

management = os.environ.get("LAVINMQ_HTTP_URL", "http://127.0.0.1:15672")
amqp = os.environ.get("LAVINMQ_AMQP_URL", "amqp://guest:guest@127.0.0.1:5672/")
credentials = os.environ.get("LAVINMQ_USER", "guest") + ":" + os.environ.get("LAVINMQ_PASSWORD", "guest")
authorization = "Basic " + base64.b64encode(credentials.encode()).decode()
name = "ws-operator-probe-" + uuid.uuid4().hex
received = queue.Queue()


def api(method, path, data=None):
    body = None if data is None else json.dumps(data).encode()
    request = urllib.request.Request(management + path, body, method=method,
                                     headers={"Authorization": authorization, "Content-Type": "application/json"})
    with urllib.request.urlopen(request, timeout=5) as response:
        content = response.read()
        return json.loads(content) if content else None


class Receiver(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        body = self.rfile.read(int(self.headers["Content-Length"]))
        received.put((self.path, {k.lower(): v for k, v in self.headers.items()}, body))
        self.send_response(204)
        self.end_headers()

    def log_message(self, *args):
        pass


server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Receiver)
thread = threading.Thread(target=server.serve_forever, daemon=True)
thread.start()
queue_path = "/api/queues/%2F/" + name
shovel_path = "/api/parameters/shovel/%2F/" + name
try:
    api("PUT", queue_path, {"durable": False, "auto_delete": False, "arguments": {}})
    api("PUT", shovel_path, {"value": {
        "src-uri": amqp, "src-queue": name, "src-prefetch-count": 1,
        "dest-uri": f"http://127.0.0.1:{server.server_port}/message",
        "dest-timeout": 5, "ack-mode": "on-confirm"}})
    deadline = time.monotonic() + 15
    while not any(x.get("name") == name and x.get("state", "").lower() == "running" for x in api("GET", "/api/shovels")):
        if time.monotonic() >= deadline:
            raise RuntimeError("shovel did not become ready")
        time.sleep(0.1)
    packet = b"\x02\x00binary\xffpayload"
    for source_header, expected_header in [("ws-user-id", "x-ws-user-id"), ("App-User", "x-app-user")]:
        result = api("POST", "/api/exchanges/%2F/amq.default/publish", {
            "properties": {"headers": {source_header: "recipient"}}, "routing_key": name,
            "payload": base64.b64encode(packet).decode(), "payload_encoding": "base64"})
        assert result["routed"], result
        path, headers, body = received.get(timeout=10)
        assert path == "/message" and body == packet, (path, body)
        assert headers[expected_header] == "recipient", headers
        assert "ws-user-id" not in headers, headers
    print(json.dumps({"payload_preserved": True, "default_header_compatible": False,
                      "configured_X_App_User_compatible": True}))
finally:
    for path in [shovel_path, queue_path]:
        try:
            api("DELETE", path)
        except urllib.error.HTTPError as error:
            if error.code != 404:
                raise
    server.shutdown()
    server.server_close()
    thread.join(timeout=2)
