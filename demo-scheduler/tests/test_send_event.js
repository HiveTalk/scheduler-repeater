// run scheduler/tests/create_test_data.js to create 2 new events in 1 min from now
// immediately run scheduler/index.js which should fetch the 2 new events and send to nostr relay

import { spawn } from 'child_process';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// Function to run a script and return a promise
function runScript(scriptPath, args = []) {
    return new Promise((resolve, reject) => {
        const process = spawn('node', [scriptPath, ...args], {
            stdio: 'inherit'
        });

        process.on('close', (code) => {
            if (code !== 0) {
                reject(new Error(`Script exited with code ${code}`));
            } else {
                resolve();
            }
        });

        process.on('error', (err) => {
            reject(err);
        });
    });
}

async function runTest() {
    try {
        // Create 2 test events scheduled 1 minute from now
        console.log('Creating test events...');
        const createTestDataPath = path.join(__dirname, 'create_test_data.js');
        await runScript(createTestDataPath, ['2', '--in', '1']);

        // Run the scheduler to process these events
        console.log('Running scheduler...');
        const schedulerPath = path.join(__dirname, '..', 'index.js');
        await runScript(schedulerPath);

        console.log('Test completed successfully!');
    } catch (error) {
        console.error('Test failed:', error);
        process.exit(1);
    }
}

// Run the test
runTest();
