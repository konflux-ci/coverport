/**
 * Node.js Coverage HTTP Client
 * Collect coverage data from Node.js applications running in Kubernetes pods.
 */

import { spawn } from 'child_process';
import { promisify } from 'util';
import { writeFile, mkdir, readFile, rm, readdir } from 'fs/promises';
import { existsSync } from 'fs';
import { resolve, join } from 'path';
import axios from 'axios';

const sleep = promisify(setTimeout);

export class CoverageClient {
  /**
   * Initialize coverage client
   * @param {string} namespace - Kubernetes namespace
   * @param {string} outputDir - Directory to store coverage data
   */
  constructor(namespace = 'default', outputDir = './coverage-output') {
    this.namespace = namespace;
    this.outputDir = resolve(outputDir);
  }

  /**
   * Get pod name dynamically using Kubernetes label selector
   * @param {string} namespace - Kubernetes namespace
   * @param {string} labelSelector - Label selector to find the pod
   * @returns {Promise<string>} Name of the first running pod
   */
  static async getPodName(namespace, labelSelector = 'app=coverage-demo') {
    try {
      const k8s = await import('@kubernetes/client-node');
      const kc = new k8s.KubeConfig();
      
      // Check if running in a cluster by checking if the service account files exist
      const { existsSync } = await import('fs');
      const inCluster = existsSync('/var/run/secrets/kubernetes.io/serviceaccount/token');
      
      if (inCluster) {
        kc.loadFromCluster();
      } else {
        kc.loadFromDefault();
      }

      const k8sApi = kc.makeApiClient(k8s.CoreV1Api);
      const res = await k8sApi.listNamespacedPod(namespace, undefined, undefined, undefined, undefined, labelSelector);

      for (const pod of res.body.items) {
        if (pod.status.phase === 'Running') {
          return pod.metadata.name;
        }
      }

      throw new Error(`No running pod found with label '${labelSelector}' in namespace '${namespace}'`);
    } catch (error) {
      if (error.code === 'MODULE_NOT_FOUND') {
        throw new Error(
          'kubernetes-client package required for pod discovery. ' +
          'Install with: npm install @kubernetes/client-node'
        );
      }
      throw error;
    }
  }

  /**
   * Reset coverage counters in the pod
   * @param {string} podName - Name of the pod
   * @param {number} coveragePort - Port where coverage server is running
   * @param {number} timeout - Timeout in seconds
   * @param {boolean} useKubectl - If true, use kubectl binary
   * @returns {Promise<boolean>} True if reset was successful
   */
  async resetCoverage(podName, coveragePort = 9095, timeout = 30, useKubectl = true) {
    console.log(`[coverage-client] Resetting coverage counters in pod ${podName}`);

    if (useKubectl) {
      return this._resetWithKubectl(podName, coveragePort, timeout);
    } else {
      return this._resetWithNativePortForward(podName, coveragePort, timeout);
    }
  }

  /**
   * Collect coverage data from a Kubernetes pod via port-forwarding
   * @param {string} podName - Name of the pod
   * @param {string} testName - Name of the test (used for labeling)
   * @param {number} coveragePort - Port where coverage server is running
   * @param {number} timeout - Timeout in seconds
   * @param {boolean} useKubectl - If true, use kubectl binary
   * @returns {Promise<string|null>} Path to the saved coverage file, or null if failed
   */
  async collectCoverageFromPod(podName, testName, coveragePort = 9095, timeout = 30, useKubectl = true) {
    console.log(`[coverage-client] Collecting coverage from pod ${podName} (test: ${testName})`);

    if (useKubectl) {
      return this._collectWithKubectl(podName, testName, coveragePort, timeout);
    } else {
      return this._collectWithNativePortForward(podName, testName, coveragePort, timeout);
    }
  }

