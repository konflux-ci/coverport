#!/usr/bin/env node
/**
 * Coverage HTTP Server Wrapper for Node.js
 * 
 * A pure wrapper that runs any Node.js script with coverage collection and
 * exposes coverage data via HTTP. Completely application-agnostic.
 * 
 * Usage:
 *     node coverage_server.js app.js
 *     node coverage_server.js path/to/script.js
 * 
 * Environment Variables:
 *     COVERAGE_PORT - Port for coverage HTTP server (default: 9095)
 */

import { createServer } from 'http';
import { fileURLToPath } from 'url';
import { dirname, resolve } from 'path';
import { existsSync, readFileSync } from 'fs';
import inspector from 'inspector';
import v8ToIstanbul from 'v8-to-istanbul';

// Configuration
const COVERAGE_PORT = parseInt(process.env.COVERAGE_PORT || '9095', 10);
const PRINT_PREFIX = '[coverage-wrapper]';

// Global coverage tracking (purely in-memory - no filesystem needed!)
let coverageSession = null;

/**
 * HTTP handler for coverage endpoints
 */
function handleRequest(req, res) {
  const url = new URL(req.url, `http://localhost:${COVERAGE_PORT}`);
  const path = url.pathname;
  const label = url.searchParams.get('name') || 'session';

  if (path === '/coverage') {
    console.log(`${PRINT_PREFIX} Coverage dump requested (label=${label})`);
    handleCoverageDump(req, res, label);
  } else if (path === '/health') {
    console.log(`${PRINT_PREFIX} Health check requested`);
    handleHealth(req, res);
  } else if (path === '/coverage/reset') {
    console.log(`${PRINT_PREFIX} Coverage reset requested`);
    handleCoverageReset(req, res);
  } else {
    res.writeHead(404, { 'Content-Type': 'text/plain' });
    res.end('Not found');
  }
}

/**
 * Handle /coverage endpoint - dump coverage data using Inspector API
 * Purely in-memory - no filesystem writes needed!
 */
async function handleCoverageDump(req, res, label) {
  try {
    if (!coverageSession) {
      res.writeHead(500, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: 'Coverage session not initialized' }));
      return;
    }

    // Use Inspector API to get precise coverage in real-time (in-memory)
    coverageSession.post('Profiler.takePreciseCoverage', async (err, { result }) => {
      if (err) {
        console.error(`${PRINT_PREFIX} Error taking coverage:`, err);
        res.writeHead(500, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ error: err.message }));
        return;
      }

      try {
        // Convert V8 coverage to Istanbul format (purely in-memory)
        const istanbulCoverage = await convertV8ToIstanbul(result);
        
        const payload = {
          label,
          timestamp: new Date().toISOString(),
          coverage_data: Buffer.from(JSON.stringify(istanbulCoverage)).toString('base64'),
        };

        const body = JSON.stringify(payload);
        res.writeHead(200, {
          'Content-Type': 'application/json',
          'Content-Length': Buffer.byteLength(body),
        });
        res.end(body);
      } catch (conversionError) {
        console.error(`${PRINT_PREFIX} Error converting coverage:`, conversionError);
        // Fall back to empty coverage
        const payload = {
          label,
          timestamp: new Date().toISOString(),
          coverage_data: Buffer.from(JSON.stringify({})).toString('base64'),
        };
        const body = JSON.stringify(payload);
        res.writeHead(200, {
          'Content-Type': 'application/json',
          'Content-Length': Buffer.byteLength(body),
        });
        res.end(body);
      }
    });
  } catch (error) {
    console.error(`${PRINT_PREFIX} Error dumping coverage:`, error);
    res.writeHead(500, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ error: error.message }));
  }
}

/**
 * Convert V8 coverage to Istanbul format (purely in-memory)
 * @param {Array} v8Coverage - V8 coverage result from Inspector API
 * @returns {Promise<Object>} Istanbul coverage object
 */
