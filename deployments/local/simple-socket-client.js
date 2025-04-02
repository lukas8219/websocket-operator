// Note: For Node.js environments, you'll need to install ws: npm install ws
// Browser environments can use the native WebSocket API directly
const WebSocket = require('ws');

const targetPort = 3000;

const [,,user, recipientId, duration=30000, autoPublish="1"] = process.argv;

// Server URL to connect to - change this to your WebSocket server address
// Note: WebSockets use ws:// or wss:// protocol instead of http:// or https://
const SERVER_URL = `ws://localhost:${targetPort}`;

console.log("Connecting to WebSocket server...");

// Create a WebSocket connection
const socket = new WebSocket(SERVER_URL, { headers: { "ws-user-id": user } });

// When successfully connected
socket.on('open', () => {
  console.log("Connected to WebSocket server");
  
  if (autoPublish === "1") {
    const hiInterval = setInterval(() => {
      const message = JSON.stringify({ message: "HI", recipientId: recipientId, from: user });
      console.log(`Sent ${message} to server`);
      socket.send(message);  
    }, 3000);
  
    setTimeout(() => {
      clearInterval(hiInterval);
      console.log("Disconnecting...");
      socket.close();
    }, Number(duration))
  }
})

// Handle errors
socket.on('error', (error) => {
  console.error("WebSocket error:", error);
  socket.close();
});

// When disconnected
socket.on('close', (code, reason) => {
  console.log(`Disconnected: Code ${code}${reason ? ', ' + reason : ''}`);
  process.exit(0);
});

// Optional: Handle incoming messages
socket.on('message', (data) => {
  console.log("Received message:", data.toString());
}); 