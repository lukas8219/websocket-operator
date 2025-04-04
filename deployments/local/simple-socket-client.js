// Note: For Node.js environments, you'll need to install ws: npm install ws
// Browser environments can use the native WebSocket API directly
const WebSocket = require('ws');
const yargs = require('yargs');
const { hideBin } = require('yargs/helpers');
const readline = require('readline');

// Create readline interface for reading from stdin
const rl = readline.createInterface({
  input: process.stdin,
  output: process.stdout,
  prompt: '> '
});

// Function to handle sending messages from stdin
function setupStdinMessageHandler(socket, recipientId, user) {
  rl.on('line', (line) => {
    if (line.trim()) {
      const message = JSON.stringify({
        message: line,
        recipientId: String(recipientId),
        from: String(user)
      });
      
      console.log(`Sending: ${message}`);
      socket.send(message);
    }
    rl.prompt();
  });
  
  rl.on('close', () => {
    console.log('Stdin closed. Disconnecting...');
    socket.close();
    process.exit(0);
  });
}


const argv = yargs(hideBin(process.argv)).argv;

const targetPort = argv.port || 3000;
const user = argv.user || "1";
const recipientId = argv.recipientId || "recipient";
const duration = argv.duration || 30000;
const autoPublish = argv.autoPublish

// Server URL to connect to - change this to your WebSocket server address
// Note: WebSockets use ws:// or wss:// protocol instead of http:// or https://
const SERVER_URL = `ws://localhost:${targetPort}`;

console.log("Connecting to WebSocket server...");

// Create a WebSocket connection
const socket = new WebSocket(SERVER_URL, { headers: { "ws-user-id": user } });

socket.on('upgrade', function(request, socket, head) {
  console.log(request.headers);
})

// When successfully connected
socket.on('open', function(){
  console.log("Connected to WebSocket server");
  
  if (autoPublish === "1") {
    const hiInterval = setInterval(() => {
      const message = JSON.stringify({ message: "HI", recipientId: String(recipientId), from: String(user) });
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

setupStdinMessageHandler(socket, recipientId, user);

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