  /**
   * Collect coverage using kubectl binary
   * @private
   */
  async _collectWithKubectl(podName, testName, coveragePort, timeout) {
    const localPort = await this._findFreePort();
    console.log(`[coverage-client] Using kubectl port-forward`);

    return new Promise((resolve, reject) => {
      const pfProcess = spawn('kubectl', [
        'port-forward',
        '-n', this.namespace,
        podName,
        `${localPort}:${coveragePort}`,
      ], {
        stdio: ['ignore', 'pipe', 'pipe'],
      });

      let settled = false;

      const cleanup = () => {
        if (!pfProcess.killed) {
          pfProcess.kill('SIGTERM');
        }
      };

      // Wait for port-forward to be ready
      sleep(2000)
        .then(async () => {
          // Check if port-forward is working
          const healthUrl = `http://localhost:${localPort}/health`;
          for (let i = 0; i < 5; i++) {
            try {
              const response = await axios.get(healthUrl, { timeout: 2000 });
              if (response.status === 200) {
                console.log(`[coverage-client] Port-forward ready on port ${localPort}`);
                break;
              }
            } catch (error) {
              await sleep(1000);
            }
          }

          // Fetch coverage data
          const result = await this._fetchCoverageData(localPort, testName, timeout);
          settled = true;
          cleanup();
          resolve(result);
        })
        .catch((error) => {
          if (!settled) {
            settled = true;
            cleanup();
            reject(error);
          }
        });

      pfProcess.on('error', (error) => {
        if (!settled) {
          settled = true;
          cleanup();
          reject(error);
        }
      });
    });
  }

  /**
   * Collect coverage using native Python Kubernetes client
   * @private
   */
  async _collectWithNativePortForward(podName, testName, coveragePort, timeout) {
    console.log(`[coverage-client] Using native port-forward`);
    try {
      const k8s = await import('@kubernetes/client-node');
      const kc = new k8s.KubeConfig();

      try {
        kc.loadFromCluster();
        console.log('[coverage-client] Using in-cluster config');
      } catch {
        kc.loadFromDefault();
        console.log('[coverage-client] Using kubeconfig');
      }

      const forward = new k8s.PortForward(kc);
      const server = await forward.portForward(this.namespace, podName, [coveragePort]);

      try {
        const localPort = server.address().port;
        console.log(`[coverage-client] Port-forward established on port ${localPort}`);
        
        // Give it a moment to stabilize
        await sleep(1000);
        
        const result = await this._fetchCoverageData(localPort, testName, timeout);
        return result;
      } finally {
        server.close();
      }
    } catch (error) {
      if (error.code === 'MODULE_NOT_FOUND') {
        console.error('[coverage-client] @kubernetes/client-node package not installed');
        console.error('  Install with: npm install @kubernetes/client-node');
        console.error('  Or use useKubectl=true to use kubectl binary');
      } else {
        console.error(`[coverage-client] Error with native port-forward:`, error);
      }
      return null;
    }
  }

  /**
   * Fetch coverage data from the port-forwarded endpoint
   * @private
   */
  async _fetchCoverageData(localPort, testName, timeout) {
    try {
      // Ensure output directory exists
      await mkdir(this.outputDir, { recursive: true });

      // Collect coverage data
      const coverageUrl = `http://localhost:${localPort}/coverage?name=${testName}`;
      const response = await axios.get(coverageUrl, { timeout: timeout * 1000 });

      if (response.status !== 200) {
        console.error(`[coverage-client] Failed to collect coverage: HTTP ${response.status}`);
        return null;
      }

      const data = response.data;
      const coverageB64 = data.coverage_data;

      if (!coverageB64) {
        console.error('[coverage-client] No coverage data in response');
        return null;
      }

      // Decode coverage data (it's a base64-encoded JSON string of NYC coverage data)
      const coverageJson = Buffer.from(coverageB64, 'base64').toString('utf8');
      const coverageData = JSON.parse(coverageJson);

      // Save as JSON file (NYC format)
      const coverageFile = resolve(this.outputDir, `coverage_${testName}.json`);
      await writeFile(coverageFile, JSON.stringify(coverageData, null, 2));

      console.log(`[coverage-client] Coverage data saved to ${coverageFile}`);
      return coverageFile;
    } catch (error) {
      console.error(`[coverage-client] Error fetching coverage data:`, error.message);
      return null;
    }
  }

