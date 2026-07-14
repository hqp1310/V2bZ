const https = require('https');
const fs = require('fs');
const zlib = require('zlib');
const unzipper = require('unzipper');

const options = {
  hostname: 'api.github.com',
  path: '/repos/hqp1310/zicnode/actions/runs?per_page=1',
  headers: { 'User-Agent': 'NodeJS' }
};

https.get(options, (res) => {
  let data = '';
  res.on('data', chunk => data += chunk);
  res.on('end', () => {
    const runs = JSON.parse(data);
    const runId = runs.workflow_runs[0].id;
    console.log("Run ID:", runId);
    
    const logOptions = {
      hostname: 'api.github.com',
      path: `/repos/hqp1310/zicnode/actions/runs/${runId}/logs`,
      headers: { 'User-Agent': 'NodeJS' }
    };
    
    https.get(logOptions, (res2) => {
      if (res2.statusCode === 302) {
        https.get(res2.headers.location, (res3) => {
          res3.pipe(unzipper.Parse())
            .on('entry', function (entry) {
              if (entry.path.includes("Build ZicNode") || entry.path.includes("Build and Release")) {
                console.log("--- " + entry.path + " ---");
                entry.pipe(process.stdout);
              } else {
                entry.autodrain();
              }
            });
        });
      }
    });
  });
});
