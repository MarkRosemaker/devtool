# Proving a test

Break the code on purpose and watch the test fail before trusting it. Copy
the file aside first and copy it back after; `git checkout` would also throw
away the change you are testing.

A mutation that leaves a variable unused does not compile, and a grep for
`FAIL` or `ok` over that output prints nothing — which reads like a pass at a
glance. Make the mutation keep every variable in use, such as `|| true`
appended to a condition, and grep for `FAIL` and `ok` both, so silence is
itself a failure to read.
