// Note: For Node.js environments, you'll need to install ws: npm install ws
// Browser environments can use the native WebSocket API directly
const WebSocket = require('ws');

// Server URL to connect to - change this to your WebSocket server address
// Note: WebSockets use ws:// or wss:// protocol instead of http:// or https://
const SERVER_URL = "ws://localhost:3000";

console.log("Connecting to WebSocket server...");

// Create a WebSocket connection
const socket = new WebSocket(SERVER_URL);

// When successfully connected
socket.on('open', () => {
  console.log("Connected to WebSocket server");
  
  // Send "HI" message to the server
  socket.send("HI");
  console.log("Sent 'HI' message to server");
  
  // Wait 1 second and then disconnect
  setTimeout(() => {
    console.log("Disconnecting...");
    socket.close();
  }, 1000);
});

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