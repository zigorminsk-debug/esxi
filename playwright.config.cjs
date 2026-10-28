const { defineConfig } = require('@playwright/test');
module.exports = defineConfig({
  testDir: './tests/ui',
  timeout: 30000,
  fullyParallel: true,
  workers: 2,
  reporter: [['list'],['html',{open:'never'}]],
  use: { baseURL: 'http://127.0.0.1:3000', launchOptions: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ? {executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH} : {}, viewport: {width:1440,height:1000}, trace:'retain-on-failure', screenshot:'only-on-failure' },
  projects: [{name:'chromium',use:{browserName:'chromium'}},{name:'mobile',use:{browserName:'chromium',viewport:{width:390,height:844},isMobile:true,hasTouch:true}}],
  webServer: {command:'npm start',url:'http://127.0.0.1:3000',reuseExistingServer:!process.env.CI,timeout:30000}
});