async function convertV8ToIstanbul(v8Coverage) {
  const istanbulCoverage = {};
  
  for (const script of v8Coverage) {
    // Skip scripts without URL or with unsupported URLs
    if (!script.url || script.url.startsWith('node:') || script.url.includes('node_modules')) {
      continue;
    }
    
    // Skip server, client, and test files
    if (script.url.includes('/server/') || 
        script.url.includes('/client/') || 
        script.url.includes('/test/')) {
      continue;
    }
    
    // Convert file:// URL to path
    let filePath;
    try {
      filePath = fileURLToPath(script.url);
    } catch (e) {
      continue; // Skip if URL is not a file:// URL
    }
    
    // Skip if file doesn't exist
    if (!existsSync(filePath)) {
      continue;
    }
    
    try {
      // Create v8-to-istanbul converter
      const converter = v8ToIstanbul(filePath);
      await converter.load(); // Load source file
      
      // Apply V8 coverage data
      converter.applyCoverage([{
        functionName: '',
        ranges: script.functions.flatMap(fn => fn.ranges || [])
      }]);
      
      // Convert to Istanbul format
      const istanbul = converter.toIstanbul();
      Object.assign(istanbulCoverage, istanbul);
    } catch (e) {
      console.error(`${PRINT_PREFIX} Error converting ${filePath}:`, e.message);
      // Continue with other files
    }
  }
  
  return istanbulCoverage;
}

/**
 * Handle /health endpoint
 */
function handleHealth(req, res) {
  const payload = {
    status: 'ok',
    coverage_enabled: true,
  };

  const body = JSON.stringify(payload);
  res.writeHead(200, {
    'Content-Type': 'application/json',
    'Content-Length': Buffer.byteLength(body),
  });
  res.end(body);
}

/**
 * Handle /coverage/reset endpoint
 * Purely in-memory - no filesystem operations needed!
 */
function handleCoverageReset(req, res) {
  try {
    // Reset coverage by stopping and restarting the profiler
    if (coverageSession) {
      coverageSession.post('Profiler.stopPreciseCoverage', () => {
        coverageSession.post('Profiler.startPreciseCoverage', { callCount: true, detailed: true }, () => {
          console.log(`${PRINT_PREFIX} Coverage counters reset`);
        });
      });
    }

    res.writeHead(200, { 'Content-Type': 'text/plain' });
    res.end('Coverage reset');
  } catch (error) {
    console.error(`${PRINT_PREFIX} Error resetting coverage:`, error);
    res.writeHead(500, { 'Content-Type': 'text/plain' });
    res.end('Error resetting coverage');
  }
}

/**
 * Start the coverage HTTP server
 */
function startServer() {
  const server = createServer(handleRequest);
  server.listen(COVERAGE_PORT, '0.0.0.0', () => {
    console.log(`${PRINT_PREFIX} HTTP server listening on port ${COVERAGE_PORT}`);
  });
  return server;
}

/**
 * Run the target application with V8 coverage using Inspector API
 * This allows us to collect coverage in real-time from a long-running server
 * Purely in-memory - no filesystem operations!
 */
async function runAppWithCoverage(scriptPath) {
  console.log(`${PRINT_PREFIX} Running script with coverage: ${scriptPath}`);

  // Start coverage using Inspector API (purely in-memory)
  coverageSession = new inspector.Session();
  coverageSession.connect();
  
  // Enable Profiler and start precise coverage
  coverageSession.post('Profiler.enable', () => {
    coverageSession.post('Profiler.startPreciseCoverage', { callCount: true, detailed: true }, () => {
      console.log(`${PRINT_PREFIX} V8 coverage started via Inspector API (in-memory)`);
      
      // Now import and run the app
      import(scriptPath).catch((error) => {
        console.error(`${PRINT_PREFIX} Error running application:`, error);
        process.exit(1);
      });
    });
  });
}

/**
 * Main entry point
 */
function main() {
  if (process.argv.length < 3) {
    console.error(`Usage: node ${process.argv[1]} <script.js> [args...]`);
    process.exit(1);
  }

  const scriptPath = resolve(process.argv[2]);
  
  if (!existsSync(scriptPath)) {
    console.error(`${PRINT_PREFIX} Script not found: ${scriptPath}`);
    process.exit(1);
  }

  console.log(`${PRINT_PREFIX} Coverage collection started (purely in-memory)`);

  // Start HTTP server for coverage endpoints
  const server = startServer();

  // Run the application with coverage
  runAppWithCoverage(scriptPath);

  // Handle shutdown
  process.on('SIGINT', () => {
    console.log(`\n${PRINT_PREFIX} Shutting down...`);
    if (coverageSession) {
      coverageSession.disconnect();
    }
    server.close();
    process.exit(0);
  });

  process.on('SIGTERM', () => {
    console.log(`\n${PRINT_PREFIX} Shutting down...`);
    if (coverageSession) {
      coverageSession.disconnect();
    }
    server.close();
    process.exit(0);
  });
}

// Run main if executed directly
if (import.meta.url === `file://${process.argv[1]}`) {
  main();
}

