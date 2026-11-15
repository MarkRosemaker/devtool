// The smoke test, run by make smoke and on every pull request. harness.js,
// beside it, serves the app and fails on any error a page shows; this file
// says which pages to open and what to check on them. Add every screen.

const harness = require('./harness');

harness.run(async (app) => {
  await app.open('/');
});
