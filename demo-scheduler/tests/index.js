import { spawn } from 'child_process';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const testFiles = [
//    'create_test_data.js', //redundant, as its called in test_send_event.js
    'test_send_event.js'
];

async function runTests() {
    for (const testFile of testFiles) {
        console.log(`\n=== Running ${testFile} ===\n`);
        
        try {
            await new Promise((resolve, reject) => {
                const proc = spawn('node', [path.join(__dirname, testFile)], {
                    stdio: 'inherit'
                });

                proc.on('close', (code) => {
                    if (code === 0) {
                        console.log(`\n ${testFile} completed successfully\n`);
                        resolve();
                    } else {
                        console.error(`\n ${testFile} failed with code ${code}\n`);
                        reject(new Error(`Test failed with code ${code}`));
                    }
                });

                proc.on('error', (err) => {
                    console.error(`\n Failed to start ${testFile}: ${err}\n`);
                    reject(err);
                });
            });
        } catch (err) {
            console.error(`Error running ${testFile}:`, err);
            process.exit(1);
        }
    }
}

runTests().catch(err => {
    console.error('Test suite failed:', err);
    process.exit(1);
});
