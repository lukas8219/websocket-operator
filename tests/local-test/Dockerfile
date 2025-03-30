FROM node:18-alpine

# Create app directory
WORKDIR /app

# Copy package files and install dependencies
COPY package*.json ./
RUN npm install

# Bundle app source
COPY simple-socket-server.js ./

# Expose the WebSocket port
EXPOSE 3001

# Command to run the application
CMD ["node", "simple-socket-server.js"] 