  /**
   * Reset coverage using kubectl binary
   * @private
   */
  async _resetWithKubectl(podName, coveragePort, timeout) {
    const localPort = await this._findFreePort();

    return new Promise((resolve) => {
      const pfProcess = spawn('kubectl', [
        'port-forward',
        '-n', this.namespace,
        podName,
        `${localPort}:${coveragePort}`,
      ], {
        stdio: ['ignore', 'pipe', 'pipe'],
      });

      let settled = false;

      const cleanup = () => {
        if (!pfProcess.killed) {
          pfProcess.kill('SIGTERM');
        }
      };

      sleep(2000)
        .then(async () => {
          const resetUrl = `http://localhost:${localPort}/coverage/reset`;
          const response = await axios.get(resetUrl, { timeout: timeout * 1000 });

          if (response.status === 200) {
            console.log('[coverage-client] ✓ Coverage counters reset');
            settled = true;
            cleanup();
            resolve(true);
          } else {
            console.error(`[coverage-client] Failed to reset coverage: HTTP ${response.status}`);
            settled = true;
            cleanup();
            resolve(false);
          }
        })
        .catch((error) => {
          if (!settled) {
            console.error(`[coverage-client] Error resetting coverage:`, error.message);
            settled = true;
            cleanup();
            resolve(false);
          }
        });

      pfProcess.on('error', (error) => {
        if (!settled) {
          console.error(`[coverage-client] Port-forward error:`, error.message);
          settled = true;
          cleanup();
          resolve(false);
        }
      });
    });
  }

  /**
   * Reset coverage using native Kubernetes client
   * @private
   */
  async _resetWithNativePortForward(podName, coveragePort, timeout) {
    try {
      const k8s = await import('@kubernetes/client-node');
      const kc = new k8s.KubeConfig();

      try {
        kc.loadFromCluster();
      } catch {
        kc.loadFromDefault();
      }

      const forward = new k8s.PortForward(kc);
      const server = await forward.portForward(this.namespace, podName, [coveragePort]);

      try {
        const localPort = server.address().port;
        await sleep(1000);

        const resetUrl = `http://localhost:${localPort}/coverage/reset`;
        const response = await axios.get(resetUrl, { timeout: timeout * 1000 });

        if (response.status === 200) {
          console.log('[coverage-client] ✓ Coverage counters reset');
          return true;
        } else {
          console.error(`[coverage-client] Failed to reset coverage: HTTP ${response.status}`);
          return false;
        }
      } finally {
        server.close();
      }
    } catch (error) {
      console.error(`[coverage-client] Error resetting coverage:`, error.message);
      return false;
    }
  }

