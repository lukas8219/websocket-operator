# WebSocket Operator for Kubernetes
Operator to manager distributed websocket in Kubernetes Environment. Inspired by [this article](https://medium.com/lumen-engineering-blog/how-to-implement-a-distributed-and-auto-scalable-websocket-server-architecture-on-kubernetes-4cc32e1dfa45) and by the fact that I solved the same issue. Should be experimental.

## Intro

The `websocket-operator` is a Kubernetes operator designed to manage stateful WebSocket connections within a Kubernetes environment. It automates the deployment and scaling of WebSocket proxies alongside application pods, facilitating efficient handling of long-lived connections and message routing.

## Features

- **SideCar Deployment:** Automatically injects a WebSocket Proxy SideCar into application pods based on annotations or labels.
- **Load Balancing:** Utilizes a LoadBalancer App to distribute WebSocket connections across multiple pods using Rendezvous Hashing.
- **Extensible Proxying:** Initially supports HTTP proxying - optimizations in the Roadmap
- **Controller Automation:** Manages the lifecycle and configuration of WebSocket Sidecars within the Kubernetes cluster.

## Architecture

The operator comprises the following components:

1. **WebSocket Proxy SideCar:** Handles WebSocket connections, manages routing keys, and proxies messages via HTTP requests.
2. **Load Balancer Service:** Distributes incoming WebSocket Connections using same algorithm from SideCar.
3. **Controller:** Automates the injection and management of the WebSocket Proxy SideCar in application pods.

## Getting Started

To deploy the WebSocket Operator in your Kubernetes cluster:

1. **Annotate Deployments:** Add label `ws.operator/enable: true` to your Deployment to enable SideCar injection.
2. Expose `ws-proxy-headless` Headless Service
3. **Deploy the Operator:** Apply the operator manifests to your cluster to activate the controller and related resources.

## WIP: Architecture Diagram
![Diagram](https://github.com/user-attachments/assets/b5bf52e4-6db8-4344-bbe8-4d7e00faafce)


## Future Enhancements

- **Plugin Support:** Develop and integrate plugins for alternative proxying methods beyond HTTP.
- **Metrics Exposure:** Implement metrics to monitor connection success and failure rates.
- **Enhanced Routing:** Improve routing algorithms for more efficient message distribution.

## Contributing

Contributions are welcome! Please fork the repository and submit a pull request with your changes.

## License

This project is licensed under the MIT License. See the LICENSE file for details.
