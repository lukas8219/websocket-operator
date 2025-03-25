// Import the WebSocket server library
const WebSocket = require('ws');

// Port to listen on
const PORT = 3001;

// Create a WebSocket server
const server = new WebSocket.Server({ port: PORT });

console.log(`WebSocket server started and listening on port ${PORT}`);

// Handle new connections
server.on('connection', (socket, req) => {
  const clientAddress = req.socket.remoteAddress;
  console.log(`New client connected from ${clientAddress}`);
  
  // Send a welcome message to the client
  socket.send('Welcome to the WebSocket server!');
  
  // Handle messages from clients
  socket.on('message', (message) => {
    const messageStr = message.toString();
    console.log(`Received message: ${messageStr}`);
    
    // Echo the message back with a prefix
    console.log(`Sending message back to client ${messageStr}`);
    socket.send(`Server received: ${messageStr}`);
  });
  
  // Handle client disconnection
  socket.on('close', (code, reason) => {
    console.log(`Client disconnected. Code: ${code}${reason ? ', Reason: ' + reason : ''}`);
  });
  
  // Handle errors
  socket.on('error', (error) => {
    console.error('Socket error:', error);
  });
});

// Handle server errors
server.on('error', (error) => {
  console.error('Server error:', error);
});

// Clean up on process termination
process.on('SIGINT', () => {
  console.log('Shutting down server...');
  server.close(() => {
    console.log('Server closed');
    process.exit(0);
  });
}); 