  /**
   * Generate text coverage report from collected data
   * @param {string} testName - Name of the test
   * @param {string} sourceDir - Source directory for coverage analysis
   * @param {boolean} remapPaths - If true, remap container paths to local paths
   */
  async generateCoverageReport(testName, sourceDir = '.', remapPaths = true) {
    const coverageFile = resolve(this.outputDir, `coverage_${testName}.json`);
    
    if (!existsSync(coverageFile)) {
      console.error(`[coverage-client] Coverage file not found: ${coverageFile}`);
      return;
    }

    // Load and potentially remap coverage data
    let coverageData = JSON.parse(await readFile(coverageFile, 'utf8'));
    
    if (remapPaths) {
      coverageData = await this._remapCoveragePaths(coverageData, sourceDir);
    }

    // Save remapped coverage
    const remappedFile = resolve(this.outputDir, `coverage_${testName}_remapped.json`);
    await writeFile(remappedFile, JSON.stringify(coverageData, null, 2));

    // Generate text report using NYC
    const reportFile = resolve(this.outputDir, `report_${testName}.txt`);
    
    return new Promise((resolve) => {
      const nycProcess = spawn('npx', [
        'nyc',
        'report',
        '--reporter=text',
        '--temp-dir', this.outputDir,
      ], {
        stdio: ['ignore', 'pipe', 'pipe'],
      });

      let output = '';
      nycProcess.stdout.on('data', (data) => {
        output += data.toString();
      });

      nycProcess.on('close', async (code) => {
        if (code === 0) {
          await writeFile(reportFile, output);
          console.log(`[coverage-client] Text report saved to ${reportFile}`);
          console.log(output);
        } else {
          console.error(`[coverage-client] Failed to generate report (exit code ${code})`);
        }
        resolve();
      });
    });
  }

  /**
   * Generate HTML coverage report from collected data
   * @param {string} testName - Name of the test
   * @param {string} sourceDir - Source directory for coverage analysis
   * @param {boolean} remapPaths - If true, remap container paths to local paths
   */
  async generateHtmlReport(testName, sourceDir = '.', remapPaths = true) {
    const coverageFile = resolve(this.outputDir, `coverage_${testName}.json`);
    
    if (!existsSync(coverageFile)) {
      console.error(`[coverage-client] Coverage file not found: ${coverageFile}`);
      return;
    }

    // Load and potentially remap coverage data
    let coverageData = JSON.parse(await readFile(coverageFile, 'utf8'));
    
    if (remapPaths) {
      coverageData = await this._remapCoveragePaths(coverageData, sourceDir);
    }

    // Create temporary .nyc_output directory
    const tempDir = resolve(this.outputDir, '.nyc_output');
    await mkdir(tempDir, { recursive: true });

    // Write coverage data to temp directory
    await writeFile(
      resolve(tempDir, 'coverage.json'),
      JSON.stringify(coverageData, null, 2)
    );

    // Generate HTML report
    const htmlDir = resolve(this.outputDir, `html_${testName}`);
    
    return new Promise((resolve) => {
      const nycProcess = spawn('npx', [
        'nyc',
        'report',
        '--reporter=html',
        '--report-dir', htmlDir,
        '--temp-dir', tempDir,
      ], {
        stdio: 'inherit',
      });

      nycProcess.on('close', async (code) => {
        if (code === 0) {
          console.log(`[coverage-client] HTML report saved to ${htmlDir}/index.html`);
        } else {
          console.error(`[coverage-client] Failed to generate HTML report (exit code ${code})`);
        }
        
        // Cleanup temp directory
        await rm(tempDir, { recursive: true, force: true });
        resolve();
      });
    });
  }

  /**
   * Generate XML coverage report (Cobertura format for CI tools)
   * @param {string} testName - Name of the test
   * @param {string} sourceDir - Source directory for coverage analysis
   * @param {boolean} remapPaths - If true, remap container paths to local paths
   */
  async generateXmlReport(testName, sourceDir = '.', remapPaths = true) {
    const coverageFile = resolve(this.outputDir, `coverage_${testName}.json`);
    
    if (!existsSync(coverageFile)) {
      console.error(`[coverage-client] Coverage file not found: ${coverageFile}`);
      return;
    }

    // Load and potentially remap coverage data
    let coverageData = JSON.parse(await readFile(coverageFile, 'utf8'));
    
    if (remapPaths) {
      coverageData = await this._remapCoveragePaths(coverageData, sourceDir);
    }

    // Create temporary .nyc_output directory
    const tempDir = resolve(this.outputDir, '.nyc_output');
    await mkdir(tempDir, { recursive: true });

    // Write coverage data to temp directory
    await writeFile(
      resolve(tempDir, 'coverage.json'),
      JSON.stringify(coverageData, null, 2)
    );

    // Generate XML report
    const xmlFile = resolve(this.outputDir, 'coverage.xml');
    
    return new Promise((resolve) => {
      const nycProcess = spawn('npx', [
        'nyc',
        'report',
        '--reporter=cobertura',
        '--report-dir', this.outputDir,
        '--temp-dir', tempDir,
      ], {
        stdio: 'inherit',
      });

      nycProcess.on('close', async (code) => {
        if (code === 0) {
          console.log(`[coverage-client] XML report saved to ${xmlFile}`);
        } else {
          console.error(`[coverage-client] Failed to generate XML report (exit code ${code})`);
        }
        
        // Cleanup temp directory
        await rm(tempDir, { recursive: true, force: true });
        resolve();
      });
    });
  }

