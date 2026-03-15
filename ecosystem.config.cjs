const path = require('path');

const isWindows = process.platform === 'win32';
const binaryName = isWindows ? 'localsend-go.exe' : 'localsend-go';

module.exports = {
  apps: [
    {
      name: 'localsend-go',
      cwd: __dirname,
      script: path.resolve(__dirname, binaryName),
      args: 'daemon --port 53317',
      interpreter: 'none',
      instances: 1,
      exec_mode: 'fork',
      autorestart: true,
      watch: false,
      max_memory_restart: '256M',
      env: {
        NODE_ENV: 'production'
      },
      env_production: {
        NODE_ENV: 'production'
      }
    }
  ]
};