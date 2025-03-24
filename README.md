# socketio-operator
Operator to manager distributed websocket in Kubernetes Environment. Inspired by [this article](https://medium.com/lumen-engineering-blog/how-to-implement-a-distributed-and-auto-scalable-websocket-server-architecture-on-kubernetes-4cc32e1dfa45) 
and by the fact that I solved the same issue. Should be experimental.

- WS Proxy
- Controller for "Signaling Server"
- Exposing metrics of failed/successful rates
- Able to inject via pod annotation/label

WIP Diagram https://excalidraw.com/#json=G8pdBZbTTap_aNkDMKiqz,d2JNhKt-gacOyOgCPK3uqQ