  /**
   * Auto-detect container path mappings by analyzing coverage data.
   * Uses intelligent matching based on relative path structure.
   * @private
   */
  async _detectContainerPaths(coverageData, sourceDir) {
    const { readdirSync, statSync } = await import('fs');
    const { relative, sep } = await import('path');
    
    const measuredFiles = Object.keys(coverageData).sort();
    const containerFiles = [];
    
    // Find files that don't exist locally (these are container paths)
    for (const filePath of measuredFiles) {
      if (!existsSync(filePath)) {
        containerFiles.push(filePath);
      }
    }
    
    if (containerFiles.length === 0) {
      // No container paths detected - all files already have local paths
      return {};
    }
    
    // Build a map of relative paths to local file paths
    const sourcePathResolved = resolve(sourceDir);
    const localFilesByRelPath = new Map();
    
    // Collect all local JavaScript files with their relative path structure
    const collectLocalFiles = (dir) => {
      try {
        const entries = readdirSync(dir);
        for (const entry of entries) {
          // Skip node_modules and hidden directories
          if (entry === 'node_modules' || entry.startsWith('.')) {
            continue;
          }
          
          const fullPath = join(dir, entry);
          try {
            const stat = statSync(fullPath);
            if (stat.isDirectory()) {
              collectLocalFiles(fullPath);
            } else if (entry.endsWith('.js') || entry.endsWith('.mjs')) {
              const relPath = relative(sourcePathResolved, fullPath);
              const pathParts = relPath.split(sep);
              localFilesByRelPath.set(pathParts.join('/'), fullPath);
            }
          } catch (err) {
            // Skip files we can't read
          }
        }
      } catch (err) {
        // Skip directories we can't read
      }
    };
    
    collectLocalFiles(sourcePathResolved);
    
    // Try to find the best common container root by matching directory structures
    const potentialMappings = new Map();
    
    for (const containerFile of containerFiles) {
      const containerParts = containerFile.split('/').filter(p => p);
      const filename = containerParts[containerParts.length - 1];
      
      // Try to find matching local file based on path structure
      let bestMatch = null;
      let bestMatchScore = 0;
      
      for (const [localRelPath, localFullPath] of localFilesByRelPath.entries()) {
        const localParts = localRelPath.split('/');
        
        // Files must have same name
        if (localParts[localParts.length - 1] !== filename) {
          continue;
        }
        
        // Count matching suffix parts (from filename backwards)
        let matchScore = 0;
        for (let i = 1; i <= Math.min(containerParts.length, localParts.length); i++) {
          if (containerParts[containerParts.length - i] === localParts[localParts.length - i]) {
            matchScore = i;
          } else {
            break;
          }
        }
        
        // Prefer longer matches (more specific paths)
        if (matchScore > bestMatchScore) {
          bestMatchScore = matchScore;
          bestMatch = { localParts, localFullPath };
        }
      }
      
      if (bestMatch) {
        // Extract the container root by removing the matched suffix
        const containerRootParts = containerParts.slice(0, -bestMatchScore);
        if (containerRootParts.length > 0) {
          const containerRoot = '/' + containerRootParts.join('/') + '/';
          
          if (!potentialMappings.has(containerRoot)) {
            potentialMappings.set(containerRoot, []);
          }
          potentialMappings.get(containerRoot).push({
            containerFile,
            localFile: bestMatch.localFullPath,
            matchScore: bestMatchScore
          });
        }
      }
    }
    
    // Select the most common container root (the one with the most matches)
    const pathMappings = {};
    
    if (potentialMappings.size > 0) {
      // Sort by number of matches (descending), then alphabetically for determinism
      const sortedRoots = Array.from(potentialMappings.entries())
        .sort((a, b) => {
          const countDiff = b[1].length - a[1].length;
          if (countDiff !== 0) return countDiff;
          return a[0].localeCompare(b[0]);
        });
      
      // Use the root with the most matches
      const [bestRoot, matches] = sortedRoots[0];
      
      if (matches.length > 0) {
        const firstMatch = matches[0];
        const containerParts = firstMatch.containerFile.split('/').filter(p => p);
        const localParts = firstMatch.localFile.split(sep);
        const matchLen = firstMatch.matchScore;
        
        // Calculate local root: remove the matching suffix from local path
        const localRootParts = localParts.slice(0, -matchLen);
        if (localRootParts.length > 0) {
          let localRoot = localRootParts.join(sep);
          if (!localRoot.startsWith(sep)) {
            localRoot = sep + localRoot;
          }
          if (!localRoot.endsWith(sep)) {
            localRoot += sep;
          }
          pathMappings[bestRoot] = localRoot;
        }
      }
    }
    
    return pathMappings;
  }

