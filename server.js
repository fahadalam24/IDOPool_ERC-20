// Simple HTTP server to serve the frontend
const http = require('http');
const fs = require('fs');
const path = require('path');

const PORT = 3001;

const server = http.createServer((req, res) => {
    // Handle request for the frontend
    if (req.url === '/' || req.url === '/index.html') {
        const filePath = path.join(__dirname, 'frontend', 'index.html');
        fs.readFile(filePath, (err, content) => {
            if (err) {
                res.writeHead(500);
                res.end('Error loading the frontend');
                return;
            }
            
            res.writeHead(200, { 'Content-Type': 'text/html' });
            res.end(content);
        });
    } 
    // Handle favicon.ico request to avoid 404 errors
    else if (req.url === '/favicon.ico') {
        res.writeHead(204); // No content response
        res.end();
    } 
    else {
        res.writeHead(404);
        res.end('Page not found');
    }
});

server.listen(PORT, () => {
    console.log(`Server running at http://localhost:${PORT}/`);
});