  /**
   * Remap coverage paths from container to local filesystem.
   * Automatically detects container paths by analyzing the coverage data.
   * @private
   */
  async _remapCoveragePaths(coverageData, sourceDir) {
    // Auto-detect container paths
    const pathMappings = await this._detectContainerPaths(coverageData, sourceDir);
    
    if (Object.keys(pathMappings).length > 0) {
      console.log(`[coverage-client] Auto-detected path mappings:`, pathMappings);
    } else {
      console.log(`[coverage-client] No container paths detected, using paths as-is`);
    }
    
    const remapped = {};
    
    for (const [filePath, data] of Object.entries(coverageData)) {
      // Skip coverage server and other instrumentation files
      if (filePath.includes('coverage_server') || 
          filePath.includes('node_modules') || 
          filePath.includes('/server/') ||
          filePath.includes('/client/') ||
          filePath.includes('/test/')) {
        continue;
      }
      
      let newPath = filePath;
      
      // Try each path mapping
      for (const [containerPrefix, localPrefix] of Object.entries(pathMappings)) {
        if (filePath.startsWith(containerPrefix)) {
          newPath = filePath.replace(containerPrefix, localPrefix);
          break;
        }
      }
      
      // Only include if local file exists
      if (existsSync(newPath)) {
        // Clone the data object and update the internal path field
        const remappedData = { ...data };
        if (remappedData.path) {
          // Remap the internal path field as well
          for (const [containerPrefix, localPrefix] of Object.entries(pathMappings)) {
            if (remappedData.path.startsWith(containerPrefix)) {
              remappedData.path = remappedData.path.replace(containerPrefix, localPrefix);
              break;
            }
          }
        }
        remapped[newPath] = remappedData;
      }
    }
    
    console.log(`[coverage-client] Remapped ${Object.keys(remapped).length} files`);
    return remapped;
  }

  /**
   * Find a free local port
   * @private
   */
  async _findFreePort() {
    const { createServer } = await import('net');
    return new Promise((resolve, reject) => {
      const server = createServer();
      server.unref();
      server.on('error', reject);
      server.listen(0, () => {
        const { port } = server.address();
        server.close(() => resolve(port));
      });
    });
  }
}




